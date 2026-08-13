package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func BackupDatabase(databasePath string, keep int) (string, error) {
	if keep <= 0 {
		return "", nil
	}
	input, err := os.Open(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = input.Close() }()

	backupDirectory := filepath.Join(filepath.Dir(databasePath), ".gdrive-bisync-backups")
	if err := os.MkdirAll(backupDirectory, 0700); err != nil {
		return "", err
	}
	backupName := fmt.Sprintf("%s.%s.bak", filepath.Base(databasePath), time.Now().Format("20060102T150405.000000000"))
	backupPath := filepath.Join(backupDirectory, backupName)
	output, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(backupPath)
		return "", err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		_ = os.Remove(backupPath)
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	if err := pruneBackups(backupDirectory, filepath.Base(databasePath)+".", keep); err != nil {
		return backupPath, err
	}
	return backupPath, nil
}

func pruneBackups(directory string, prefix string, keep int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	backups := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".bak") {
			backups = append(backups, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(backups)
	for len(backups) > keep {
		if err := os.Remove(backups[0]); err != nil {
			return err
		}
		backups = backups[1:]
	}
	return nil
}
