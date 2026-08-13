package appstate

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MaxRuntimeEvents = 2000

type Event struct {
	Time     time.Time      `json:"time"`
	Level    string         `json:"level"`
	Category string         `json:"category"`
	Message  string         `json:"message"`
	Fields   map[string]any `json:"fields,omitempty"`
}

type EventJournal struct {
	path  string
	mu    sync.Mutex
	count int
}

func OpenEventJournal(path string) (*EventJournal, error) {
	journal := &EventJournal{path: path}
	events, err := ReadEvents(path, MaxRuntimeEvents)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	journal.count = len(events)
	return journal, nil
}

func (journal *EventJournal) Append(event Event) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(journal.path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(journal.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(event)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	journal.count++
	if journal.count > MaxRuntimeEvents+200 {
		return journal.compact()
	}
	return nil
}

func (journal *EventJournal) compact() error {
	events, err := ReadEvents(journal.path, MaxRuntimeEvents)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(journal.path), ".events-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	encoder := json.NewEncoder(temporary)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			_ = temporary.Close()
			return err
		}
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, journal.path); err != nil {
		return err
	}
	journal.count = len(events)
	return nil
}

func ReadEvents(path string, limit int) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	events := make([]Event, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			events = append(events, event)
			if limit > 0 && len(events) > limit {
				events = events[len(events)-limit:]
			}
		}
	}
	return events, scanner.Err()
}

func Request(path string) error {
	return os.WriteFile(path, []byte(time.Now().Format(time.RFC3339Nano)), 0600)
}

func ConsumeRequest(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return os.Remove(path) == nil
}
