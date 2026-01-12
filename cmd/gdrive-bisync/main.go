package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gdrive-bisync/internal/api"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/core"
	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/services/systemd"
	"gdrive-bisync/internal/types"
	"gdrive-bisync/internal/utils"
)

func main() {
	setupFlag := flag.Bool("setup", false, "Run authentication setup")
	forceFlag := flag.Bool("force", false, "Force a fresh sync by deleting metadata and state files")
	installServiceFlag := flag.Bool("install-service", false, "Install systemd user service (Linux only)")
	uninstallServiceFlag := flag.Bool("uninstall-service", false, "Uninstall systemd user service (Linux only)")
	flag.Parse()

	logger.Init()
	defer logger.Close()

	// Handle service installation
	if *installServiceFlag {
		if err := systemd.InstallService(); err != nil {
			logger.Error("Failed to install service", "error", err)
			os.Exit(1)
		}
		return
	}

	// Handle service uninstallation
	if *uninstallServiceFlag {
		if err := systemd.UninstallService(); err != nil {
			logger.Error("Failed to uninstall service", "error", err)
			os.Exit(1)
		}
		return
	}

	if *setupFlag {
		if err := api.SetupAuthentication(context.Background()); err != nil {
			logger.Error("Setup failed", "error", err)
			os.Exit(1)
		}
		logger.Info("Setup complete. You can now run the application without flags.")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
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

	// Force Reset Logic
	statePath := filepath.Join(resolvedLocalPath, cfg.StateFileName)
	metadataPath := filepath.Join(resolvedLocalPath, cfg.MetadataFileName)

	if *forceFlag {
		logger.Info("Force flag detected. Resetting state...", "state", statePath, "metadata", metadataPath)
		if err := os.Remove(statePath); err != nil {
			if !os.IsNotExist(err) {
				logger.Warn("Failed to remove state file", "error", err)
			}
		} else {
			logger.Info("Removed state file.")
		}

		if err := os.Remove(metadataPath); err != nil {
			if !os.IsNotExist(err) {
				logger.Warn("Failed to remove metadata file", "error", err)
			}
		} else {
			logger.Info("Removed metadata file.")
		}
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

	// Load State
	state, err := core.LoadState(statePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Failed to load state", "error", err)
		} else {
			logger.Info("No previous state found. Starting fresh.")
		}

		// Initialize empty state
		state = &core.State{
			RemoteFiles: make(types.DriveFileMap),
		}
	} else {
		logger.Info("Loaded previous state.", "files", len(state.RemoteFiles), "token", state.PageToken)
	}

	metadata := make(map[string]*types.FileMetadata)

	// Helper to save state
	saveState := func() {
		if err := core.SaveState(statePath, state); err != nil {
			logger.Error("Failed to save state", "error", err)
		} else {
			logger.Debug("State saved.")
		}
	}

	// Initial Sync
	if err := core.Sync(driveService, state.RemoteFiles, metadata, cfg, &state.PageToken); err != nil {
		logger.Error("Initial sync failed", "error", err)
	}
	saveState()

	// Watcher
	go core.WatchLocalFiles(resolvedLocalPath, driveService, state.RemoteFiles, metadata, cfg)

	// Periodic Sync
	ticker := time.NewTicker(time.Duration(cfg.PeriodicSyncIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	// Handle graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Application started. Press Ctrl+C to stop.")

	go func() {
		for range ticker.C {
			logger.Info("Triggering periodic sync...")
			if err := core.Sync(driveService, state.RemoteFiles, metadata, cfg, &state.PageToken); err != nil {
				logger.Error("Periodic sync failed", "error", err)
			}
			saveState()
		}
	}()

	<-sigs
	logger.Info("Shutting down...")
	saveState()
}
