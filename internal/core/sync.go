package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/store"
	"gdrive-bisync/internal/types"
)

func Sync(
	driveService api.DriveClient,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	pageToken *string,
	dbStore *store.Store,
	sharedState *SharedState,
	options SyncOptions,
) error {
	ctx := context.Background()
	if options.DryRun {
		remoteFiles = cloneRemoteFiles(remoteFiles)
		metadata = cloneMetadata(metadata)
		dbStore = nil
	}
	resolvedLocalPath, err := ensureSyncRoot(cfg)
	if err != nil {
		return err
	}

	logger.Info("Starting sync cycle...")

	localFiles, err := scanLocalFiles(resolvedLocalPath, cfg, metadata)
	if err != nil {
		return err
	}

	if err := refreshRemoteState(ctx, driveService, remoteFiles, metadata, cfg, pageToken, dbStore); err != nil {
		return err
	}
	preflightTasks := collapseLocalDeletionSubtrees(planDeletionPreflight(localFiles, remoteFiles, metadata, cfg.IgnoreRegexps), localFiles)
	if err := validateDeletionSafety(preflightTasks, len(localFiles), cfg, options.AllowUnsafeDeletes); err != nil {
		return err
	}

	changedMetadata := make(map[string]*types.FileMetadata)
	createdRemoteFolders := make(map[string]struct{})

	_, err = resolveRemoteTypeConflicts(ctx, driveService, localFiles, remoteFiles, metadata, options.DryRun)
	if err != nil {
		return err
	}

	initializeFolderMetadata(localFiles, remoteFiles, metadata)
	_, folderTasks, err := reconcileFolders(ctx, driveService, resolvedLocalPath, localFiles, remoteFiles, metadata, cfg, dbStore, changedMetadata, createdRemoteFolders, options.DryRun)
	if err != nil {
		return err
	}

	tasks := planSyncTasks(localFiles, remoteFiles, metadata, cfg.IgnoreRegexps, createdRemoteFolders)
	tasks = append(tasks, folderTasks...)
	tasks = collapseLocalDeletionSubtrees(tasks, localFiles)
	if options.OnPlan != nil {
		options.OnPlan(len(tasks))
	}
	if err := validateDeletionSafety(tasks, len(localFiles), cfg, options.AllowUnsafeDeletes); err != nil {
		return err
	}

	executor := NewExecutor(driveService, remoteFiles, metadata, cfg, resolvedLocalPath, sharedState, options.DryRun)
	if err := executor.ExecuteTasks(tasks); err != nil {
		return fmt.Errorf("sync tasks execution failed: %w", err)
	}

	if dbStore != nil && !options.DryRun {
		if err := dbStore.ReplaceAllRemoteFiles(remoteFiles); err != nil {
			logger.Error("Failed to save remote files", "error", err)
		}
		if err := dbStore.ReplaceAllMetadata(metadata); err != nil {
			logger.Error("Failed to save metadata", "error", err)
		}
		if err := dbStore.SavePageToken(*pageToken); err != nil {
			logger.Error("Failed to save page token", "error", err)
		}
	}

	logger.Info("Sync cycle finished.")
	logger.Info("--------------------------------------------------")
	return nil
}

