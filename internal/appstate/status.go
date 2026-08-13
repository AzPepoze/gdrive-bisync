package appstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Status struct {
	PID              int       `json:"pid"`
	State            string    `json:"state"`
	Paused           bool      `json:"paused"`
	StartedAt        time.Time `json:"startedAt"`
	LastSyncStarted  time.Time `json:"lastSyncStarted,omitempty"`
	LastSyncFinished time.Time `json:"lastSyncFinished,omitempty"`
	NextSync         time.Time `json:"nextSync,omitempty"`
	LastError        string    `json:"lastError,omitempty"`
	TaskCount        int       `json:"taskCount"`
}

func WriteStatus(path string, status Status) error {
	encoded, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".status-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func ReadStatus(path string) (Status, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(encoded, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func SetPaused(path string, paused bool) error {
	if paused {
		return os.WriteFile(path, []byte("paused\n"), 0600)
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func IsPaused(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
