package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPluginToDir(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := "/usr/local/bin/agy-guard"

	dir, err := InstallPluginToDir(tmpDir, binPath)
	if err != nil {
		t.Fatalf("InstallPluginToDir failed: %v", err)
	}
	if dir != tmpDir {
		t.Errorf("Returned dir = %q, want %q", dir, tmpDir)
	}

	// Verify plugin.json
	manifestFile := filepath.Join(tmpDir, "plugin.json")
	manifestBytes, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("Failed to read plugin.json: %v", err)
	}
	var manifest PluginManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("Failed to parse plugin.json: %v", err)
	}
	if manifest.Name != "agy-guard" {
		t.Errorf("Plugin name = %q, want 'agy-guard'", manifest.Name)
	}

	// Verify hooks.json
	hooksFile := filepath.Join(tmpDir, "hooks.json")
	hooksBytes, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatalf("Failed to read plugin hooks.json: %v", err)
	}
	var hooks PluginHooksFile
	if err := json.Unmarshal(hooksBytes, &hooks); err != nil {
		t.Fatalf("Failed to parse plugin hooks.json: %v", err)
	}
	if !hooks.AgyGuard.Enabled {
		t.Errorf("Expected agy-guard to be enabled in plugin hooks")
	}
	if len(hooks.AgyGuard.PreToolUse) == 0 || len(hooks.AgyGuard.PreToolUse[0].Hooks) == 0 {
		t.Fatalf("Expected hook registrations in plugin hooks")
	}
	if hooks.AgyGuard.PreToolUse[0].Hooks[0].Command != binPath {
		t.Errorf("Hook command = %q, want %q", hooks.AgyGuard.PreToolUse[0].Hooks[0].Command, binPath)
	}

	// Test RemovePluginDir
	removedDir, err := RemovePluginDir(tmpDir)
	if err != nil {
		t.Fatalf("RemovePluginDir error: %v", err)
	}
	if removedDir != tmpDir {
		t.Errorf("expected removed dir %q, got %q", tmpDir, removedDir)
	}
	if _, err := os.Stat(manifestFile); !os.IsNotExist(err) {
		t.Errorf("manifest file should not exist after remove")
	}

	// Idempotent remove on non-existent dir
	if _, err := RemovePluginDir(tmpDir); err != nil {
		t.Errorf("RemovePluginDir on non-existent dir failed: %v", err)
	}
}
