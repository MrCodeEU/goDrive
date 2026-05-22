package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"godrive/internal/auth"
	"godrive/internal/preview"
	"godrive/internal/store"
)

type Service struct {
	trashDir string
	store    *store.Store
}

type Entry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modified_at"`
	MimeType    string    `json:"mime_type,omitempty"`
	PreviewKind string    `json:"preview_kind,omitempty"`
}

type TrashMeta struct {
	ID           string    `json:"id"`
	UserID       int64     `json:"user_id"`
	OriginalPath string    `json:"original_path"`
	OriginalName string    `json:"original_name"`
	IsDir        bool      `json:"is_dir"`
	Size         int64     `json:"size"`
	DeletedAt    time.Time `json:"deleted_at"`
}

func NewService(trashDir string, store *store.Store) *Service {
	return &Service{trashDir: trashDir, store: store}
}

func (s *Service) List(ctx context.Context, user store.User, logical string) ([]Entry, error) {
	resolved, err := ResolveExisting(user.HomeRoot, logical)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved.Physical)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: not a directory", ErrInvalidPath)
	}

	dirEntries, err := os.ReadDir(resolved.Physical)
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		entryInfo, err := dirEntry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}

		entryType := "file"
		if entryInfo.IsDir() {
			entryType = "dir"
		}

		logicalPath, err := JoinLogical(resolved.Logical, dirEntry.Name())
		if err != nil {
			continue
		}
		mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(dirEntry.Name())))
		entries = append(entries, Entry{
			Name:        dirEntry.Name(),
			Path:        logicalPath,
			Type:        entryType,
			Size:        entryInfo.Size(),
			ModifiedAt:  entryInfo.ModTime().UTC(),
			MimeType:    mimeType,
			PreviewKind: preview.KindForName(dirEntry.Name()),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "dir"
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries, nil
}

func (s *Service) Mkdir(ctx context.Context, user store.User, logical string) (Entry, error) {
	parent, name, err := ResolveParent(user.HomeRoot, logical)
	if err != nil {
		return Entry{}, err
	}
	target := filepath.Join(parent.Physical, name)
	if err := os.Mkdir(target, 0o750); err != nil {
		return Entry{}, err
	}
	finalLogical, err := JoinLogical(parent.Logical, name)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return Entry{}, err
	}
	return entryFromInfo(name, finalLogical, info), nil
}

func (s *Service) Move(ctx context.Context, user store.User, from, to string) (Entry, error) {
	source, err := ResolveExisting(user.HomeRoot, from)
	if err != nil {
		return Entry{}, err
	}
	if source.Logical == "/" {
		return Entry{}, fmt.Errorf("%w: cannot move user root", ErrInvalidPath)
	}
	parent, name, err := ResolveParent(user.HomeRoot, to)
	if err != nil {
		return Entry{}, err
	}
	target := filepath.Join(parent.Physical, name)
	if _, err := os.Lstat(target); err == nil {
		return Entry{}, fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Entry{}, err
	}

	if err := movePath(source.Physical, target); err != nil {
		return Entry{}, err
	}

	finalLogical, err := JoinLogical(parent.Logical, name)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return Entry{}, err
	}
	return entryFromInfo(name, finalLogical, info), nil
}

func (s *Service) MoveInto(ctx context.Context, user store.User, from, targetDir string) (Entry, error) {
	source, err := ResolveExisting(user.HomeRoot, from)
	if err != nil {
		return Entry{}, err
	}
	if source.Logical == "/" {
		return Entry{}, fmt.Errorf("%w: cannot move user root", ErrInvalidPath)
	}

	sourceInfo, err := os.Stat(source.Physical)
	if err != nil {
		return Entry{}, err
	}

	parent, err := ResolveExisting(user.HomeRoot, targetDir)
	if err != nil {
		return Entry{}, err
	}
	parentInfo, err := os.Stat(parent.Physical)
	if err != nil {
		return Entry{}, err
	}
	if !parentInfo.IsDir() {
		return Entry{}, fmt.Errorf("%w: target is not a directory", ErrInvalidPath)
	}
	if filepath.Clean(filepath.Dir(source.Physical)) == filepath.Clean(parent.Physical) {
		return Entry{}, fmt.Errorf("%w: source is already in target", fs.ErrExist)
	}
	if sourceInfo.IsDir() && isSameOrChild(source.Physical, parent.Physical) {
		return Entry{}, fmt.Errorf("%w: cannot move a folder into itself", ErrInvalidPath)
	}

	name, target, err := AvailableName(parent.Physical, path.Base(source.Logical))
	if err != nil {
		return Entry{}, err
	}
	if source.Physical == target {
		return Entry{}, fmt.Errorf("%w: source already exists in target", fs.ErrExist)
	}
	if err := movePath(source.Physical, target); err != nil {
		return Entry{}, err
	}

	finalLogical, err := JoinLogical(parent.Logical, name)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return Entry{}, err
	}
	return entryFromInfo(name, finalLogical, info), nil
}

