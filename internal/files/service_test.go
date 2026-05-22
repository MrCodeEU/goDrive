package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"godrive/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestDeleteToTrashAndRestoreRoundtrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	trashDir := t.TempDir()
	st := openTestStore(t)
	svc := NewService(trashDir, st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("pixels"), 0o640); err != nil {
		t.Fatal(err)
	}

	item, err := svc.DeleteToTrash(context.Background(), user, "/photo.jpg")
	if err != nil {
		t.Fatalf("DeleteToTrash: %v", err)
	}
	if item.OriginalPath != "/photo.jpg" {
		t.Fatalf("item.OriginalPath = %q, want /photo.jpg", item.OriginalPath)
	}
	if _, err := os.Stat(filepath.Join(root, "photo.jpg")); !os.IsNotExist(err) {
		t.Fatal("source file should be gone after trash")
	}

	entry, err := svc.RestoreTrash(context.Background(), user, item.ID)
	if err != nil {
		t.Fatalf("RestoreTrash: %v", err)
	}
	if entry.Path != "/photo.jpg" {
		t.Fatalf("restored path = %q, want /photo.jpg", entry.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "photo.jpg")); err != nil {
		t.Fatalf("restored file should exist: %v", err)
	}
}

func TestRestoreTrashAppliesConflictSuffix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	trashDir := t.TempDir()
	st := openTestStore(t)
	svc := NewService(trashDir, st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "bob",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}

	// Trash the file but recreate the original before restoring.
	item, err := svc.DeleteToTrash(context.Background(), user, "/photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("newer"), 0o640); err != nil {
		t.Fatal(err)
	}

	entry, err := svc.RestoreTrash(context.Background(), user, item.ID)
	if err != nil {
		t.Fatalf("RestoreTrash with conflict: %v", err)
	}
	if entry.Path == "/photo.jpg" {
		t.Fatalf("restore should use conflict suffix, got %q", entry.Path)
	}
	if entry.Path != "/photo_01.jpg" {
		t.Fatalf("restored path = %q, want /photo_01.jpg", entry.Path)
	}
}

func TestRestoreTrashFailsWhenParentGone(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	trashDir := t.TempDir()
	st := openTestStore(t)
	svc := NewService(trashDir, st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "carol",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(root, "photos"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photos", "img.jpg"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}

	item, err := svc.DeleteToTrash(context.Background(), user, "/photos/img.jpg")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(root, "photos")); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RestoreTrash(context.Background(), user, item.ID); err == nil {
		t.Fatal("RestoreTrash should fail when original parent is missing")
	}
}

func TestPermanentlyDeleteTrash(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	trashDir := t.TempDir()
	st := openTestStore(t)
	svc := NewService(trashDir, st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "dave",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "doc.pdf"), []byte("content"), 0o640); err != nil {
		t.Fatal(err)
	}
	item, err := svc.DeleteToTrash(context.Background(), user, "/doc.pdf")
	if err != nil {
		t.Fatal(err)
	}
	trashPath := item.TrashPath

	if err := svc.PermanentlyDeleteTrash(context.Background(), user, item.ID); err != nil {
		t.Fatalf("PermanentlyDeleteTrash: %v", err)
	}
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Fatal("trash file should be permanently removed")
	}
}

func TestFinalizeUploadMovesToTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uploadDir := t.TempDir()
	st := openTestStore(t)
	svc := NewService(t.TempDir(), st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "eve",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	tempFile := filepath.Join(uploadDir, "tmp-123")
	if err := os.WriteFile(tempFile, []byte("upload content"), 0o640); err != nil {
		t.Fatal(err)
	}

	entry, err := svc.FinalizeUpload(user, tempFile, "/", "photo.jpg")
	if err != nil {
		t.Fatalf("FinalizeUpload: %v", err)
	}
	if entry.Path != "/photo.jpg" {
		t.Fatalf("entry.Path = %q, want /photo.jpg", entry.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "photo.jpg")); err != nil {
		t.Fatalf("finalized file should exist at target: %v", err)
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Fatal("temp file should be gone after finalization")
	}
}

func TestFinalizeUploadAppliesConflictSuffix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	uploadDir := t.TempDir()
	st := openTestStore(t)
	svc := NewService(t.TempDir(), st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "frank",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("existing"), 0o640); err != nil {
		t.Fatal(err)
	}

	tempFile := filepath.Join(uploadDir, "tmp-456")
	if err := os.WriteFile(tempFile, []byte("new upload"), 0o640); err != nil {
		t.Fatal(err)
	}

	entry, err := svc.FinalizeUpload(user, tempFile, "/", "photo.jpg")
	if err != nil {
		t.Fatalf("FinalizeUpload conflict: %v", err)
	}
	if entry.Path != "/photo_01.jpg" {
		t.Fatalf("entry.Path = %q, want /photo_01.jpg", entry.Path)
	}
}

func TestFinalizeUploadRejectsInvalidFilename(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	st := openTestStore(t)
	svc := NewService(t.TempDir(), st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "grace",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.FinalizeUpload(user, "/tmp/fake", "/", "../../escape.txt"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("FinalizeUpload traversal err = %v, want ErrInvalidPath", err)
	}
}

func TestWriteContentReplacesExistingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	st := openTestStore(t)
	svc := NewService(t.TempDir(), st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "writer",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := svc.WriteContent(user, "/note.txt", strings.NewReader("new content"), 32); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new content" {
		t.Fatalf("content = %q, want replacement", string(content))
	}
}

func TestWriteContentRejectsOversizedBodyWithoutTruncating(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	st := openTestStore(t)
	svc := NewService(t.TempDir(), st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "limit",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}

	err = svc.WriteContent(user, "/note.txt", strings.NewReader("123456"), 5)
	if !errors.Is(err, ErrContentTooLarge) {
		t.Fatalf("WriteContent oversized err = %v, want ErrContentTooLarge", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Fatalf("content = %q, want original content preserved", string(content))
	}
}

func TestWriteContentRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	st := openTestStore(t)
	svc := NewService(t.TempDir(), st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "escape",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}

	err = svc.WriteContent(user, "/link.txt", strings.NewReader("replacement"), 64)
	if !errors.Is(err, ErrEscapesRoot) {
		t.Fatalf("WriteContent symlink escape err = %v, want ErrEscapesRoot", err)
	}
	content, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "secret" {
		t.Fatalf("outside content = %q, want unchanged", string(content))
	}
}

func TestDeleteToTrashRejectsRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	st := openTestStore(t)
	svc := NewService(t.TempDir(), st)

	user, err := st.CreateUser(context.Background(), store.User{
		Username:     "hank",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.DeleteToTrash(context.Background(), user, "/"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("DeleteToTrash root err = %v, want ErrInvalidPath", err)
	}
}