func collapseLocalDeletionSubtrees(tasks []types.SyncTask, localFiles types.LocalFileMap) []types.SyncTask {
	deletedFolders := make([]string, 0)
	for _, task := range tasks {
		local := localFiles[task.FilePath]
		if task.Action == types.ActionDeleteLocal && local != nil && local.IsDirectory {
			deletedFolders = append(deletedFolders, task.FilePath)
		}
	}
	if len(deletedFolders) == 0 {
		return tasks
	}
	filtered := make([]types.SyncTask, 0, len(tasks))
	for _, task := range tasks {
		isDescendant := false
		for _, folder := range deletedFolders {
			if task.FilePath != folder && strings.HasPrefix(task.FilePath, folder+string(os.PathSeparator)) {
				isDescendant = true
				break
			}
		}
		if !isDescendant {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func planDeletionPreflight(
	localFiles types.LocalFileMap,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	ignoreRegexps []*regexp.Regexp,
) []types.SyncTask {
	tasks := make([]types.SyncTask, 0)
	seen := make(map[string]struct{})
	for path, local := range localFiles {
		if shouldIgnorePath(path, ignoreRegexps) {
			continue
		}
		remote := remoteFiles[path]
		if remote != nil && local.IsDirectory != remote.IsDirectory {
			tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteRemote, FilePath: path})
			seen[path] = struct{}{}
			continue
		}
		if remote == nil {
			if local.IsDirectory {
				if _, wasSynced := metadata[path]; wasSynced {
					tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteLocal, FilePath: path})
					seen[path] = struct{}{}
				}
			} else if DetermineSyncAction(path, local, nil, metadata) == types.ActionDeleteLocal {
				tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteLocal, FilePath: path})
				seen[path] = struct{}{}
			}
		}
	}
	for path, remote := range remoteFiles {
		if _, exists := seen[path]; exists || shouldIgnorePath(path, ignoreRegexps) {
			continue
		}
		if localFiles[path] == nil {
			if remote.IsDirectory {
				if shouldDeleteRemoteDirectory(path, localFiles, metadata) {
					tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteRemote, FilePath: path})
				}
			} else if DetermineSyncAction(path, nil, remote, metadata) == types.ActionDeleteRemote {
				tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteRemote, FilePath: path})
			}
		}
	}
	return tasks
}

func cloneRemoteFiles(source types.DriveFileMap) types.DriveFileMap {
	clone := make(types.DriveFileMap, len(source))
	for path, file := range source {
		if file == nil {
			continue
		}
		copy := *file
		clone[path] = &copy
	}
	return clone
}

func cloneMetadata(source map[string]*types.FileMetadata) map[string]*types.FileMetadata {
	clone := make(map[string]*types.FileMetadata, len(source))
	for path, metadata := range source {
		if metadata == nil {
			continue
		}
		copy := *metadata
		clone[path] = &copy
	}
	return clone
}

type SyncOptions struct {
	DryRun             bool
	AllowUnsafeDeletes bool
	OnPlan             func(taskCount int)
}

func validateDeletionSafety(tasks []types.SyncTask, localFileCount int, cfg *config.Config, allowUnsafe bool) error {
	if allowUnsafe {
		return nil
	}
	deletions := 0
	for _, task := range tasks {
		if task.Action == types.ActionDeleteLocal || task.Action == types.ActionDeleteRemote {
			deletions++
		}
	}
	if deletions == 0 {
		return nil
	}
	if cfg.MaxDeletionsPerSync > 0 && deletions > cfg.MaxDeletionsPerSync {
		return fmt.Errorf("deletion safety threshold exceeded: planned %d deletions, maximum is %d", deletions, cfg.MaxDeletionsPerSync)
	}
	if cfg.MaxDeletionPercent > 0 && localFileCount > 0 {
		percent := float64(deletions) * 100 / float64(localFileCount)
		if percent > cfg.MaxDeletionPercent {
			return fmt.Errorf("deletion safety threshold exceeded: %.1f%% planned, maximum is %.1f%%", percent, cfg.MaxDeletionPercent)
		}
	}
	return nil
}

func resolveRemoteTypeConflicts(
	ctx context.Context,
	driveService api.DriveClient,
	localFiles types.LocalFileMap,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	dryRun bool,
) ([]string, error) {
	deletedMetadataPaths := make([]string, 0)

	for path, local := range localFiles {
		remote := remoteFiles[path]
		if local == nil || remote == nil || local.IsDirectory == remote.IsDirectory {
			continue
		}

		if local.IsDirectory {
			logger.Warn("Replacing remote file with local folder", "path", path)
		} else {
			logger.Warn("Replacing remote folder with local file", "path", path)
		}

		if remote.ID != "" {
			if dryRun {
				logger.Info("[DRY-RUN] TRASH_REMOTE_TYPE_CONFLICT", "path", path)
			} else if err := driveService.TrashRemoteFile(ctx, remote.ID); err != nil {
				return deletedMetadataPaths, fmt.Errorf("failed to reconcile path type change for %s: %w", path, err)
			}
		}

		deletedMetadataPaths = append(deletedMetadataPaths, removeRemotePathAndMetadata(remoteFiles, metadata, path)...)
	}

	return deletedMetadataPaths, nil
}

