package tui

import (
	"path/filepath"
	"strings"
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

func TestModelNavigationPrivacyAndControlRequests(t *testing.T) {
	directory := t.TempDir()
	paths := appstate.Paths{Directory: directory, StatusFile: filepath.Join(directory, "status.json"), PauseFile: filepath.Join(directory, "paused"), EventsFile: filepath.Join(directory, "events.jsonl"), SyncNowFile: filepath.Join(directory, "sync-now"), DryRunFile: filepath.Join(directory, "dry-run")}
	if err := appstate.WriteStatus(paths.StatusFile, appstate.Status{PID: 1, State: "idle"}); err != nil {
		t.Fatal(err)
	}
	journal, err := appstate.OpenEventJournal(paths.EventsFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(appstate.Event{Level: "INFO", Category: "CORE", Message: "Uploaded", Fields: map[string]any{"path": "private/photo.jpg"}}); err != nil {
		t.Fatal(err)
	}
	model := NewModel(paths, directory)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	model = updated.(Model)
	if model.page != pageActivity {
		t.Fatal("activity page not selected")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = updated.(Model)
	if strings.Contains(model.View(), "private/") {
		t.Fatal("privacy mode exposed full path")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	_ = updated
	if !appstate.ConsumeRequest(paths.SyncNowFile) {
		t.Fatal("sync request was not written")
	}
}

func TestViewAdaptsToNarrowAndWideTerminals(t *testing.T) {
	directory := t.TempDir()
	paths := appstate.Paths{StatusFile: filepath.Join(directory, "status.json"), EventsFile: filepath.Join(directory, "events.jsonl"), PauseFile: filepath.Join(directory, "paused")}
	if err := appstate.WriteStatus(paths.StatusFile, appstate.Status{State: "idle"}); err != nil {
		t.Fatal(err)
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 55, Height: 18}, {Width: 140, Height: 40}} {
		model := NewModel(paths, directory)
		updated, _ := model.Update(size)
		view := updated.(Model).View()
		if !strings.Contains(view, "gdrive-bisync") || !strings.Contains(view, "CURRENT SYNC") {
			t.Fatalf("dashboard missing at size %#v", size)
		}
	}
}

func TestQuestionMarkOpensHelp(t *testing.T) {
	model := NewModel(appstate.Paths{}, t.TempDir())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if updated.(Model).page != pageHelp {
		t.Fatal("question mark did not open help")
	}
}

func TestLogArrowKeysScrollInTheirVisualDirection(t *testing.T) {
	model := NewModel(appstate.Paths{}, t.TempDir())
	model.follow = false
	model.scroll = 2

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.scroll != 1 {
		t.Fatalf("down arrow should move toward newer entries, got scroll offset %d", model.scroll)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.scroll != 2 {
		t.Fatalf("up arrow should move toward older entries, got scroll offset %d", model.scroll)
	}
}
