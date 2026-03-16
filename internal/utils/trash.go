package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func MoveToTrash(rootPath string, filePath string) error {
	trashDir := filepath.Join(rootPath, ".trash")

	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return fmt.Errorf("failed to create trash directory: %w", err)
	}

	baseName := filepath.Base(filePath)
	timestamp := time.Now().Format("20060102-150405")
	trashName := fmt.Sprintf("%s_%s", timestamp, baseName)
	trashPath := filepath.Join(trashDir, trashName)

	counter := 1
	for {
		if _, err := os.Stat(trashPath); err != nil {
			if os.IsNotExist(err) {
				break
			}
			return err
		}

		trashPath = filepath.Join(trashDir, fmt.Sprintf("%s_%s_%d", timestamp, baseName, counter))
		counter++
		if counter > 1000 {
			return fmt.Errorf("could not find unique trash path for %s", filePath)
		}
	}

	if err := os.Rename(filePath, trashPath); err != nil {
		return fmt.Errorf("failed to move to trash: %w", err)
	}

	return nil
}
