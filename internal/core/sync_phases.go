package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/services/scanner"
	"gdrive-bisync/internal/store"
	"gdrive-bisync/internal/types"
	"gdrive-bisync/internal/utils"
)

func ensureSyncRoot(cfg *config.Config) (string, error) {
	resolvedLocalPath := utils.ResolvePath(cfg.LocalSyncPath)
	if resolvedLocalPath == "" || cfg.RemoteFolderID == "" {
		logger.Error("Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.")
		return "", fmt.Errorf("invalid configuration")
	}

	if err := os.MkdirAll(resolvedLocalPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create local sync path: %w", err)
	}

	return resolvedLocalPath, nil
}

func scanLocalFiles(resolvedLocalPath string, cfg *config.Config, metadata map[string]*types.FileMetadata) (types.LocalFileMap, error) {
	logger.Info("Scanning local files...")
	localFiles, err := scanner.GetLocalFilesRecursive(resolvedLocalPath, cfg.IgnoreRegexps, metadata, func(path string) {
		logger.UpdateStatus(fmt.Sprintf("Scanning local: %s", path))
	})
	if err != nil {
		return nil, err
	}
	logger.UpdateStatus("")
	logger.Info("Local scan complete.", "files", len(localFiles))
	return localFiles, nil
}

func refreshRemoteState(
	ctx context.Context,
	driveService api.DriveClient,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	pageToken *string,
	dbStore *store.Store,
) error {
	if len(remoteFiles) == 0 || *pageToken == "" {
		logger.Info("Performing full remote scan...", "concurrency", cfg.MaxConcurrentScans, "retries", cfg.MaxRetries)

		token, err := driveService.GetStartPageToken(ctx)
		if err != nil {
			logger.Error("Failed to get start page token", "error", err)
		} else {
			*pageToken = token
		}

		currentRemoteFiles, err := driveService.ListFilesRecursive(ctx, api.ListFilesRequest{
			FolderID:      cfg.RemoteFolderID,
			IgnoreRegexps: cfg.IgnoreRegexps,
			OnProgress: func(path string) {
				logger.UpdateStatus(fmt.Sprintf("Scanning remote: %s", path))
			},
			MaxConcurrent: cfg.MaxConcurrentScans,
			MaxRetries:    cfg.MaxRetries,
		})
		if err != nil {
			return err
		}
		logger.UpdateStatus("")
		logger.Info("Remote scan complete.", "files", len(currentRemoteFiles))

		for key := range remoteFiles {
			delete(remoteFiles, key)
		}
		for key, value := range currentRemoteFiles {
			remoteFiles[key] = value
		}

		if dbStore != nil {
			if err := dbStore.ReplaceAllRemoteFiles(remoteFiles); err != nil {
				logger.Error("Failed to persist remote files after full scan", "error", err)
			}
		}

		return nil
	}

	logger.Info("Checking for remote changes...", "token", *pageToken)
	changeResult, err := driveService.FetchChanges(ctx, api.FetchChangesRequest{PageToken: *pageToken})
	if err != nil {
		logger.Error("Failed to fetch changes, falling back to full scan", "error", err)
		*pageToken = ""
		return err
	}

	relevantChanges := filterRelevantChanges(changeResult.Changes, remoteFiles)
	if len(relevantChanges) > 0 {
		logger.Info(fmt.Sprintf("Processing %d relevant remote changes (of %d total)...", len(relevantChanges), len(changeResult.Changes)))
		changedByApply, deletedByApply, deletedMetadataByApply := applyChanges(relevantChanges, remoteFiles, metadata, cfg.RemoteFolderID, cfg.IgnoreRegexps)
		if dbStore != nil {
			if err := dbStore.SaveRemoteFiles(changedByApply, deletedByApply); err != nil {
				logger.Error("Failed to persist incremental remote changes", "error", err)
			}
			if err := dbStore.SaveMetadata(metadata, deletedMetadataByApply); err != nil {
				logger.Error("Failed to persist incremental metadata changes", "error", err)
			}
		}
	} else {
		logger.Info("No remote changes found.")
	}

	*pageToken = changeResult.NewPageToken
	return nil
}

func initializeFolderMetadata(localFiles types.LocalFileMap, remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata) {
	for path, local := range localFiles {
		if !local.IsDirectory {
			continue
		}

		if _, remoteExists := remoteFiles[path]; remoteExists {
			if _, metaExists := metadata[path]; !metaExists {
				metadata[path] = &types.FileMetadata{}
			}
		}
	}
}

