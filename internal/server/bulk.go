package server

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"godrive/internal/files"
	"godrive/internal/store"
)

type bulkRequest struct {
	Paths []string `json:"paths"`
}

type bulkMoveRequest struct {
	Paths     []string `json:"paths"`
	TargetDir string   `json:"target_dir"`
}

type bulkResult struct {
	Path  string       `json:"path"`
	OK    bool         `json:"ok"`
	Error string       `json:"error,omitempty"`
	Entry *files.Entry `json:"entry,omitempty"`
}

func (s *Server) bulkDelete(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	var req bulkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "paths are required")
		return
	}

	results := make([]bulkResult, 0, len(req.Paths))
	for _, logical := range uniqueStrings(req.Paths) {
		_, err := s.files.DeleteToTrash(r.Context(), user, logical)
		if err != nil {
			results = append(results, bulkResult{Path: logical, OK: false, Error: err.Error()})
			continue
		}
		s.deleteIndexPath(r.Context(), user, logical)
		s.fireEvent(user, "file.deleted", map[string]any{"path": logical})
		results = append(results, bulkResult{Path: logical, OK: true})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) bulkMove(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	var req bulkMoveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Paths) == 0 || req.TargetDir == "" {
		writeError(w, http.StatusBadRequest, "paths and target_dir are required")
		return
	}

	results := make([]bulkResult, 0, len(req.Paths))
	for _, logical := range uniqueStrings(req.Paths) {
		entry, err := s.files.MoveInto(r.Context(), user, logical, req.TargetDir)
		if err != nil {
			results = append(results, bulkResult{Path: logical, OK: false, Error: err.Error()})
			continue
		}
		s.deleteIndexPath(r.Context(), user, logical)
		s.refreshIndexPath(r.Context(), user, entry.Path)
		s.fireEvent(user, "file.moved", map[string]any{"old_path": logical, "path": entry.Path})
		results = append(results, bulkResult{Path: logical, OK: true, Entry: &entry})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) bulkDownload(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	var req bulkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	paths := uniqueStrings(req.Paths)
	if len(paths) == 0 {
		writeError(w, http.StatusBadRequest, "paths are required")
		return
	}

	sources := make([]zipSource, 0, len(paths))
	archiveNames := map[string]struct{}{}
	for _, logical := range paths {
		resolved, err := files.ResolveExisting(user.HomeRoot, logical)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		if resolved.Logical == "/" {
			writeError(w, http.StatusBadRequest, "cannot download user root as a bulk item")
			return
		}
		info, err := os.Stat(resolved.Physical)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		name := uniqueArchiveName(path.Base(resolved.Logical), archiveNames)
		sources = append(sources, zipSource{
			Logical:     resolved.Logical,
			Physical:    resolved.Physical,
			ArchiveName: name,
			IsDir:       info.IsDir(),
		})
	}

	filename := "godrive-selection-" + time.Now().UTC().Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+quoteHeaderValue(filename))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	for _, source := range sources {
		if err := addZipSource(zw, source); err != nil {
			s.log.Warn("bulk zip entry failed", "logical", source.Logical, "err", err)
			break
		}
	}
	if err := zw.Close(); err != nil {
		s.log.Warn("bulk zip close failed", "err", err)
	}
}

type zipSource struct {
	Logical     string
	Physical    string
	ArchiveName string
	IsDir       bool
}

func addZipSource(zw *zip.Writer, source zipSource) error {
	info, err := os.Lstat(source.Physical)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !source.IsDir {
		return addZipFile(zw, source.Physical, source.ArchiveName, info)
	}

	return filepath.WalkDir(source.Physical, func(current string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}

		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(source.Physical, current)
		if err != nil {
			return err
		}
		name := source.ArchiveName
		if rel != "." {
			name = path.Join(source.ArchiveName, filepath.ToSlash(rel))
		}
		if info.IsDir() {
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = name
			_, err = zw.CreateHeader(header)
			return err
		}
		return addZipFile(zw, current, name, info)
	})
}

func addZipFile(zw *zip.Writer, physical, archiveName string, info fs.FileInfo) error {
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = archiveName
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	file, err := os.Open(physical)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = io.Copy(writer, file)
	return err
}

func uniqueArchiveName(name string, used map[string]struct{}) string {
	if _, ok := used[name]; !ok {
		used[name] = struct{}{}
		return name
	}

	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := base + "_" + twoDigit(i) + ext
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return strconv.Itoa(n)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
