package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"gdrive-bisync/internal/services/logger"
)

var (
	DefaultConfig = Config{
		LocalSyncPath:          "~/GoogleDrive",
		RemoteFolderID:         "root",
		MetadataFileName:       ".gdrive-bisync-metadata.json",
		StateFileName:          ".gdrive-bisync-state.json",
		WatchDebounceDelay:     5000,
		PeriodicSyncIntervalMs: 1 * 60 * 1000, // 1 minute
		MaxConcurrentScans:     20,
		MaxRetries:             10,
		Ignore:                 []string{`(^|.*[\\/])node_modules([\\/].*|$)`},
	}
	ConfigPath = filepath.Join("config", "config.json")
)

type Config struct {
	Ignore                 []string `json:"ignore"`
	LocalSyncPath          string   `json:"LOCAL_SYNC_PATH"`
	RemoteFolderID         string   `json:"REMOTE_FOLDER_ID"`
	MetadataFileName       string   `json:"METADATA_FILE_NAME"`
	StateFileName          string   `json:"STATE_FILE_NAME"`
	WatchDebounceDelay     int      `json:"WATCH_DEBOUNCE_DELAY"`
	PeriodicSyncIntervalMs int      `json:"PERIODIC_SYNC_INTERVAL_MS"`
	MaxConcurrentScans     int      `json:"MAX_CONCURRENT_SCANS"`
	MaxRetries             int      `json:"MAX_RETRIES"`
	IgnoreRegexps          []*regexp.Regexp
}

func Load() (*Config, error) {
	config := DefaultConfig

	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		logger.Warn("No config/config.json found or error reading config. Using default settings.", "error", err)
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			logger.Error("Error parsing config file", "error", err)
			return &config, err
		}
	}

	if config.MetadataFileName != "" {
		escaped := regexp.QuoteMeta(config.MetadataFileName)
		config.Ignore = append(config.Ignore, "^"+escaped+"$")
	}
	if config.StateFileName != "" {
		escaped := regexp.QuoteMeta(config.StateFileName)
		config.Ignore = append(config.Ignore, "^"+escaped+"$")
	}

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

	return &config, nil
}