func reconcileFolders(
	ctx context.Context,
	driveService api.DriveClient,
	resolvedLocalPath string,
	localFiles types.LocalFileMap,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	dbStore *store.Store,
	changedMetadata map[string]*types.FileMetadata,
	createdRemoteFolders map[string]struct{},
) ([]string, error) {
	deletedMetadataPaths := make([]string, 0)
	localFolders := localDirectories(localFiles)

	for _, folder := range localFolders {
		if _, exists := remoteFiles[folder.Path]; exists {
			continue
		}

		if _, inMetadata := metadata[folder.Path]; inMetadata {
			childPrefix := folder.Path + string(os.PathSeparator)
			childrenStillInRemote := false
			for remotePath := range remoteFiles {
				if strings.HasPrefix(remotePath, childPrefix) {
					childrenStillInRemote = true
					break
				}
			}
			if childrenStillInRemote {
				logger.Warn("Folder missing from remote map but children still present, skipping deletion", "path", folder.Path)
				continue
			}

			logger.Info("Folder deleted remotely. Moving to trash.", "path", folder.Path)
			fullPath := filepath.Join(resolvedLocalPath, folder.Path)
			if err := utils.MoveToTrash(resolvedLocalPath, fullPath); err != nil {
				logger.Error("Failed to move local folder to trash", "path", folder.Path, "error", err)
			} else {
				delete(metadata, folder.Path)
				deletedMetadataPaths = append(deletedMetadataPaths, folder.Path)
				for key := range metadata {
					if strings.HasPrefix(key, childPrefix) {
						delete(metadata, key)
						deletedMetadataPaths = append(deletedMetadataPaths, key)
					}
				}
			}
			continue
		}

		parentFolderID, ok := resolveRemoteParentFolderID(folder.Path, remoteFiles, cfg.RemoteFolderID)
		if !ok {
			logger.Warn("Skipping folder creation, parent not found", "folder", folder.Path)
			continue
		}

		logger.Info("Creating remote folder", "path", folder.Path)
		newDriveFile, err := driveService.CreateFolder(ctx, api.CreateFolderRequest{
			ParentFolderID: parentFolderID,
			FolderName:     filepath.Base(folder.Path),
			RemotePath:     folder.Path,
		})
		if err != nil {
			logger.Error("Failed to create remote folder", "path", folder.Path, "error", err)
			continue
		}

		remoteFiles[folder.Path] = newDriveFile
		if dbStore != nil {
			if err := dbStore.SaveRemoteFiles(types.DriveFileMap{folder.Path: newDriveFile}, nil); err != nil {
				logger.Error("Failed to persist new remote folder", "path", folder.Path, "error", err)
			}
		}
		metadata[folder.Path] = &types.FileMetadata{}
		changedMetadata[folder.Path] = metadata[folder.Path]
		createdRemoteFolders[folder.Path] = struct{}{}
	}

	return deletedMetadataPaths, nil
}

func planSyncTasks(
	localFiles types.LocalFileMap,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	ignoreRegexps []*regexp.Regexp,
	createdRemoteFolders map[string]struct{},
) []types.SyncTask {
	allPaths := make(map[string]struct{}, len(localFiles)+len(remoteFiles))
	for key := range localFiles {
		allPaths[key] = struct{}{}
	}
	for key := range remoteFiles {
		allPaths[key] = struct{}{}
	}

	tasks := make([]types.SyncTask, 0)
	for pathStr := range allPaths {
		if shouldIgnorePath(pathStr, ignoreRegexps) {
			continue
		}

		local := localFiles[pathStr]
		remote := remoteFiles[pathStr]
		isDir := (local != nil && local.IsDirectory) || (remote != nil && remote.IsDirectory)
		if isDir {
			if local == nil && remote != nil && shouldDeleteRemoteDirectory(pathStr, localFiles, metadata) {
				tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteRemote, FilePath: pathStr})
			}
			continue
		}

		action := determineTaskAction(pathStr, local, remote, metadata, createdRemoteFolders)
		if action != types.ActionSkipNoChange && action != types.ActionSkipIdentical {
			tasks = append(tasks, types.SyncTask{Action: action, FilePath: pathStr})
		}
	}

	return tasks
}

func localDirectories(localFiles types.LocalFileMap) []*types.LocalFile {
	localFolders := make([]*types.LocalFile, 0)
	for _, file := range localFiles {
		if file.IsDirectory {
			localFolders = append(localFolders, file)
		}
	}

	sort.Slice(localFolders, func(i, j int) bool {
		return strings.Count(localFolders[i].Path, string(os.PathSeparator)) < strings.Count(localFolders[j].Path, string(os.PathSeparator))
	})

	return localFolders
}

func shouldIgnorePath(path string, ignoreRegexps []*regexp.Regexp) bool {
	for _, re := range ignoreRegexps {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
