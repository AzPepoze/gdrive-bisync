package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"gdrive-bisync/internal/appstate"
)

func TestModelPauseKeyUsesSharedControlLayer(t *testing.T) {
	directory := t.TempDir()
	paths := appstate.Paths{
		Directory:  directory,
		LockFile:   filepath.Join(directory, "lock"),
		StatusFile: filepath.Join(directory, "status.json"),
		PauseFile:  filepath.Join(directory, "paused"),
	}
	if err := appstate.WriteStatus(paths.StatusFile, appstate.Status{PID: 123, State: "idle"}); err != nil {
		t.Fatal(err)
	}
	model := NewModel(paths, directory)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !appstate.IsPaused(paths.PauseFile) {
		t.Fatal("expected pause key to create pause marker")
	}
	if updated.(Model).message != "Sync paused" {
		t.Fatalf("unexpected message: %q", updated.(Model).message)
	}
}
