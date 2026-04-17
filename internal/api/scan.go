package api

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/types"
)

func (s *DriveService) ListFilesRecursive(ctx context.Context, request ListFilesRequest) (types.DriveFileMap, error) {
	request = normalizeListRequest(request)

	fileMap := make(types.DriveFileMap)
	var mu sync.Mutex

	group, groupCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, request.MaxConcurrent)

	var walk func(folderID string, currentPath string) error
	walk = func(folderID string, currentPath string) error {
		if request.OnProgress != nil {
			request.OnProgress(currentPath)
		}

		select {
		case sem <- struct{}{}:
		case <-groupCtx.Done():
			return groupCtx.Err()
		}
		defer func() { <-sem }()

		childFolders := make([]*types.DriveFile, 0)
		err := s.retry(groupCtx, fmt.Sprintf("list folder %s", currentPath), normalizeRetryConfig(request.MaxRetries), func() error {
			call := applySharedDriveOptionsToFilesList(s.srv.Files.List()).
				Q(fmt.Sprintf("'%s' in parents and trashed = false", folderID)).
				Fields(driveFileFieldsSelector()).
				PageSize(1000).
				Context(groupCtx)

			return call.Pages(groupCtx, func(response *drive.FileList) error {
				pageChildren := make([]*types.DriveFile, 0)

				for _, file := range response.Files {
					filePath := filepath.Join(currentPath, file.Name)
					if shouldIgnorePath(filePath, request.IgnoreRegexps) {
						logger.Debug("Ignoring Drive file/folder", "path", filePath)
						continue
					}

					entry := buildDriveFile(filePath, file)

					mu.Lock()
					existing, exists := fileMap[filePath]
					if !exists || entry.IsDirectory || entry.ModifiedTime.After(existing.ModifiedTime) {
						fileMap[filePath] = entry
					}
					mu.Unlock()

					if entry.IsDirectory {
						pageChildren = append(pageChildren, entry)
					}
				}

				childFolders = append(childFolders, pageChildren...)
				return nil
			})
		})
		if err != nil {
			return err
		}

		for _, child := range childFolders {
			child := child
			group.Go(func() error {
				return walk(child.ID, child.Path)
			})
		}

		return nil
	}

	group.Go(func() error {
		return walk(request.FolderID, "")
	})

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return fileMap, nil
}
