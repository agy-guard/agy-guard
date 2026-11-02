package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSettingsPermissions_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")

	commands := []string{"command(go test)", "command(git status)"}
	savedPath, err := EnsureSettingsPermissions(path, commands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if savedPath != path {
		t.Errorf("savedPath = %q, want %q", savedPath, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}

	perms, ok := root["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions object missing or invalid: %v", root)
	}

	allow, ok := perms["allow"].([]any)
	if !ok || len(allow) != 2 {
		t.Fatalf("expected 2 items in allow, got %v", allow)
	}
	if allow[0] != "command(go test)" || allow[1] != "command(git status)" {
		t.Errorf("unexpected allow items: %v", allow)
	}
}

func TestEnsureSettingsPermissions_PreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")

	initial := map[string]any{
		"model": "Gemini 3.8 Flash (High)",
		"permissions": map[string]any{
			"allow": []any{"command(existing)"},
		},
		"trustedWorkspaces": []any{"/Users/test"},
	}
	data, _ := json.Marshal(initial)
	_ = os.WriteFile(path, data, 0644)

	commands := []string{"command(existing)", "command(go test)"}
	_, err := EnsureSettingsPermissions(path, commands)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	readData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var result map[string]any
	_ = json.Unmarshal(readData, &result)

	if result["model"] != "Gemini 3.8 Flash (High)" {
		t.Errorf("expected model to be preserved, got %v", result["model"])
	}

	perms := result["permissions"].(map[string]any)
	allow := perms["allow"].([]any)
	if len(allow) != 2 {
		t.Fatalf("expected 2 items, got %v", allow)
	}
	if allow[0] != "command(existing)" || allow[1] != "command(go test)" {
		t.Errorf("unexpected allow list: %v", allow)
	}
}
