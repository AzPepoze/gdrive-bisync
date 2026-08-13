package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gdrive-bisync/internal/services/logger"
)

var (
	DefaultConfig = Config{
		LocalSyncPath:          "~/GoogleDrive",
		RemoteFolderID:         "root",
		DBFileName:             ".gdrive-bisync.db",
		MetadataFileName:       ".gdrive-bisync-metadata.json",
		StateFileName:          ".gdrive-bisync-state.json",
		WatchDebounceDelay:     5000,
		PeriodicSyncIntervalMs: 1 * 60 * 1000,
		MaxConcurrentScans:     20,
		MaxConcurrentDownloads: 20,
		MaxConcurrentUploads:   10,
		MaxRetries:             10,
		MaxDeletionsPerSync:    20,
		MaxDeletionPercent:     5,
		DatabaseBackupCount:    5,
		ShowLogs:               false,
		Ignore:                 []string{`(^|.*[\\/])node_modules([\\/].*|$)`},
	}
)

type Config struct {
	Ignore                 []string `json:"ignore"`
	LocalSyncPath          string   `json:"LOCAL_SYNC_PATH"`
	RemoteFolderID         string   `json:"REMOTE_FOLDER_ID"`
	DBFileName             string   `json:"DB_FILE_NAME"`
	MetadataFileName       string   `json:"METADATA_FILE_NAME"`
	StateFileName          string   `json:"STATE_FILE_NAME"`
	WatchDebounceDelay     int      `json:"WATCH_DEBOUNCE_DELAY"`
	PeriodicSyncIntervalMs int      `json:"PERIODIC_SYNC_INTERVAL_MS"`
	MaxConcurrentScans     int      `json:"MAX_CONCURRENT_SCANS"`
	MaxConcurrentDownloads int      `json:"MAX_CONCURRENT_DOWNLOADS"`
	MaxConcurrentUploads   int      `json:"MAX_CONCURRENT_UPLOADS"`
	MaxRetries             int      `json:"MAX_RETRIES"`
	MaxDeletionsPerSync    int      `json:"MAX_DELETIONS_PER_SYNC"`
	MaxDeletionPercent     float64  `json:"MAX_DELETION_PERCENT"`
	DatabaseBackupCount    int      `json:"DATABASE_BACKUP_COUNT"`
	ShowLogs               bool     `json:"SHOW_LOGS"`
	IgnoreRegexps          []*regexp.Regexp
}

func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, ".config", "gdrive-bisync", "config")
		if info, err := os.Stat(userPath); err == nil && info.IsDir() {
			return userPath
		}
	}
	return "config"
}

func Load() (*Config, error) {
	config := DefaultConfig
	path := filepath.Join(GetConfigDir(), "config.json")

	data, err := os.ReadFile(path)
	if err != nil {
		logger.Warn("Config file not found or error reading it. Using default settings.", "path", path, "error", err)
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			logger.Error("Error parsing config file", "path", path, "error", err)
			return &config, err
		}
	}

	if config.DBFileName != "" {
		escaped := regexp.QuoteMeta(config.DBFileName)
		config.Ignore = append(config.Ignore, "^"+escaped+"$")
	}
	if config.MetadataFileName != "" {
		escaped := regexp.QuoteMeta(config.MetadataFileName)
		config.Ignore = append(config.Ignore, "^"+escaped+"$")
	}
	if config.StateFileName != "" {
		escaped := regexp.QuoteMeta(config.StateFileName)
		config.Ignore = append(config.Ignore, "^"+escaped+"$")
	}

	config.Ignore = append(config.Ignore, `(^|.*[\\/])\.trash([\\/].*|$)`)
	config.Ignore = append(config.Ignore, `(^|.*[\\/])\.gdrive-bisync-backups([\\/].*|$)`)
	config.Ignore = append(config.Ignore, `(^|.*[\\/])\.gdrive-download-.*\.partial$`)

	logger.Info("Final Ignore Patterns", "count", len(config.Ignore), "list", config.Ignore)

	config.IgnoreRegexps = make([]*regexp.Regexp, 0, len(config.Ignore))
	for _, pattern := range config.Ignore {
		re, err := regexp.Compile(pattern)
		if err != nil {
			logger.Warn("Invalid ignore pattern", "pattern", pattern, "error", err)
			continue
		}
		config.IgnoreRegexps = append(config.IgnoreRegexps, re)
	}
	if err := config.Validate(); err != nil {
		return &config, err
	}

	return &config, nil
}

func (config Config) Validate() error {
	if config.WatchDebounceDelay < 0 {
		return fmt.Errorf("WATCH_DEBOUNCE_DELAY must not be negative")
	}
	if config.PeriodicSyncIntervalMs <= 0 {
		return fmt.Errorf("PERIODIC_SYNC_INTERVAL_MS must be greater than zero")
	}
	if config.MaxConcurrentScans <= 0 || config.MaxConcurrentDownloads <= 0 || config.MaxConcurrentUploads <= 0 {
		return fmt.Errorf("concurrency limits must be greater than zero")
	}
	if config.MaxRetries <= 0 {
		return fmt.Errorf("MAX_RETRIES must be greater than zero")
	}
	if config.MaxDeletionsPerSync < 0 || config.MaxDeletionPercent < 0 || config.MaxDeletionPercent > 100 {
		return fmt.Errorf("deletion thresholds must be non-negative and percentage must not exceed 100")
	}
	if config.DatabaseBackupCount < 0 {
		return fmt.Errorf("DATABASE_BACKUP_COUNT must not be negative")
	}
	return nil
}
