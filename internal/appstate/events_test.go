package appstate

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestEventJournalCompactsAndKeepsNewestEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	journal, err := OpenEventJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MaxRuntimeEvents+250; index++ {
		if err := journal.Append(Event{Message: fmt.Sprintf("event-%d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := ReadEvents(path, MaxRuntimeEvents)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != MaxRuntimeEvents {
		t.Fatalf("expected %d events, got %d", MaxRuntimeEvents, len(events))
	}
	if events[len(events)-1].Message != fmt.Sprintf("event-%d", MaxRuntimeEvents+249) {
		t.Fatal("newest event was not preserved")
	}
}

func TestRequestIsConsumedOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync-now")
	if err := Request(path); err != nil {
		t.Fatal(err)
	}
	if !ConsumeRequest(path) {
		t.Fatal("expected request")
	}
	if ConsumeRequest(path) {
		t.Fatal("request consumed twice")
	}
}
