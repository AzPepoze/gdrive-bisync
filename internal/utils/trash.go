package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func MoveToTrash(rootPath string, filePath string) error {
	dateFolder := time.Now().Format("02-01-2006")
	trashDir := filepath.Join(rootPath, ".trash", dateFolder)

	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return fmt.Errorf("failed to create trash directory: %w", err)
	}

	baseName := filepath.Base(filePath)
	trashPath := filepath.Join(trashDir, baseName)

	if _, err := os.Stat(trashPath); err == nil {
		extension := filepath.Ext(baseName)
		nameWithoutExtension := strings.TrimSuffix(baseName, extension)
		counter := 2
		for {
			trashPath = filepath.Join(trashDir, fmt.Sprintf("%s (%d)%s", nameWithoutExtension, counter, extension))
			if _, err := os.Stat(trashPath); os.IsNotExist(err) {
				break
			}
			counter++
			if counter > 1000 {
				return fmt.Errorf("could not find unique trash path for %s", filePath)
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(filePath, trashPath); err != nil {
		return fmt.Errorf("failed to move to trash: %w", err)
	}

	return nil
}
