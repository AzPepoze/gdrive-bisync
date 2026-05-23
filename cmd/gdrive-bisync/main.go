package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/pflag"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/core"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/services/systemd"
	"gdrive-bisync/internal/store"
	"gdrive-bisync/internal/types"
	"gdrive-bisync/internal/utils"
)

func main() {
	setupFlag := pflag.BoolP("setup", "s", false, "Run authentication setup")
	forceFlag := pflag.BoolP("force", "f", false, "Force a fresh sync by deleting the database")
	installServiceFlag := pflag.Bool("install-service", false, "Install systemd user service (Linux only)")
	uninstallServiceFlag := pflag.Bool("uninstall-service", false, "Uninstall systemd user service (Linux only)")
	showLogsFlag := pflag.BoolP("show-logs", "l", false, "Enable logging output to console")
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

	showLogs := *showLogsFlag || cfg.ShowLogs
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

	if err := os.MkdirAll(resolvedLocalPath, 0755); err != nil {
		logger.Error("Failed to create local directory", "error", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(resolvedLocalPath, cfg.DBFileName)

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

	runSync := func() {
		token := sharedState.GetPageToken()
		sharedState.RunExclusive(func(remoteFilesMap types.DriveFileMap, metadataMap map[string]*types.FileMetadata) {
			if err := core.Sync(driveService, remoteFilesMap, metadataMap, cfg, &token, dbStore, sharedState); err != nil {
				logger.Error("Sync failed", "error", err)
			}
		})
		sharedState.SetPageToken(token)
	}

	runSync()

	go core.WatchLocalFiles(resolvedLocalPath, driveService, sharedState, cfg, dbStore)

	ticker := time.NewTicker(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	nextSyncTime := time.Now().Add(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
	logger.Info("Application started. Press Ctrl+C to stop.")
	logger.Info("Next periodic sync scheduled", "time", nextSyncTime.Format("2006-01-02 15:04:05"))

	go func() {
		for range ticker.C {
			logger.Info("Triggering periodic sync...")
			runSync()
			nextSyncTime = time.Now().Add(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
			logger.Info("Next periodic sync scheduled", "time", nextSyncTime.Format("2006-01-02 15:04:05"))
		}
	}()

	<-sigs
	logger.Info("Shutting down...")
}
