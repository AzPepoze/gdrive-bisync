package utils

import (
	"os"
	"path/filepath"
	"strings"
)

func ResolvePath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return p
}
