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
	if *tuiFlag {
		if err := tui.RunTerminal(runtimePaths, resolvedLocalPath); err != nil {
			logger.Error("TUI failed", "error", err)
			os.Exit(1)
		}
		return
	}

	instanceLock, err := appstate.AcquireInstanceLock(runtimePaths.LockFile)
	if err != nil {
		logger.Error("Another gdrive-bisync process is already running", "error", err)
		os.Exit(2)
	}
	defer instanceLock.Close()

	dbPath := filepath.Join(resolvedLocalPath, cfg.DBFileName)
	if !*dryRunFlag {
		if backupPath, err := store.BackupDatabase(dbPath, cfg.DatabaseBackupCount); err != nil {
			logger.Error("Failed to back up database", "error", err)
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
		os.Remove(legacyStatePath)
		os.Remove(legacyMetaPath)
	}

	authClient, err := api.Authorize(context.Background())
	if err != nil {
		logger.Error("Authentication failed", "error", err)
		logger.Info("Please run with --setup to configure authentication.")
		os.Exit(1)
	}

	driveService, err := api.NewDriveService(authClient)
	if err != nil {
		logger.Error("Failed to create Drive service", "error", err)
		os.Exit(1)
	}

	dbStore, err := store.Open(dbPath)
	if err != nil {
		logger.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer dbStore.Close()

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
	runtimeStatus := appstate.Status{PID: os.Getpid(), State: "starting", StartedAt: time.Now()}
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

	runSync := func() {
		if appstate.IsPaused(runtimePaths.PauseFile) && !*dryRunFlag {
			writeStatus(func(status *appstate.Status) { status.State = "paused" })
			return
		}
		writeStatus(func(status *appstate.Status) {
			status.State = "syncing"
			status.LastSyncStarted = time.Now()
			status.LastError = ""
		})
		token := sharedState.GetPageToken()
		var syncErr error
		sharedState.RunMutation(func() {
			sharedState.RunExclusive(func(remoteFilesMap types.DriveFileMap, metadataMap map[string]*types.FileMetadata) {
				if err := core.Sync(driveService, remoteFilesMap, metadataMap, cfg, &token, dbStore, sharedState, core.SyncOptions{
					DryRun:             *dryRunFlag,
					AllowUnsafeDeletes: *unsafeDeletesFlag,
					OnPlan: func(taskCount int) {
						writeStatus(func(status *appstate.Status) { status.TaskCount = taskCount })
					},
				}); err != nil {
					syncErr = err
					logger.Error("Sync failed", "error", err)
					writeStatus(func(status *appstate.Status) { status.LastError = err.Error() })
				}
			})
		})
		sharedState.SetPageToken(token)
		writeStatus(func(status *appstate.Status) {
			if syncErr != nil {
				status.State = "error"
			} else {
				status.State = "idle"
			}
			status.LastSyncFinished = time.Now()
		})
	}

	runSync()
	if *dryRunFlag {
		writeStatus(func(status *appstate.Status) { status.State = "stopped" })
		return
	}

	go core.WatchLocalFiles(resolvedLocalPath, driveService, sharedState, cfg, dbStore, runtimePaths.PauseFile)

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
			runSync()
			nextSyncTime = time.Now().Add(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
			writeStatus(func(status *appstate.Status) { status.NextSync = nextSyncTime })
			logger.Info("Next periodic sync scheduled", "time", nextSyncTime.Format("2006-01-02 15:04:05"))
		}
	}()

	<-sigs
	writeStatus(func(status *appstate.Status) { status.State = "stopped" })
	logger.Info("Shutting down...")
}