func (s *Service) DeleteToTrash(ctx context.Context, user store.User, logical string) (store.TrashItem, error) {
	resolved, err := ResolveExisting(user.HomeRoot, logical)
	if err != nil {
		return store.TrashItem{}, err
	}
	if resolved.Logical == "/" {
		return store.TrashItem{}, fmt.Errorf("%w: cannot trash user root", ErrInvalidPath)
	}

	info, err := os.Stat(resolved.Physical)
	if err != nil {
		return store.TrashItem{}, err
	}

	id, err := auth.RandomID(16)
	if err != nil {
		return store.TrashItem{}, err
	}
	itemDir := filepath.Join(s.trashDir, fmt.Sprintf("%d", user.ID), id)
	if err := os.MkdirAll(itemDir, 0o750); err != nil {
		return store.TrashItem{}, err
	}

	trashPath := filepath.Join(itemDir, "file")
	item := store.TrashItem{
		ID:           id,
		UserID:       user.ID,
		OriginalPath: resolved.Logical,
		OriginalName: path.Base(resolved.Logical),
		TrashPath:    trashPath,
		IsDir:        info.IsDir(),
		Size:         info.Size(),
		DeletedAt:    time.Now().UTC(),
	}
	meta := TrashMeta{
		ID:           item.ID,
		UserID:       item.UserID,
		OriginalPath: item.OriginalPath,
		OriginalName: item.OriginalName,
		IsDir:        item.IsDir,
		Size:         item.Size,
		DeletedAt:    item.DeletedAt,
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return store.TrashItem{}, err
	}
	if err := os.WriteFile(filepath.Join(itemDir, "meta.json"), metaBytes, 0o640); err != nil {
		return store.TrashItem{}, err
	}

	if err := movePath(resolved.Physical, trashPath); err != nil {
		return store.TrashItem{}, err
	}
	if err := s.store.CreateTrashItem(ctx, item); err != nil {
		return store.TrashItem{}, err
	}
	return item, nil
}

