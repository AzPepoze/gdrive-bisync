package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/pflag"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/appstate"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/core"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/services/notifier"
	"gdrive-bisync/internal/services/systemd"
	"gdrive-bisync/internal/store"
	"gdrive-bisync/internal/tui"
	"gdrive-bisync/internal/types"
	"gdrive-bisync/internal/utils"
)

func main() {
	setupFlag := pflag.BoolP("setup", "s", false, "Run authentication setup")
	forceFlag := pflag.BoolP("force", "f", false, "Force a fresh sync by deleting the database")
	installServiceFlag := pflag.Bool("install-service", false, "Install systemd user service (Linux only)")
	uninstallServiceFlag := pflag.Bool("uninstall-service", false, "Uninstall systemd user service (Linux only)")
	showLogsFlag := pflag.BoolP("show-logs", "l", false, "Enable logging output to console")
	dryRunFlag := pflag.Bool("dry-run", false, "Plan and print sync actions without changing files")
	unsafeDeletesFlag := pflag.Bool("allow-unsafe-deletes", false, "Override configured deletion safety thresholds")
	statusFlag := pflag.Bool("status", false, "Show runtime status")
	pauseFlag := pflag.Bool("pause", false, "Pause periodic sync and watcher uploads")
	resumeFlag := pflag.Bool("resume", false, "Resume syncing")
	trashListFlag := pflag.Bool("trash-list", false, "List recoverable local trash entries")
	trashRestoreFlag := pflag.String("trash-restore", "", "Restore a local trash entry by ID")
	tuiFlag := pflag.Bool("tui", false, "Open the terminal management interface")
	pflag.Parse()

	if *installServiceFlag {
		logger.Init(true)
		defer logger.Close()
		if err := systemd.InstallService(); err != nil {
			logger.Error("Failed to install service", "error", err)
			os.Exit(1)
		}
		return
	}

	if *uninstallServiceFlag {
		logger.Init(true)
		defer logger.Close()
		if err := systemd.UninstallService(); err != nil {
			logger.Error("Failed to uninstall service", "error", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Init(true)
		defer logger.Close()
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	showLogs := *showLogsFlag || cfg.ShowLogs || *dryRunFlag
	logger.Init(showLogs)
	defer logger.Close()
	desktopNotifier := notifier.New(cfg.DesktopNotifications, time.Duration(cfg.NotificationCooldownMs)*time.Millisecond)

	if *setupFlag {
		if err := api.SetupAuthentication(context.Background()); err != nil {
			logger.Error("Setup failed", "error", err)
			os.Exit(1)
		}
		logger.Info("Setup complete. You can now run the application without flags.")
		return
	}

	resolvedLocalPath := utils.ResolvePath(cfg.LocalSyncPath)
	if resolvedLocalPath == "" || cfg.RemoteFolderID == "" {
		logger.Error("Error: LOCAL_SYNC_PATH and REMOTE_FOLDER_ID must be configured.")
		os.Exit(1)
	}

	if !*dryRunFlag {
		if err := os.MkdirAll(resolvedLocalPath, 0755); err != nil {
			logger.Error("Failed to create local directory", "error", err)
			os.Exit(1)
		}
	}

	runtimePaths, err := appstate.DefaultPaths()
	if err != nil {
		logger.Error("Failed to initialize runtime directory", "error", err)
		os.Exit(1)
	}
	if err := runtimePaths.EnsureDirectory(); err != nil {
		logger.Error("Failed to initialize runtime directory", "error", err)
		os.Exit(1)
	}
	if shouldOpenTUI(pflag.NFlag(), stdoutIsTerminal()) || *tuiFlag {
		if err := tui.RunTerminal(runtimePaths, resolvedLocalPath, cfg); err != nil {
			logger.Error("TUI failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if *statusFlag {
		status, err := appstate.ReadStatus(runtimePaths.StatusFile)
		if err != nil {
			fmt.Println("gdrive-bisync is stopped or status is unavailable")
			return
		}
		status.Paused = appstate.IsPaused(runtimePaths.PauseFile)
		fmt.Printf("state=%s pid=%d paused=%v tasks=%d last_sync=%s error=%q\n", status.State, status.PID, status.Paused, status.TaskCount, status.LastSyncFinished.Format(time.RFC3339), status.LastError)
		return
	}
	if *pauseFlag || *resumeFlag {
		paused := *pauseFlag
		if err := appstate.SetPaused(runtimePaths.PauseFile, paused); err != nil {
			logger.Error("Failed to update pause state", "error", err)
			os.Exit(1)
		}
		fmt.Printf("sync paused=%v\n", paused)
		return
	}
	if *trashListFlag {
		entries, err := utils.ListTrash(resolvedLocalPath)
		if err != nil {
			logger.Error("Failed to list trash", "error", err)
			os.Exit(1)
		}
		for _, entry := range entries {
			fmt.Printf("%s\t%s\t%s\n", entry.ID, entry.DeletedAt.Format(time.RFC3339), entry.OriginalPath)
		}
		return
	}
	if *trashRestoreFlag != "" {
		entry, err := utils.RestoreTrash(resolvedLocalPath, *trashRestoreFlag)
		if err != nil {
			logger.Error("Failed to restore trash entry", "error", err)
			os.Exit(1)
		}
		fmt.Printf("restored %s\n", entry.OriginalPath)
		return
	}
	instanceLock, err := appstate.AcquireInstanceLock(runtimePaths.LockFile)
	if err != nil {
		logger.Error("Another gdrive-bisync process is already running", "error", err)
		os.Exit(2)
	}
	defer func() { _ = instanceLock.Close() }()
	eventJournal, eventErr := appstate.OpenEventJournal(runtimePaths.EventsFile)
	if eventErr != nil {
		logger.Warn("Failed to open runtime event journal", "error", eventErr)
	} else {
		logger.SetEventSink(func(level, category, message string, fields map[string]any) {
			_ = eventJournal.Append(appstate.Event{Time: time.Now(), Level: level, Category: category, Message: message, Fields: fields})
		})
		defer logger.SetEventSink(nil)
	}

	dbPath := filepath.Join(resolvedLocalPath, cfg.DBFileName)
	if !*dryRunFlag {
		if backupPath, err := store.BackupDatabase(dbPath, cfg.DatabaseBackupCount); err != nil {
			logger.Error("Failed to back up database", "error", err)
			desktopNotifier.Critical("database-backup", "Google Drive backup failed", err.Error())
			os.Exit(1)
		} else if backupPath != "" {
			logger.Info("Database backup created", "path", backupPath)
		}
	}

	if *forceFlag {
		logger.Info("Force flag detected. Resetting state...", "db", dbPath)
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("Failed to remove database file", "error", err)
		} else {
			logger.Info("Removed database file.")
		}
		legacyStatePath := filepath.Join(resolvedLocalPath, cfg.StateFileName)
		legacyMetaPath := filepath.Join(resolvedLocalPath, cfg.MetadataFileName)
		for _, legacyPath := range []string{legacyStatePath, legacyMetaPath} {
			if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("Failed to remove legacy state file", "path", legacyPath, "error", err)
			}
		}
	}

	authClient, err := api.Authorize(context.Background())
	if err != nil {
		logger.Error("Authentication failed", "error", err)
		desktopNotifier.Critical("authentication", "Google Drive authentication failed", err.Error())
		logger.Info("Please run with --setup to configure authentication.")
		os.Exit(1)
	}

	driveService, err := api.NewDriveService(authClient)
	if err != nil {
		logger.Error("Failed to create Drive service", "error", err)
		desktopNotifier.Critical("drive-service", "Google Drive connection failed", err.Error())
		os.Exit(1)
	}

	dbStore, err := store.Open(dbPath)
	if err != nil {
		logger.Error("Failed to open database", "error", err)
		desktopNotifier.Critical("database-open", "Google Drive sync database failed", err.Error())
		os.Exit(1)
	}
	defer func() { _ = dbStore.Close() }()

	remoteFiles, err := dbStore.LoadRemoteFiles()
	if err != nil {
		logger.Warn("Failed to load remote files from database, starting fresh", "error", err)
		remoteFiles = make(types.DriveFileMap)
	} else {
		logger.Info("Loaded remote files from database.", "files", len(remoteFiles))
	}

	metadata, err := dbStore.LoadMetadata()
	if err != nil {
		logger.Warn("Failed to load metadata from database, starting fresh", "error", err)
		metadata = make(map[string]*types.FileMetadata)
	} else {
		logger.Info("Loaded metadata from database.", "entries", len(metadata))
	}

	pageToken, err := dbStore.LoadPageToken()
	if err != nil {
		logger.Warn("Failed to load page token", "error", err)
	}

	sharedState := core.NewSharedState(remoteFiles, metadata, pageToken)
	runtimeStatus := appstate.Status{PID: os.Getpid(), State: "starting", StartedAt: time.Now(), WatcherHealthy: true, Notifications: cfg.DesktopNotifications}
	var statusMu sync.Mutex
	writeStatus := func(update func(*appstate.Status)) {
		statusMu.Lock()
		defer statusMu.Unlock()
		update(&runtimeStatus)
		runtimeStatus.Paused = appstate.IsPaused(runtimePaths.PauseFile)
		if err := appstate.WriteStatus(runtimePaths.StatusFile, runtimeStatus); err != nil {
			logger.Warn("Failed to write runtime status", "error", err)
		}
	}
	writeStatus(func(status *appstate.Status) {})

	var syncRunMu sync.Mutex
	runSync := func(requestDryRun bool) {
		syncRunMu.Lock()
		defer syncRunMu.Unlock()
		effectiveDryRun := *dryRunFlag || requestDryRun
		if appstate.IsPaused(runtimePaths.PauseFile) && !effectiveDryRun {
			writeStatus(func(status *appstate.Status) { status.State = "paused" })
			return
		}
		writeStatus(func(status *appstate.Status) {
			status.State = "syncing"
			status.LastSyncStarted = time.Now()
			status.LastError = ""
			status.CompletedTasks = 0
			status.FailedTasks = 0
			status.CurrentOperation = "Planning sync"
			status.DryRun = effectiveDryRun
		})
		token := sharedState.GetPageToken()
		var syncErr error
		sharedState.RunMutation(func() {
			sharedState.RunExclusive(func(remoteFilesMap types.DriveFileMap, metadataMap map[string]*types.FileMetadata) {
				if err := core.Sync(driveService, remoteFilesMap, metadataMap, cfg, &token, dbStore, sharedState, core.SyncOptions{
					DryRun:             effectiveDryRun,
					AllowUnsafeDeletes: *unsafeDeletesFlag,
					OnPlan: func(taskCount int) {
						writeStatus(func(status *appstate.Status) { status.TaskCount = taskCount })
					},
					OnInventory: func(localItems, remoteItems int) {
						writeStatus(func(status *appstate.Status) { status.LocalItems = localItems; status.RemoteItems = remoteItems })
					},
					OnTasks: func(tasks []types.SyncTask) {
						writeStatus(func(status *appstate.Status) {
							status.Uploads, status.Downloads, status.Deletions, status.Folders = 0, 0, 0, 0
							for _, task := range tasks {
								switch task.Action {
								case types.ActionUploadNew, types.ActionUploadUpdate, types.ActionUploadConflict:
									status.Uploads++
								case types.ActionDownloadNew, types.ActionDownloadUpdate:
									status.Downloads++
								case types.ActionDeleteLocal, types.ActionDeleteRemote:
									status.Deletions++
								case types.ActionCreateLocalFolder:
									status.Folders++
								}
							}
						})
					},
					OnTaskComplete: func(task types.SyncTask, taskErr error) {
						writeStatus(func(status *appstate.Status) {
							status.CompletedTasks++
							status.CurrentOperation = task.Action.String()
							if taskErr != nil {
								status.FailedTasks++
							}
						})
					},
				}); err != nil {
					syncErr = err
					logger.Error("Sync failed", "error", err)
					writeStatus(func(status *appstate.Status) { status.LastError = err.Error() })
				}
			})
		})
		if !effectiveDryRun {
			sharedState.SetPageToken(token)
		}
		writeStatus(func(status *appstate.Status) {
			if syncErr != nil {
				status.State = "error"
			} else {
				status.State = "idle"
			}
			status.LastSyncFinished = time.Now()
			if syncErr == nil {
				status.CompletedTasks = status.TaskCount
			}
			status.CurrentOperation = ""
			status.DryRun = false
			if syncErr != nil && status.FailedTasks == 0 {
				status.FailedTasks = 1
			}
		})
		if syncErr != nil {
			desktopNotifier.Critical("sync", "Google Drive sync failed", syncErr.Error())
		} else {
			desktopNotifier.Recovered("sync", "Files are syncing normally again.")
		}
	}

	runSync(false)
	if *dryRunFlag {
		writeStatus(func(status *appstate.Status) { status.State = "stopped" })
		return
	}

	go core.WatchLocalFiles(resolvedLocalPath, driveService, sharedState, cfg, dbStore, runtimePaths.PauseFile, func(err error) {
		writeStatus(func(status *appstate.Status) { status.WatcherHealthy = false })
		desktopNotifier.Critical("watcher", "Google Drive live sync failed", err.Error())
	})

	ticker := time.NewTicker(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	nextSyncTime := time.Now().Add(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
	writeStatus(func(status *appstate.Status) {
		status.State = "idle"
		status.NextSync = nextSyncTime
	})
	logger.Info("Application started. Press Ctrl+C to stop.")
	logger.Info("Next periodic sync scheduled", "time", nextSyncTime.Format("2006-01-02 15:04:05"))

	go func() {
		for range ticker.C {
			logger.Info("Triggering periodic sync...")
			runSync(false)
			nextSyncTime = time.Now().Add(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
			writeStatus(func(status *appstate.Status) { status.NextSync = nextSyncTime })
			logger.Info("Next periodic sync scheduled", "time", nextSyncTime.Format("2006-01-02 15:04:05"))
		}
	}()

	controlTicker := time.NewTicker(time.Second)
	defer controlTicker.Stop()
	go func() {
		for range controlTicker.C {
			if appstate.ConsumeRequest(runtimePaths.SyncNowFile) {
				logger.Info("Manual sync requested from TUI")
				runSync(false)
			}
			if appstate.ConsumeRequest(runtimePaths.DryRunFile) {
				logger.Info("Dry-run preview requested from TUI")
				runSync(true)
			}
		}
	}()

	<-sigs
	writeStatus(func(status *appstate.Status) { status.State = "stopped" })
	logger.Info("Shutting down...")
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func shouldOpenTUI(explicitFlagCount int, terminal bool) bool {
	return explicitFlagCount == 0 && terminal
}
