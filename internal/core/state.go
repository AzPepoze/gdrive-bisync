package core

import (
	"encoding/json"
	"os"

	"gdrive-bisync/internal/types"
)

type State struct {
	PageToken   string             `json:"pageToken"`
	RemoteFiles types.DriveFileMap `json:"remoteFiles"`
}

func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.RemoteFiles == nil {
		state.RemoteFiles = make(types.DriveFileMap)
	}
	return &state, nil
}

// SaveState saves the state to the specified path.
func SaveState(path string, state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