func removeRemotePathAndMetadata(
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	path string,
) []string {
	deletedMetadataPaths := make([]string, 0)
	remote, exists := remoteFiles[path]
	if !exists {
		return deletedMetadataPaths
	}

	delete(remoteFiles, path)
	if _, ok := metadata[path]; ok {
		delete(metadata, path)
		deletedMetadataPaths = append(deletedMetadataPaths, path)
	}

	if remote.IsDirectory {
		childPrefix := path + string(os.PathSeparator)
		for childPath := range remoteFiles {
			if strings.HasPrefix(childPath, childPrefix) {
				delete(remoteFiles, childPath)
			}
		}
		for childPath := range metadata {
			if strings.HasPrefix(childPath, childPrefix) {
				delete(metadata, childPath)
				deletedMetadataPaths = append(deletedMetadataPaths, childPath)
			}
		}
	}

	return deletedMetadataPaths
}

func determineTaskAction(
	filePath string,
	localFile *types.LocalFile,
	remoteFile *types.DriveFile,
	metadata map[string]*types.FileMetadata,
	createdRemoteFolders map[string]struct{},
) types.SyncAction {
	action := DetermineSyncAction(filePath, localFile, remoteFile, metadata)
	if action == types.ActionDeleteLocal && localFile != nil && remoteFile == nil && hasAncestorInSet(filePath, createdRemoteFolders) {
		return types.ActionUploadNew
	}
	return action
}

func hasAncestorInSet(path string, folders map[string]struct{}) bool {
	for parent := filepath.Dir(path); parent != "." && parent != string(os.PathSeparator); parent = filepath.Dir(parent) {
		if _, ok := folders[parent]; ok {
			return true
		}
	}
	return false
}

func shouldDeleteRemoteDirectory(
	path string,
	localFiles types.LocalFileMap,
	metadata map[string]*types.FileMetadata,
) bool {
	if _, ok := metadata[path]; ok {
		return true
	}

	childPrefix := path + string(os.PathSeparator)
	for localPath := range localFiles {
		if strings.HasPrefix(localPath, childPrefix) {
			return false
		}
	}
	for metadataPath := range metadata {
		if strings.HasPrefix(metadataPath, childPrefix) {
			return true
		}
	}

	return false
}

func filterRelevantChanges(changes []*drive.Change, remoteFiles types.DriveFileMap) []*drive.Change {
	knownIDs := make(map[string]struct{}, len(remoteFiles))
	for _, driveFile := range remoteFiles {
		knownIDs[driveFile.ID] = struct{}{}
	}

	relevant := make([]*drive.Change, 0, len(changes))
	for _, change := range changes {
		if _, known := knownIDs[change.FileId]; known {
			relevant = append(relevant, change)
			continue
		}
		if change.File != nil && len(change.File.Parents) > 0 {
			for _, parentID := range change.File.Parents {
				if _, known := knownIDs[parentID]; known {
					relevant = append(relevant, change)
					break
				}
			}
		}
	}
	return relevant
}

