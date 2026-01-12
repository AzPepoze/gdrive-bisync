package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/services/scanner"
	"gdrive-bisync/internal/types"
	"gdrive-bisync/internal/utils"
)

func Sync(
	driveService *api.DriveService,
	remoteFiles types.DriveFileMap,
	metadata map[string]*types.FileMetadata,
	cfg *config.Config,
	pageToken *string, // Pointer to update the token
) error {
	resolvedLocalPath := utils.ResolvePath(cfg.LocalSyncPath)
	if resolvedLocalPath == "" || cfg.RemoteFolderID == "" {
		logger.Error("Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.")
		return fmt.Errorf("invalid configuration")
	}

	if err := os.MkdirAll(resolvedLocalPath, 0755); err != nil {
		return fmt.Errorf("failed to create local sync path: %w", err)
	}

	logger.Info("Starting sync cycle...")

	// --- Metadata Loading ---
	metadataPath := filepath.Join(resolvedLocalPath, cfg.MetadataFileName)
	if len(metadata) == 0 {
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.Error("Error loading sync metadata", "error", err)
			} else {
				logger.Warn("No sync metadata file found. Starting fresh.")
			}
		} else {
			var rawEntries []json.RawMessage
			if err := json.Unmarshal(data, &rawEntries); err == nil {
				for _, raw := range rawEntries {
					var entry []json.RawMessage
					if err := json.Unmarshal(raw, &entry); err == nil && len(entry) == 2 {
						var key string
						var val types.FileMetadata
						if err := json.Unmarshal(entry[0], &key); err == nil {
							if err := json.Unmarshal(entry[1], &val); err == nil {
								metadata[key] = &val
							}
						}
					}
				}
				logger.Info("Loaded sync metadata.")
			}
		}
	}

	// File Scanning
	logger.Info("Scanning local files...")
	localFiles, err := scanner.GetLocalFilesRecursive(resolvedLocalPath, cfg.IgnoreRegexps, func(path string) {
		logger.UpdateStatus(fmt.Sprintf("Scanning local: %s", path))
	})
	if err != nil {
		return err
	}
	logger.UpdateStatus("")
	logger.Info("Local scan complete.", "files", len(localFiles))

	// Remote Scanning / Updating
	if len(remoteFiles) == 0 || *pageToken == "" {
		logger.Info("Performing full remote scan...", "concurrency", cfg.MaxConcurrentScans, "retries", cfg.MaxRetries)

		token, err := driveService.GetStartPageToken()
		if err != nil {
			logger.Error("Failed to get start page token", "error", err)
		} else {
			*pageToken = token
		}

		currentRemoteFiles, err := driveService.ListFilesRecursive(
			cfg.RemoteFolderID,
			cfg.IgnoreRegexps,
			func(path string) {
				logger.UpdateStatus(fmt.Sprintf("Scanning remote: %s", path))
			},
			cfg.MaxConcurrentScans,
			cfg.MaxRetries,
		)
		if err != nil {
			return err
		}
		logger.UpdateStatus("")
		logger.Info("Remote scan complete.", "files", len(currentRemoteFiles))

		// Replace map
		for k := range remoteFiles {
			delete(remoteFiles, k)
		}
		for k, v := range currentRemoteFiles {
			remoteFiles[k] = v
		}

	} else {
		// Incremental Update
		logger.Info("Checking for remote changes...", "token", *pageToken)
		changes, newToken, err := driveService.FetchChanges(*pageToken)
		if err != nil {
			logger.Error("Failed to fetch changes, falling back to full scan", "error", err)
			*pageToken = ""
			return err
		}

		if len(changes) > 0 {
			logger.Info(fmt.Sprintf("Processing %d remote changes...", len(changes)))
			applyChanges(changes, remoteFiles, cfg.RemoteFolderID, cfg.IgnoreRegexps)
		} else {
			logger.Info("No remote changes found.")
		}
		*pageToken = newToken
	}

	// Metadata Maintenance
	for path, local := range localFiles {
		if local.IsDirectory {
			if _, remoteExists := remoteFiles[path]; remoteExists {
				if _, metaExists := metadata[path]; !metaExists {
					metadata[path] = &types.FileMetadata{}
				}
			}
		}
	}

	// Handle Folders
	localFolders := make([]*types.LocalFile, 0)
	for _, f := range localFiles {
		if f.IsDirectory {
			localFolders = append(localFolders, f)
		}
	}
	sort.Slice(localFolders, func(i, j int) bool {
		return strings.Count(localFolders[i].Path, string(os.PathSeparator)) < strings.Count(localFolders[j].Path, string(os.PathSeparator))
	})

	for _, folder := range localFolders {
		if _, exists := remoteFiles[folder.Path]; !exists {
			if _, inMetadata := metadata[folder.Path]; inMetadata {
				logger.Info("Folder deleted remotely. Deleting locally.", "path", folder.Path)
				fullPath := filepath.Join(resolvedLocalPath, folder.Path)
				if err := os.RemoveAll(fullPath); err != nil {
					logger.Error("Failed to delete local folder", "path", folder.Path, "error", err)
				} else {
					delete(metadata, folder.Path)
					prefix := folder.Path + string(os.PathSeparator)
					for k := range metadata {
						if strings.HasPrefix(k, prefix) {
							delete(metadata, k)
						}
					}
				}
				continue
			}

			parentPath := filepath.Dir(folder.Path)
			parentFolderID := cfg.RemoteFolderID
			if parentPath != "." {
				if parent, ok := remoteFiles[parentPath]; ok {
					parentFolderID = parent.ID
				} else {
					logger.Warn("Skipping folder creation, parent not found", "folder", folder.Path)
					continue
				}
			}

			logger.Info("Creating remote folder", "path", folder.Path)
			id, err := driveService.CreateFolder(parentFolderID, filepath.Base(folder.Path))
			if err != nil {
				logger.Error("Failed to create remote folder", "path", folder.Path, "error", err)
				continue
			}

			remoteFiles[folder.Path] = &types.DriveFile{
				ID:           id,
				Name:         filepath.Base(folder.Path),
				Path:         folder.Path,
				ModifiedTime: time.Now(),
				IsDirectory:  true,
			}
			metadata[folder.Path] = &types.FileMetadata{}
		}
	}

	// Determine All Sync Tasks
	allPaths := make(map[string]struct{})
	for k := range localFiles {
		allPaths[k] = struct{}{}
	}
	for k := range remoteFiles {
		allPaths[k] = struct{}{}
	}

	var tasks []types.SyncTask

	for pathStr := range allPaths {
		local := localFiles[pathStr]
		remote := remoteFiles[pathStr]

		// Check Ignore Patterns AGAIN

		ignored := false
		for _, re := range cfg.IgnoreRegexps {
			if re.MatchString(pathStr) {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}

		// -----------------------------------

		isDir := (local != nil && local.IsDirectory) || (remote != nil && remote.IsDirectory)

		if isDir {
			if local == nil && remote != nil {
				if _, inMetadata := metadata[pathStr]; inMetadata {
					tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteRemote, FilePath: pathStr})
				} else {
					if _, inMetadata := metadata[pathStr]; inMetadata {
						tasks = append(tasks, types.SyncTask{Action: types.ActionDeleteRemote, FilePath: pathStr})
					}
				}
			}
			continue
		}

		action := DetermineSyncAction(pathStr, local, remote, metadata)
		if action != types.ActionSkipNoChange && action != types.ActionSkipIdentical {
			tasks = append(tasks, types.SyncTask{Action: action, FilePath: pathStr})
		}
	}

	// Execute All Sync Tasks
	if len(tasks) == 0 {
		logger.Info("All files are up to date.")
	} else {
		logger.Info(fmt.Sprintf("Executing %d sync tasks...", len(tasks)))

		for i, task := range tasks {
			localFilePath := filepath.Join(resolvedLocalPath, task.FilePath)
			remoteFile := remoteFiles[task.FilePath]

			logger.Info(fmt.Sprintf("[%d/%d] %s: %s", i+1, len(tasks), task.Action.String(), task.FilePath))

			switch task.Action {
		case types.ActionDownloadNew, types.ActionDownloadUpdate:
			if remoteFile != nil && remoteFile.ID != "" {
				if err := os.MkdirAll(filepath.Dir(localFilePath), 0755); err != nil {
					logger.Error("Failed to create dir", "path", filepath.Dir(localFilePath), "error", err)
					continue
				}
				logger.Info("Downloading file from Google Drive", "path", task.FilePath)
				if err := driveService.DownloadFile(remoteFile.ID, localFilePath); err != nil {
					logger.Error("Failed to download file", "path", task.FilePath, "error", err)
					continue
				}
			metadata[task.FilePath] = &types.FileMetadata{RemoteMD5Checksum: remoteFile.MD5Checksum}
		} else {
			logger.Warn("Skipping download, remote ID missing", "path", task.FilePath)
		}

		case types.ActionUploadNew, types.ActionUploadUpdate, types.ActionUploadConflict:
			parentPath := filepath.Dir(task.FilePath)
			parentFolderID := cfg.RemoteFolderID
			if parentPath != "." {
				if parent, ok := remoteFiles[parentPath]; ok {
					parentFolderID = parent.ID
				} else {
					logger.Error("Could not find remote parent folder", "path", task.FilePath)
					continue
				}
			}

			remoteInfo := struct {
				Name     string
				FolderID string
				FileID   string
			}{
				Name:     filepath.Base(task.FilePath),
				FolderID: parentFolderID,
			}
			if remoteFile != nil {
				remoteInfo.FileID = remoteFile.ID
			}

			logger.Info("Uploading file to Google Drive", "path", task.FilePath)
			uploadedFile, err := driveService.UploadOrUpdateFile(localFilePath, remoteInfo)
			if err != nil {
				logger.Error("Failed to upload/update file", "path", task.FilePath, "error", err)
				continue
			}
			metadata[task.FilePath] = &types.FileMetadata{RemoteMD5Checksum: uploadedFile.Md5Checksum}

			case types.ActionDeleteLocal:
				if err := os.Remove(localFilePath); err != nil {
					if !os.IsNotExist(err) {
						logger.Error("Failed to delete local file", "path", task.FilePath, "error", err)
						continue
					}
				}
				delete(metadata, task.FilePath)

			case types.ActionDeleteRemote:
				if remoteFile != nil && remoteFile.ID != "" {
					if err := driveService.TrashRemoteFile(remoteFile.ID); err != nil {
						logger.Error("Failed to trash remote file", "path", task.FilePath, "error", err)
						continue
					}
					delete(metadata, task.FilePath)
					delete(remoteFiles, task.FilePath)

					if remoteFile.IsDirectory {
						prefix := task.FilePath + string(os.PathSeparator)
						for k := range remoteFiles {
							if strings.HasPrefix(k, prefix) {
								delete(remoteFiles, k)
								delete(metadata, k)
							}
						}
					}
				} else {
					logger.Warn("Skipping remote deletion, ID missing", "path", task.FilePath)
				}
			}
		}
	}

	// Save Metadata
	var entries [][2]interface{}
	for k, v := range metadata {
		entries = append(entries, [2]interface{}{k, v})
	}

	metaDataBytes, err := json.Marshal(entries)
	if err != nil {
		logger.Error("Error marshaling sync metadata", "error", err)
	} else {
		if err := os.WriteFile(metadataPath, metaDataBytes, 0644); err != nil {
			logger.Error("Error saving sync metadata", "error", err)
		} else {
			logger.Info("Sync metadata saved.")
		}
	}

	logger.Info("Sync cycle finished.")
	logger.Info("--------------------------------------------------")
	return nil
}