func isSameOrChild(parent, candidate string) bool {
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(parentAbs, candidateAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

func (s *Service) RestoreTrash(ctx context.Context, user store.User, id string) (Entry, error) {
	item, err := s.store.GetTrashItem(ctx, user.ID, id)
	if err != nil {
		return Entry{}, err
	}

	parentLogical := path.Dir(item.OriginalPath)
	parent, err := ResolveExisting(user.HomeRoot, parentLogical)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(parent.Physical)
	if err != nil {
		return Entry{}, err
	}
	if !info.IsDir() {
		return Entry{}, fmt.Errorf("%w: original parent is not a directory", ErrInvalidPath)
	}

	name, target, err := AvailableName(parent.Physical, item.OriginalName)
	if err != nil {
		return Entry{}, err
	}
	if err := movePath(item.TrashPath, target); err != nil {
		return Entry{}, err
	}
	if err := s.store.DeleteTrashItem(ctx, user.ID, id); err != nil {
		return Entry{}, err
	}
	_ = os.RemoveAll(filepath.Dir(item.TrashPath))

	finalLogical, err := JoinLogical(parent.Logical, name)
	if err != nil {
		return Entry{}, err
	}
	restoredInfo, err := os.Stat(target)
	if err != nil {
		return Entry{}, err
	}
	return entryFromInfo(name, finalLogical, restoredInfo), nil
}

func (s *Service) PermanentlyDeleteTrash(ctx context.Context, user store.User, id string) error {
	item, err := s.store.GetTrashItem(ctx, user.ID, id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Dir(item.TrashPath)); err != nil {
		return err
	}
	return s.store.DeleteTrashItem(ctx, user.ID, id)
}

func (s *Service) ResolveForRead(user store.User, logical string) (ResolvedPath, os.FileInfo, error) {
	resolved, err := ResolveExisting(user.HomeRoot, logical)
	if err != nil {
		return ResolvedPath{}, nil, err
	}
	info, err := os.Stat(resolved.Physical)
	if err != nil {
		return ResolvedPath{}, nil, err
	}
	if info.IsDir() {
		return ResolvedPath{}, nil, fmt.Errorf("%w: cannot read a directory", ErrInvalidPath)
	}
	return resolved, info, nil
}

func (s *Service) FinalizeUpload(user store.User, tempPath, targetDir, filename string) (Entry, error) {
	if err := ValidateBaseName(filename); err != nil {
		return Entry{}, err
	}
	parent, err := ResolveExisting(user.HomeRoot, targetDir)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(parent.Physical)
	if err != nil {
		return Entry{}, err
	}
	if !info.IsDir() {
		return Entry{}, fmt.Errorf("%w: target is not a directory", ErrInvalidPath)
	}

	name, target, err := AvailableName(parent.Physical, filename)
	if err != nil {
		return Entry{}, err
	}
	if err := movePath(tempPath, target); err != nil {
		return Entry{}, err
	}

	finalLogical, err := JoinLogical(parent.Logical, name)
	if err != nil {
		return Entry{}, err
	}
	finalInfo, err := os.Stat(target)
	if err != nil {
		return Entry{}, err
	}
	return entryFromInfo(name, finalLogical, finalInfo), nil
}

// WriteContent atomically replaces the content of an existing regular file.
// Capped at maxBytes to prevent runaway writes.
func (s *Service) WriteContent(user store.User, logical string, body io.Reader, maxBytes int64) error {
	if maxBytes < 0 {
		return ErrInvalidPath
	}
	resolved, info, err := s.ResolveForRead(user, logical)
	if err != nil {
		return err
	}
	if info.Size() > maxBytes {
		return ErrContentTooLarge
	}

	tmp := resolved.Physical + ".godrive-edit.tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	written, err := io.Copy(f, io.LimitReader(body, maxBytes+1))
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if written > maxBytes {
		_ = f.Close()
		_ = os.Remove(tmp)
		return ErrContentTooLarge
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, resolved.Physical)
}

func AvailableName(parentPhysical, filename string) (string, string, error) {
	if err := ValidateBaseName(filename); err != nil {
		return "", "", err
	}

	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	for i := 0; i < 10_000; i++ {
		name := filename
		if i > 0 {
			name = fmt.Sprintf("%s_%02d%s", base, i, ext)
		}
		candidate := filepath.Join(parentPhysical, name)
		if _, err := os.Lstat(candidate); errors.Is(err, fs.ErrNotExist) {
			return name, candidate, nil
		} else if err != nil {
			return "", "", err
		}
	}

	return "", "", fmt.Errorf("%w: no available filename after suffix attempts", fs.ErrExist)
}

func entryFromInfo(name, logical string, info os.FileInfo) Entry {
	entryType := "file"
	if info.IsDir() {
		entryType = "dir"
	}
	return Entry{
		Name:        name,
		Path:        logical,
		Type:        entryType,
		Size:        info.Size(),
		ModifiedAt:  info.ModTime().UTC(),
		MimeType:    mime.TypeByExtension(strings.ToLower(filepath.Ext(name))),
		PreviewKind: preview.KindForName(name),
	}
}

func movePath(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if err := copyPath(source, target); err != nil {
		_ = os.RemoveAll(target)
		return err
	}
	return os.RemoveAll(source)
}

func copyPath(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		linkTarget, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(linkTarget, target)
	case info.IsDir():
		if err := os.Mkdir(target, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	default:
		src, err := os.Open(source)
		if err != nil {
			return err
		}
		defer func() {
			_ = src.Close()
		}()

		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			_ = dst.Close()
			return err
		}
		if err := dst.Close(); err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	}
}
