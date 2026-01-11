package scanner

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/types"
)

// GetFileMD5 computes the MD5 checksum of a file.
func GetFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GetLocalFilesRecursive recursively scans local files.
func GetLocalFilesRecursive(rootPath string, ignoreRegexps []*regexp.Regexp, onProgress func(path string)) (types.LocalFileMap, error) {
	fileMap := make(types.LocalFileMap)

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == rootPath {
			return nil
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		if onProgress != nil {
			onProgress(relPath)
		}

		// Check ignore patterns
		ignored := false
		for _, re := range ignoreRegexps {
			if re.MatchString(relPath) {
				ignored = true
				break
			}
		}

		if ignored {
			if info.IsDir() {
				logger.Debug("Ignoring local folder", "path", relPath)
				return filepath.SkipDir
			}
			logger.Debug("Ignoring local file", "path", relPath)
			return nil
		}

		localFile := &types.LocalFile{
			Path:        relPath,
			ModTime:     info.ModTime(),
			IsDirectory: info.IsDir(),
		}

		if !info.IsDir() {
			md5, err := GetFileMD5(path)
			if err != nil {
				logger.Warn("Failed to compute MD5", "path", path, "error", err)
			} else {
				localFile.MD5Checksum = md5
			}
		}

		fileMap[relPath] = localFile
		return nil
	})

	return fileMap, err
}
