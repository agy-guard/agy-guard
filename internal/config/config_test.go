package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != "gemini" {
		t.Errorf("expected provider gemini, got %q", cfg.Provider)
	}
	if cfg.Model != "gemini-3.5-flash-lite" {
		t.Errorf("expected model gemini-3.5-flash-lite, got %q", cfg.Model)
	}
	if cfg.TimeoutSec <= 0 {
		t.Errorf("expected positive timeout, got %d", cfg.TimeoutSec)
	}
	if cfg.UnattendedMode {
		t.Errorf("expected UnattendedMode false by default")
	}
	if cfg.MaxRetriesBeforeAsk != 2 {
		t.Errorf("expected MaxRetriesBeforeAsk 2 by default, got %d", cfg.MaxRetriesBeforeAsk)
	}
	if len(cfg.StaticRules.AllowPrefixes) == 0 {
		t.Errorf("expected non-empty AllowPrefixes")
	}
	if len(cfg.StaticRules.DenySubstrings) == 0 {
		t.Errorf("expected non-empty DenySubstrings")
	}
}

func TestConfigPath(t *testing.T) {
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath error: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join(".agy-guard", "config.json")) {
		t.Errorf("expected path ending in .agy-guard/config.json, got %q", path)
	}
}

func TestLoadFromFile_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "does-not-exist.json")

	cfg, err := LoadFromFile(nonExistentPath)
	if err != nil {
		t.Fatalf("expected nil error on missing file, got: %v", err)
	}
	if cfg.Provider != "gemini" {
		t.Errorf("expected default config, got %v", cfg)
	}
}

func TestSaveAndLoadCustomConfig(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "sub", "config.json")

	// Save default
	savedPath, err := SaveDefaultToPath(targetPath)
	if err != nil {
		t.Fatalf("SaveDefaultToPath error: %v", err)
	}
	if savedPath != targetPath {
		t.Errorf("expected saved path %q, got %q", targetPath, savedPath)
	}

	// Load and check
	loaded, err := LoadFromFile(targetPath)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	if loaded.APIKey != "YOUR_GEMINI_API_KEY_HERE" {
		t.Errorf("unexpected APIKey: %q", loaded.APIKey)
	}

	// Modify file
	customContent := `{"provider": "ollama", "model": "llama3.2", "timeout_sec": 15}`
	if err := os.WriteFile(targetPath, []byte(customContent), 0600); err != nil {
		t.Fatalf("failed to overwrite file: %v", err)
	}

	// Test SaveDefaultToPath doesn't overwrite existing
	_, err = SaveDefaultToPath(targetPath)
	if err != nil {
		t.Fatalf("SaveDefaultToPath on existing file returned error: %v", err)
	}

	reloaded, err := LoadFromFile(targetPath)
	if err != nil {
		t.Fatalf("LoadFromFile after SaveDefaultToPath error: %v", err)
	}
	if reloaded.Provider != "ollama" || reloaded.Model != "llama3.2" {
		t.Errorf("file was overwritten: %+v", reloaded)
	}
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(targetPath, []byte("{invalid"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := LoadFromFile(targetPath)
	if err == nil {
		t.Fatalf("expected error on invalid json, got nil")
	}
}

func TestSaveToPath(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "custom", "config.json")

	cfg := DefaultConfig()
	cfg.Provider = "ollama"
	cfg.Model = "qwen2.5-coder:7b"
	cfg.APIKey = "custom-key"

	if err := SaveToPath(cfg, targetPath); err != nil {
		t.Fatalf("SaveToPath error: %v", err)
	}

	loaded, err := LoadFromFile(targetPath)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	if loaded.Provider != "ollama" || loaded.Model != "qwen2.5-coder:7b" || loaded.APIKey != "custom-key" {
		t.Errorf("loaded mismatch: %+v", loaded)
	}
}
