package files

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanLogicalRejectsTraversal(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"../secret",
		"/photos/../../secret",
		`photos\..\secret`,
	}

	for _, input := range invalid {
		if _, err := CleanLogical(input); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("CleanLogical(%q) err = %v, want ErrInvalidPath", input, err)
		}
	}
}

func TestResolveExistingRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveExisting(root, "/outside"); !errors.Is(err, ErrEscapesRoot) {
		t.Fatalf("ResolveExisting symlink escape err = %v, want ErrEscapesRoot", err)
	}
}

func TestResolveExistingAllowsSymlinkWithinRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "photos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "photos"), filepath.Join(root, "photos-link")); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveExisting(root, "/photos-link")
	if err != nil {
		t.Fatalf("ResolveExisting symlink inside root err = %v", err)
	}
	if resolved.Logical != "/photos-link" {
		t.Fatalf("resolved logical = %q, want /photos-link", resolved.Logical)
	}
}

func TestAvailableNameUsesTwoDigitSuffix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range []string{"photo.jpg", "photo_01.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	name, target, err := AvailableName(root, "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if name != "photo_02.jpg" {
		t.Fatalf("name = %q, want photo_02.jpg", name)
	}
	if target != filepath.Join(root, "photo_02.jpg") {
		t.Fatalf("target = %q", target)
	}
}