// applyChanges updates the remoteFiles map based on the changes list.
func applyChanges(changes []*drive.Change, remoteFiles types.DriveFileMap, rootFolderID string, ignoreRegexps []*regexp.Regexp) {
	idToPath := make(map[string]string)
	for path, file := range remoteFiles {
		idToPath[file.ID] = path
	}

	for _, change := range changes {
		if change.Removed || (change.File != nil && change.File.Trashed) {
			path, exists := idToPath[change.FileId]
			if exists {
				logger.Debug("Applying remote deletion", "path", path)
				delete(remoteFiles, path)
				delete(idToPath, change.FileId)

				prefix := path + string(os.PathSeparator)
				for k := range remoteFiles {
					if strings.HasPrefix(k, prefix) {
						delete(remoteFiles, k)
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
					if oldPath, exists := idToPath[change.FileId]; exists {
						logger.Debug("File moved out of scope", "oldPath", oldPath)
						delete(remoteFiles, oldPath)
						delete(idToPath, change.FileId)
						prefix := oldPath + string(os.PathSeparator)
						for k := range remoteFiles {
							if strings.HasPrefix(k, prefix) {
								delete(remoteFiles, k)
							}
						}
					}
					continue
				}
				parentPath = pPath
			}

			newPath := filepath.Join(parentPath, file.Name)

			// Check Ignore Patterns
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
					delete(idToPath, change.FileId)
				}
				continue
			}

			// -----------------------------

			oldPath, exists := idToPath[change.FileId]

			isDirectory := file.MimeType == "application/vnd.google-apps.folder"
			modTime, _ := time.Parse(time.RFC3339, file.ModifiedTime)

			if exists && oldPath != newPath {
				logger.Debug("Applying remote move", "old", oldPath, "new", newPath)
				delete(remoteFiles, oldPath)

				if isDirectory {
					oldPrefix := oldPath + string(os.PathSeparator)
					newPrefix := newPath + string(os.PathSeparator)

					type childMove struct {
						oldK string
						newK string
						val  *types.DriveFile
					}
					var moves []childMove

					for k, v := range remoteFiles {
						if strings.HasPrefix(k, oldPrefix) {
							suffix := strings.TrimPrefix(k, oldPrefix)
							newK := newPrefix + suffix
							moves = append(moves, childMove{k, newK, v})
						}
					}

					for _, m := range moves {
						delete(remoteFiles, m.oldK)
						m.val.Path = m.newK
						remoteFiles[m.newK] = m.val
						idToPath[m.val.ID] = m.newK
					}
				}
			}

			remoteFiles[newPath] = &types.DriveFile{
				ID:           file.Id,
				Name:         file.Name,
				Path:         newPath,
				ModifiedTime: modTime,
				MD5Checksum:  file.Md5Checksum,
				IsDirectory:  isDirectory,
			}
			idToPath[file.Id] = newPath
		}
	}
}
