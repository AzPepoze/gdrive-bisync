package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupDatabaseCopiesAndRotates(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "state.db")
	for version := byte('1'); version <= '4'; version++ {
		if err := os.WriteFile(databasePath, []byte{version}, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := BackupDatabase(databasePath, 2); err != nil {
			t.Fatalf("backup version %c: %v", version, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(directory, ".gdrive-bisync-backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(entries))
	}
}
