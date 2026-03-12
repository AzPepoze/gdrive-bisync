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

func GetLocalFilesRecursive(rootPath string, ignoreRegexps []*regexp.Regexp, metadata map[string]*types.FileMetadata, onProgress func(path string)) (types.LocalFileMap, error) {
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
			localFile.MD5Checksum = cachedMD5(path, relPath, info, metadata)
		}

		fileMap[relPath] = localFile
		return nil
	})

	return fileMap, err
}

func cachedMD5(absolutePath string, relPath string, info os.FileInfo, metadata map[string]*types.FileMetadata) string {
	if metadata != nil {
		if cached, exists := metadata[relPath]; exists {
			if cached.LocalSize == info.Size() && cached.LocalModTime.Equal(info.ModTime()) && cached.LocalMD5Checksum != "" {
				return cached.LocalMD5Checksum
			}
		}
	}

	computedMD5, err := GetFileMD5(absolutePath)
	if err != nil {
		logger.Warn("Failed to compute MD5", "path", absolutePath, "error", err)
		return ""
	}

	if metadata != nil {
		if existing, exists := metadata[relPath]; exists {
			existing.LocalMD5Checksum = computedMD5
			existing.LocalModTime = info.ModTime()
			existing.LocalSize = info.Size()
		} else {
			metadata[relPath] = &types.FileMetadata{
				LocalMD5Checksum: computedMD5,
				LocalModTime:     info.ModTime(),
				LocalSize:        info.Size(),
			}
		}
	}

	return computedMD5
}
