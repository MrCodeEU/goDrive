package search_test

import (
	"context"
	"strings"
	"testing"

	"godrive/internal/search"
)

func newTestEngine(t *testing.T) *search.BleveEngine {
	t.Helper()
	e, err := search.OpenBleve(t.TempDir() + "/idx")
	if err != nil {
		t.Fatalf("OpenBleve: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestBleveEngine_IndexAndSearchByName(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/docs/report.txt", "report.txt", ""); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "report", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/docs/report.txt" {
		t.Fatalf("expected 1 result /docs/report.txt, got %v", results)
	}
}

func TestBleveEngine_MultiTokenAND(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/notes/tasks.txt", "tasks.txt", "help the planner with tasks"); err != nil {
		t.Fatal(err)
	}
	if err := e.IndexFile(ctx, 1, "/notes/help.txt", "help.txt", "some help documentation"); err != nil {
		t.Fatal(err)
	}

	results, err := e.Search(ctx, 1, "help plan", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range results {
		if r.Path == "/notes/tasks.txt" {
			found = true
		}
		if r.Path == "/notes/help.txt" {
			t.Errorf("/notes/help.txt should not match 'help plan' (only has 'help')")
		}
	}
	if !found {
		t.Errorf("expected /notes/tasks.txt in results for 'help plan', got %v", results)
	}
}

func TestBleveEngine_Stemming(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/q/planning.txt", "planning.txt", "quarterly planning document"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "plan", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range results {
		if r.Path == "/q/planning.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("stemming: 'plan' should match content 'planning', got %v", results)
	}
}

func TestBleveEngine_UserIsolation(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/secret.txt", "secret.txt", "classified"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 2, "classified", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("user 2 must not see user 1 results, got %v", results)
	}
}

func TestBleveEngine_Delete(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/docs/old.txt", "old.txt", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.Delete(ctx, 1, "/docs/old.txt"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "old", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Path == "/docs/old.txt" {
			t.Error("deleted path must not appear in search results")
		}
	}
}

func TestBleveEngine_DeletePrefix(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	for _, p := range []struct{ path, name, content string }{
		{"/tree/a.txt", "a.txt", "alpha content"},
		{"/tree/sub/b.txt", "b.txt", "beta content"},
		{"/other/c.txt", "c.txt", "alpha content"},
	} {
		if err := e.IndexFile(ctx, 1, p.path, p.name, p.content); err != nil {
			t.Fatal(err)
		}
	}

	if err := e.DeletePrefix(ctx, 1, "/tree"); err != nil {
		t.Fatal(err)
	}

	results, err := e.Search(ctx, 1, "alpha", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Path == "/tree/a.txt" || r.Path == "/tree/sub/b.txt" {
			t.Errorf("path under deleted prefix must not appear: %v", r.Path)
		}
	}
	var found bool
	for _, r := range results {
		if r.Path == "/other/c.txt" {
			found = true
		}
	}
	if !found {
		t.Error("/other/c.txt must still appear after DeletePrefix(/tree)")
	}
}

func TestBleveEngine_SnippetHasMarkTags(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/guide.txt", "guide.txt", "this document explains the planning process in detail"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "planning", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range results {
		if r.Path == "/guide.txt" && strings.Contains(r.Snippet, "<mark>") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected snippet with <mark> tags for /guide.txt; results: %v", results)
	}
}

func TestBleveEngine_IsEmpty(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	empty, err := e.IsEmpty()
	if err != nil || !empty {
		t.Fatalf("new engine should be empty, got empty=%v err=%v", empty, err)
	}
	if err := e.IndexFile(ctx, 1, "/x.txt", "x.txt", ""); err != nil {
		t.Fatal(err)
	}
	empty, err = e.IsEmpty()
	if err != nil || empty {
		t.Fatalf("after indexing should not be empty, got empty=%v err=%v", empty, err)
	}
}
