package config

import "testing"

func TestValidateRejectsUnsafeRuntimeValues(t *testing.T) {
	config := DefaultConfig
	config.MaxConcurrentDownloads = 0
	if err := config.Validate(); err == nil {
		t.Fatal("expected zero download concurrency to fail validation")
	}

	config = DefaultConfig
	config.MaxDeletionPercent = 101
	if err := config.Validate(); err == nil {
		t.Fatal("expected deletion percentage above 100 to fail validation")
	}
}

func TestLoadAddsLocalSafetyFilesToIgnoreSet(t *testing.T) {
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		".gdrive-bisync-backups/state.bak",
		"folder/.gdrive-download-123.partial",
	} {
		matched := false
		for _, expression := range config.IgnoreRegexps {
			if expression.MatchString(path) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("expected %q to be ignored", path)
		}
	}
}
