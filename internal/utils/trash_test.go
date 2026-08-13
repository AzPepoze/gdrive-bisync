package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrashPathPreservesOriginalPathAndRestores(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "notes", "project", "plan.txt")
	if err := os.MkdirAll(filepath.Dir(original), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := TrashPath(root, original)
	if err != nil {
		t.Fatalf("trash path: %v", err)
	}
	if entry.OriginalPath != "notes/project/plan.txt" {
		t.Fatalf("unexpected original path: %s", entry.OriginalPath)
	}
	if _, err := os.Stat(entry.TrashPath); err != nil {
		t.Fatalf("trashed payload missing: %v", err)
	}

	entries, err := ListTrash(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("list trash: entries=%d err=%v", len(entries), err)
	}
	if _, err := RestoreTrash(root, entry.ID); err != nil {
		t.Fatalf("restore trash: %v", err)
	}
	content, err := os.ReadFile(original)
	if err != nil || string(content) != "keep me" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
}

func TestRestoreTrashRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "same.txt")
	if err := os.WriteFile(original, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	entry, err := TrashPath(root, original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreTrash(root, entry.ID); err == nil {
		t.Fatal("expected restore to refuse overwrite")
	}
}

func TestPathWithinRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if pathWithin(root, filepath.Join(root, "..", "outside")) {
		t.Fatal("expected traversal outside root to be rejected")
	}
	if !pathWithin(root, filepath.Join(root, "nested", "file")) {
		t.Fatal("expected nested path to be accepted")
	}
}
