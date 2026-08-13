package appstate

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Directory  string
	LockFile   string
	StatusFile string
	PauseFile  string
}

func DefaultPaths() (Paths, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	directory := filepath.Join(configDir, "gdrive-bisync", "runtime")
	return Paths{
		Directory:  directory,
		LockFile:   filepath.Join(directory, "instance.lock"),
		StatusFile: filepath.Join(directory, "status.json"),
		PauseFile:  filepath.Join(directory, "paused"),
	}, nil
}

func (paths Paths) EnsureDirectory() error {
	return os.MkdirAll(paths.Directory, 0700)
}