func applyChanges(changes []*drive.Change, remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata, rootFolderID string, ignoreRegexps []*regexp.Regexp) (types.DriveFileMap, []string, []string) {
	changedFiles := make(types.DriveFileMap)
	deletedPaths := make([]string, 0)
	deletedMetadataPaths := make([]string, 0)

	idToPath := make(map[string]string)
	for path, driveFile := range remoteFiles {
		idToPath[driveFile.ID] = path
	}

	for _, change := range changes {
		if change.Removed || (change.File != nil && change.File.Trashed) {
			path, exists := idToPath[change.FileId]
			if exists {
				logger.Debug("Applying remote deletion", "path", path)
				delete(remoteFiles, path)
				delete(idToPath, change.FileId)
				deletedPaths = append(deletedPaths, path)

				childPrefix := path + string(os.PathSeparator)
				for key := range remoteFiles {
					if strings.HasPrefix(key, childPrefix) {
						delete(remoteFiles, key)
						deletedPaths = append(deletedPaths, key)
					}
				}
			}
			continue
		}

		if change.File != nil {
			file := change.File

			parentID := ""
			if len(file.Parents) > 0 {
				parentID = file.Parents[0]
			}

			parentPath := ""
			if parentID == rootFolderID {
				parentPath = ""
			} else {
				pPath, ok := idToPath[parentID]
				if !ok {
					logger.Debug("Parent not found in scope, skipping change to avoid false deletion", "fileId", change.FileId, "parentId", parentID)
					continue
				}
				parentPath = pPath
			}

			newPath := filepath.Join(parentPath, file.Name)

			ignored := false
			for _, re := range ignoreRegexps {
				if re.MatchString(newPath) {
					ignored = true
					break
				}
			}
			if ignored {
				if oldPath, exists := idToPath[change.FileId]; exists {
					delete(remoteFiles, oldPath)
					delete(metadata, oldPath)
					delete(idToPath, change.FileId)
					deletedPaths = append(deletedPaths, oldPath)
					deletedMetadataPaths = append(deletedMetadataPaths, oldPath)
				}
				continue
			}

			oldPath, exists := idToPath[change.FileId]
			isDirectory := file.MimeType == "application/vnd.google-apps.folder"
			modTime, _ := time.Parse(time.RFC3339, file.ModifiedTime)

			if exists && oldPath != newPath {
				logger.Debug("Applying remote move", "old", oldPath, "new", newPath)
				delete(remoteFiles, oldPath)
				deletedPaths = append(deletedPaths, oldPath)
				if oldMeta, metaExists := metadata[oldPath]; metaExists {
					metadata[newPath] = oldMeta
					delete(metadata, oldPath)
					deletedMetadataPaths = append(deletedMetadataPaths, oldPath)
				}

				if isDirectory {
					oldPrefix := oldPath + string(os.PathSeparator)
					newPrefix := newPath + string(os.PathSeparator)

					type childMove struct {
						oldKey string
						newKey string
						value  *types.DriveFile
					}
					var moves []childMove

					for key, value := range remoteFiles {
						if strings.HasPrefix(key, oldPrefix) {
							suffix := strings.TrimPrefix(key, oldPrefix)
							newKey := newPrefix + suffix
							moves = append(moves, childMove{key, newKey, value})
						}
					}

					for _, move := range moves {
						delete(remoteFiles, move.oldKey)
						deletedPaths = append(deletedPaths, move.oldKey)
						if oldMeta, metaExists := metadata[move.oldKey]; metaExists {
							metadata[move.newKey] = oldMeta
							delete(metadata, move.oldKey)
							deletedMetadataPaths = append(deletedMetadataPaths, move.oldKey)
						}
						move.value.Path = move.newKey
						remoteFiles[move.newKey] = move.value
						changedFiles[move.newKey] = move.value
						idToPath[move.value.ID] = move.newKey
					}
				}
			}

			newDriveFile := &types.DriveFile{
				ID:           file.Id,
				Name:         file.Name,
				Path:         newPath,
				ModifiedTime: modTime,
				MD5Checksum:  file.Md5Checksum,
				IsDirectory:  isDirectory,
			}
			remoteFiles[newPath] = newDriveFile
			changedFiles[newPath] = newDriveFile
			idToPath[file.Id] = newPath

			if file.MimeType != "application/vnd.google-apps.folder" {
				existing := metadata[newPath]
				if existing == nil {
					existing = &types.FileMetadata{}
					metadata[newPath] = existing
				}
				existing.RemoteMD5Checksum = file.Md5Checksum
			}
		}
	}

	return changedFiles, deletedPaths, deletedMetadataPaths
}
