//go:build !windows

package api

import "os"

func replaceFile(source string, destination string) error {
	return os.Rename(source, destination)
}
