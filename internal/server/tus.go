package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"godrive/internal/auth"
	"godrive/internal/files"
	"godrive/internal/store"
)

const tusVersion = "1.0.0"

func (s *Server) tusOptions(w http.ResponseWriter, r *http.Request) {
	s.writeTusHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tusCreate(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	if !validTusVersion(r) {
		writeError(w, http.StatusPreconditionFailed, "missing or unsupported Tus-Resumable header")
		return
	}
	uploadLength, err := parseUploadLength(r.Header.Get("Upload-Length"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid Upload-Length")
		return
	}

	metadata := parseUploadMetadata(r.Header.Get("Upload-Metadata"))
	filename := metadata["filename"]
	if filename == "" {
		filename = "upload.bin"
	}
	if err := files.ValidateBaseName(filename); err != nil {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	targetDir := r.URL.Query().Get("path")
	if targetDir == "" {
		targetDir = metadata["path"]
	}
	if targetDir == "" {
		targetDir = "/"
	}
	if _, err := files.ResolveExisting(user.HomeRoot, targetDir); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}

	id, err := auth.RandomID(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create upload")
		return
	}
	userUploadDir := uploadUserDir(s.cfg.UploadDir, user.ID)
	if err := os.MkdirAll(userUploadDir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create upload directory")
		return
	}
	tempPath := expectedUploadTempPath(s.cfg.UploadDir, user.ID, id)

	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create upload")
		return
	}

	offset := int64(0)
	if r.Body != nil && r.ContentLength > 0 {
		offset, err = copyUploadChunk(file, r.Body, 0, uploadLength)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(tempPath)
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		writeError(w, http.StatusInternalServerError, "failed to create upload")
		return
	}

	metadataJSON, _ := json.Marshal(metadata)
	err = s.store.CreateUpload(r.Context(), store.Upload{
		ID:           id,
		UserID:       user.ID,
		UploadLength: uploadLength,
		Offset:       offset,
		MetadataJSON: string(metadataJSON),
		TargetDir:    targetDir,
		Filename:     filename,
		TempPath:     tempPath,
	})
	if err != nil {
		_ = os.Remove(tempPath)
		writeError(w, http.StatusInternalServerError, "failed to create upload record")
		return
	}

	location := "/api/tus/" + id
	s.writeTusHeaders(w)
	w.Header().Set("Location", location)
	w.Header().Set("Upload-Offset", strconv.FormatInt(offset, 10))
	w.WriteHeader(http.StatusCreated)

	if offset == uploadLength {
		if _, err := s.completeUpload(r, user, id); err != nil {
			s.log.Warn("failed to finalize zero-byte upload", "id", id, "err", err)
		}
	}
}

func (s *Server) tusHead(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	if !validTusVersion(r) {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	upload, ok := s.uploadForUser(w, r, user)
	if !ok {
		return
	}
	s.writeTusHeaders(w)
	w.Header().Set("Upload-Length", strconv.FormatInt(upload.UploadLength, 10))
	w.Header().Set("Upload-Offset", strconv.FormatInt(upload.Offset, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tusPatch(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	if !validTusVersion(r) {
		writeError(w, http.StatusPreconditionFailed, "missing or unsupported Tus-Resumable header")
		return
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "application/offset+octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/offset+octet-stream")
		return
	}

	upload, ok := s.uploadForUser(w, r, user)
	if !ok {
		return
	}
	if upload.CompletedAt.Valid {
		writeError(w, http.StatusConflict, "upload already completed")
		return
	}

	clientOffset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || clientOffset != upload.Offset {
		writeError(w, http.StatusConflict, "upload offset mismatch")
		return
	}
	if upload.Offset > upload.UploadLength {
		writeError(w, http.StatusConflict, "upload state is invalid")
		return
	}

	file, err := openUploadTempForWrite(s.cfg.UploadDir, upload)
	if err != nil {
		writeError(w, statusForError(err), "failed to open upload")
		return
	}
	defer func() {
		_ = file.Close()
	}()

	if err := file.Truncate(upload.Offset); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to repair upload offset")
		return
	}
	if _, err := file.Seek(upload.Offset, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to seek upload")
		return
	}

	newOffset, err := copyUploadChunk(file, r.Body, upload.Offset, upload.UploadLength)
	if err != nil {
		_ = file.Truncate(upload.Offset)
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if newOffset == upload.UploadLength {
		if err := file.Sync(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to sync upload")
			return
		}
	}

	if err := s.store.UpdateUploadOffset(r.Context(), upload.ID, newOffset); err != nil {
		writeError(w, statusForError(err), "failed to update upload")
		return
	}

	s.writeTusHeaders(w)
	w.Header().Set("Upload-Offset", strconv.FormatInt(newOffset, 10))
	if newOffset == upload.UploadLength {
		entry, err := s.completeUpload(r, user, upload.ID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		w.Header().Set("Upload-Final-Path", entry.Path)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tusDelete(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	if !validTusVersion(r) {
		writeError(w, http.StatusPreconditionFailed, "missing or unsupported Tus-Resumable header")
		return
	}
	upload, ok := s.uploadForUser(w, r, user)
	if !ok {
		return
	}
	if upload.CompletedAt.Valid {
		writeError(w, http.StatusConflict, "upload already completed")
		return
	}
	tempPath, err := validateUploadTempPath(s.cfg.UploadDir, upload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid upload temp file")
		return
	}
	_ = os.Remove(tempPath)
	if err := s.store.DeleteUpload(r.Context(), upload.ID); err != nil {
		writeError(w, statusForError(err), "failed to delete upload")
		return
	}
	s.writeTusHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uploadForUser(w http.ResponseWriter, r *http.Request, user store.User) (store.Upload, bool) {
	upload, err := s.store.GetUpload(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, statusForError(err), "upload not found")
		return store.Upload{}, false
	}
	if upload.UserID != user.ID {
		writeError(w, http.StatusNotFound, "upload not found")
		return store.Upload{}, false
	}
	return upload, true
}

func (s *Server) completeUpload(r *http.Request, user store.User, id string) (files.Entry, error) {
	upload, err := s.store.GetUpload(r.Context(), id)
	if err != nil {
		return files.Entry{}, err
	}
	if upload.CompletedAt.Valid {
		return files.Entry{Path: upload.FinalPath.String}, nil
	}
	if upload.Offset != upload.UploadLength {
		return files.Entry{}, errors.New("upload is incomplete")
	}
	if _, err := validateUploadTempPath(s.cfg.UploadDir, upload); err != nil {
		return files.Entry{}, fmt.Errorf("invalid upload temp file: %w", err)
	}

	entry, err := s.files.FinalizeUpload(user, upload.TempPath, upload.TargetDir, upload.Filename)
	if err != nil {
		return files.Entry{}, err
	}
	if err := s.store.CompleteUpload(r.Context(), upload.ID, entry.Path); err != nil {
		return files.Entry{}, err
	}
	s.refreshIndexPath(r.Context(), user, entry.Path)
	if entry.PreviewKind == "image" || entry.PreviewKind == "video" || entry.PreviewKind == "pdf" {
		go s.generateThumbnailsAsync(user, entry)
	}
	s.fireEvent(user, "upload.complete", map[string]any{
		"path":         entry.Path,
		"size":         entry.Size,
		"mime_type":    entry.MimeType,
		"preview_kind": entry.PreviewKind,
	})
	return entry, nil
}

func (s *Server) writeTusHeaders(w http.ResponseWriter) {
	w.Header().Set("Tus-Resumable", tusVersion)
	w.Header().Set("Tus-Version", tusVersion)
	w.Header().Set("Tus-Extension", "creation,termination")
	w.Header().Set("Access-Control-Expose-Headers", "Tus-Resumable,Tus-Version,Tus-Extension,Upload-Length,Upload-Offset,Upload-Final-Path,Location")
}

func validTusVersion(r *http.Request) bool {
	return r.Header.Get("Tus-Resumable") == tusVersion
}

func parseUploadLength(value string) (int64, error) {
	if value == "" {
		return 0, errors.New("missing Upload-Length")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("invalid Upload-Length")
	}
	return n, nil
}

func parseUploadMetadata(header string) map[string]string {
	metadata := map[string]string{}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, encoded, ok := strings.Cut(part, " ")
		if !ok || key == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		}
		if err == nil {
			metadata[key] = string(decoded)
		}
	}
	return metadata
}

func copyUploadChunk(file *os.File, reader io.Reader, offset, uploadLength int64) (int64, error) {
	remaining := uploadLength - offset
	if remaining < 0 {
		return offset, errors.New("upload offset exceeds upload length")
	}
	limited := io.LimitReader(reader, remaining+1)
	n, err := io.Copy(file, limited)
	if err != nil {
		return offset, err
	}
	if n > remaining {
		return offset, errors.New("chunk exceeds declared upload length")
	}
	return offset + n, nil
}

func uploadUserDir(uploadDir string, userID int64) string {
	return filepath.Join(uploadDir, fmt.Sprintf("%d", userID))
}

func expectedUploadTempPath(uploadDir string, userID int64, id string) string {
	return filepath.Join(uploadUserDir(uploadDir, userID), id+".part")
}

func validateUploadTempPath(uploadDir string, upload store.Upload) (string, error) {
	if strings.TrimSpace(uploadDir) == "" || strings.TrimSpace(upload.ID) == "" || strings.TrimSpace(upload.TempPath) == "" {
		return "", errors.New("upload temp path is incomplete")
	}
	expected, err := filepath.Abs(expectedUploadTempPath(uploadDir, upload.UserID, upload.ID))
	if err != nil {
		return "", err
	}
	actual, err := filepath.Abs(upload.TempPath)
	if err != nil {
		return "", err
	}
	expected = filepath.Clean(expected)
	actual = filepath.Clean(actual)
	if actual != expected {
		return "", fmt.Errorf("upload temp path %q does not match expected path", upload.TempPath)
	}
	info, err := os.Lstat(actual)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("upload temp path cannot be a symlink")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("upload temp path must be a regular file")
	}
	return actual, nil
}

func openUploadTempForWrite(uploadDir string, upload store.Upload) (*os.File, error) {
	tempPath, err := validateUploadTempPath(uploadDir, upload)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(tempPath, os.O_WRONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("upload temp path must be a regular file")
	}
	return file, nil
}
