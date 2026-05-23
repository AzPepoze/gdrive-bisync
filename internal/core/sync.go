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
) error {
	ctx := context.Background()
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

	changedMetadata := make(map[string]*types.FileMetadata)
	deletedMetadataPaths := make([]string, 0)
	createdRemoteFolders := make(map[string]struct{})

	typeConflictDeletedMetadata, err := resolveRemoteTypeConflicts(ctx, driveService, localFiles, remoteFiles, metadata)
	if err != nil {
		return err
	}
	deletedMetadataPaths = append(deletedMetadataPaths, typeConflictDeletedMetadata...)

	initializeFolderMetadata(localFiles, remoteFiles, metadata)
	folderDeletedMetadata, err := reconcileFolders(ctx, driveService, resolvedLocalPath, localFiles, remoteFiles, metadata, cfg, dbStore, changedMetadata, createdRemoteFolders)
	if err != nil {
		return err
	}
	deletedMetadataPaths = append(deletedMetadataPaths, folderDeletedMetadata...)

	tasks := planSyncTasks(localFiles, remoteFiles, metadata, cfg.IgnoreRegexps, createdRemoteFolders)

	executor := NewExecutor(driveService, remoteFiles, metadata, cfg, resolvedLocalPath, sharedState)
	if err := executor.ExecuteTasks(tasks); err != nil {
		logger.Error("Sync tasks execution failed", "error", err)
	}

	if dbStore != nil {
		if err := dbStore.ReplaceAllRemoteFiles(remoteFiles); err != nil {
			logger.Error("Failed to save remote files", "error", err)
		}
		if err := dbStore.SaveMetadata(metadata, deletedMetadataPaths); err != nil {
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

func resolveRemoteTypeConflicts(
	ctx context.Context,
	driveService api.DriveClient,
	localFiles types.LocalFileMap,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
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
			if err := driveService.TrashRemoteFile(ctx, remote.ID); err != nil {
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
				delete(metadata, path)
				delete(idToPath, change.FileId)
				deletedPaths = append(deletedPaths, path)
				deletedMetadataPaths = append(deletedMetadataPaths, path)

				childPrefix := path + string(os.PathSeparator)
				for key := range remoteFiles {
					if strings.HasPrefix(key, childPrefix) {
						delete(remoteFiles, key)
						delete(metadata, key)
						deletedPaths = append(deletedPaths, key)
						deletedMetadataPaths = append(deletedMetadataPaths, key)
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
