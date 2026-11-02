package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchHooksFile_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	hooksFile := filepath.Join(tmpDir, "hooks.json")
	binPath := "/usr/local/bin/agy-guard"

	saved, err := PatchHooksFile(hooksFile, binPath)
	if err != nil {
		t.Fatalf("PatchHooksFile error: %v", err)
	}
	if saved != hooksFile {
		t.Errorf("expected %q, got %q", hooksFile, saved)
	}

	data, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	var root map[string]NamedHook
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	hook, ok := root["agy-guard"]
	if !ok {
		t.Fatalf("agy-guard key not found in hooks.json")
	}
	if len(hook.PreToolUse) != 1 || hook.PreToolUse[0].Matcher != ".*" {
		t.Fatalf("unexpected PreToolUse: %+v", hook.PreToolUse)
	}
	if len(hook.PreToolUse[0].Hooks) != 1 || hook.PreToolUse[0].Hooks[0].Command != binPath {
		t.Fatalf("unexpected command in hooks: %+v", hook.PreToolUse[0].Hooks)
	}
}

func TestPatchHooksFile_PreserveExisting(t *testing.T) {
	tmpDir := t.TempDir()
	hooksFile := filepath.Join(tmpDir, "hooks.json")

	initial := `{
		"linter": {
			"PostToolUse": [
				{
					"matcher": "edit_file",
					"hooks": [{"command": "./lint.sh"}]
				}
			]
		}
	}`
	if err := os.WriteFile(hooksFile, []byte(initial), 0644); err != nil {
		t.Fatalf("write initial file error: %v", err)
	}

	binPath := "C:\\Users\\amudenn\\bin\\agy-guard.exe"
	_, err := PatchHooksFile(hooksFile, binPath)
	if err != nil {
		t.Fatalf("PatchHooksFile error: %v", err)
	}

	data, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if _, ok := root["linter"]; !ok {
		t.Errorf("existing 'linter' hook was dropped!")
	}
	if _, ok := root["agy-guard"]; !ok {
		t.Errorf("'agy-guard' hook was not added!")
	}
}

func TestUnpatchHooksFile(t *testing.T) {
	tmpDir := t.TempDir()
	hooksFile := filepath.Join(tmpDir, "hooks.json")

	initial := `{
		"linter": {
			"PostToolUse": [{"command": "./lint.sh"}]
		},
		"agy-guard": {
			"PreToolUse": [{"matcher": ".*", "hooks": [{"command": "agy-guard"}]}]
		}
	}`
	if err := os.WriteFile(hooksFile, []byte(initial), 0644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	saved, err := UnpatchHooksFile(hooksFile)
	if err != nil {
		t.Fatalf("UnpatchHooksFile error: %v", err)
	}
	if saved != hooksFile {
		t.Errorf("expected %q, got %q", hooksFile, saved)
	}

	data, err := os.ReadFile(hooksFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, exists := root["agy-guard"]; exists {
		t.Errorf("agy-guard key should have been removed")
	}
	if _, exists := root["linter"]; !exists {
		t.Errorf("linter key should have been preserved")
	}

	// Idempotent call
	if _, err := UnpatchHooksFile(hooksFile); err != nil {
		t.Errorf("second UnpatchHooksFile call failed: %v", err)
	}

	// Missing file
	if _, err := UnpatchHooksFile(filepath.Join(tmpDir, "non-existent.json")); err != nil {
		t.Errorf("UnpatchHooksFile on missing file failed: %v", err)
	}
}
