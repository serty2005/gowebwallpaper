package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeConfigAcceptsDebugLogMode(t *testing.T) {
	config := &AppConfig{Log: " DEBUG "}

	normalizeConfig(config)

	if config.Log != logLevelDebug {
		t.Fatalf("expected debug log level, got %q", config.Log)
	}
}

func TestNormalizeConfigClearsUnknownLogMode(t *testing.T) {
	config := &AppConfig{Log: "trace"}

	normalizeConfig(config)

	if config.Log != "" {
		t.Fatalf("expected unknown log level to be cleared, got %q", config.Log)
	}
}

func TestLoadConfigReadsLowercaseDebugLogMode(t *testing.T) {
	dir := t.TempDir()
	configPathOverride = filepath.Join(dir, "config.json")
	t.Cleanup(func() {
		configPathOverride = ""
	})
	if err := os.WriteFile(configPathOverride, []byte(`{"URL":"https://example.test","log":"debug"}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, err := loadConfig()

	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}
	if config.Log != logLevelDebug {
		t.Fatalf("expected debug log level, got %q", config.Log)
	}
}
