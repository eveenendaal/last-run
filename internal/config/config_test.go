package config

import (
	"os"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if cfg.DBPath != "" {
		t.Errorf("expected empty DBPath, got %q", cfg.DBPath)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := "/custom/path/data.db"
	if err := Save(&Config{DBPath: want}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBPath != want {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, want)
	}
}

func TestSaveEmptyReverts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Write a path, then clear it.
	if err := Save(&Config{DBPath: "/some/path.db"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(&Config{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load after clear: %v", err)
	}
	if cfg.DBPath != "" {
		t.Errorf("expected empty DBPath after clear, got %q", cfg.DBPath)
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := Save(&Config{DBPath: "/x"}); err != nil {
		t.Fatal(err)
	}
	path, _ := configPath()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}
