package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidPath     = errors.New("invalid path")
	ErrEscapesRoot     = errors.New("path escapes user root")
	ErrContentTooLarge = errors.New("content too large")
)

type ResolvedPath struct {
	Logical  string
	Physical string
	Root     string
}

func CleanLogical(input string) (string, error) {
	if input == "" {
		return "/", nil
	}
	if strings.ContainsRune(input, 0) {
		return "", ErrInvalidPath
	}

	normalized := strings.ReplaceAll(input, "\\", "/")
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}

	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", ErrInvalidPath
		}
	}

	clean := path.Clean(normalized)
	if clean == "." {
		return "/", nil
	}
	return clean, nil
}

func JoinLogical(parent, name string) (string, error) {
	if err := ValidateBaseName(name); err != nil {
		return "", err
	}
	cleanParent, err := CleanLogical(parent)
	if err != nil {
		return "", err
	}
	return path.Join(cleanParent, name), nil
}

func ValidateBaseName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrInvalidPath
	}
	if strings.ContainsRune(name, 0) || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return ErrInvalidPath
	}
	return nil
}

func ResolveExisting(root, logical string) (ResolvedPath, error) {
	cleanLogical, candidate, rootAbs, err := joinedPath(root, logical)
	if err != nil {
		return ResolvedPath{}, err
	}

	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return ResolvedPath{}, err
	}
	candidateEval, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return ResolvedPath{}, err
	}
	if err := ensureWithin(rootEval, candidateEval); err != nil {
		return ResolvedPath{}, err
	}
	return ResolvedPath{Logical: cleanLogical, Physical: candidate, Root: rootEval}, nil
}

func ResolveParent(root, logical string) (ResolvedPath, string, error) {
	cleanLogical, err := CleanLogical(logical)
	if err != nil {
		return ResolvedPath{}, "", err
	}
	if cleanLogical == "/" {
		return ResolvedPath{}, "", ErrInvalidPath
	}
	name := path.Base(cleanLogical)
	if err := ValidateBaseName(name); err != nil {
		return ResolvedPath{}, "", err
	}
	parent := path.Dir(cleanLogical)
	resolvedParent, err := ResolveExisting(root, parent)
	if err != nil {
		return ResolvedPath{}, "", err
	}
	info, err := os.Stat(resolvedParent.Physical)
	if err != nil {
		return ResolvedPath{}, "", err
	}
	if !info.IsDir() {
		return ResolvedPath{}, "", fmt.Errorf("%w: parent is not a directory", ErrInvalidPath)
	}
	return resolvedParent, name, nil
}

func EnsureNoExisting(root, logical string) error {
	_, err := ResolveExisting(root, logical)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func joinedPath(root, logical string) (string, string, string, error) {
	cleanLogical, err := CleanLogical(logical)
	if err != nil {
		return "", "", "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", "", err
	}
	rel := strings.TrimPrefix(cleanLogical, "/")
	candidate := filepath.Join(rootAbs, filepath.FromSlash(rel))
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", "", err
	}
	if err := ensureWithin(rootAbs, candidateAbs); err != nil {
		return "", "", "", err
	}
	return cleanLogical, candidateAbs, rootAbs, nil
}

func ensureWithin(root, candidate string) error {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return ErrEscapesRoot
	}
	return nil
}
