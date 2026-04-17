package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMoveToTrash(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "trash_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	rootPath := tempDir
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	err = MoveToTrash(rootPath, testFile)
	if err != nil {
		t.Fatalf("MoveToTrash failed: %v", err)
	}

	dateFolder := time.Now().Format("02-01-2006")
	expectedPath := filepath.Join(rootPath, ".trash", dateFolder, "test.txt")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s, but it doesn't exist", expectedPath)
	}

	// Test collision
	if err := os.WriteFile(testFile, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	err = MoveToTrash(rootPath, testFile)
	if err != nil {
		t.Fatalf("MoveToTrash failed on second call: %v", err)
	}

	expectedPath2 := filepath.Join(rootPath, ".trash", dateFolder, "test (2).txt")
	if _, err := os.Stat(expectedPath2); os.IsNotExist(err) {
		t.Errorf("Expected file at %s, but it doesn't exist", expectedPath2)
	}

	// Test another collision
	if err := os.WriteFile(testFile, []byte("again"), 0644); err != nil {
		t.Fatal(err)
	}
	err = MoveToTrash(rootPath, testFile)
	if err != nil {
		t.Fatalf("MoveToTrash failed on third call: %v", err)
	}

	expectedPath3 := filepath.Join(rootPath, ".trash", dateFolder, "test (3).txt")
	if _, err := os.Stat(expectedPath3); os.IsNotExist(err) {
		t.Errorf("Expected file at %s, but it doesn't exist", expectedPath3)
	}
}
