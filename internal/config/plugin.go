package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginManifest описывает структуру plugin.json для Antigravity CLI.
type PluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
}

// PluginHooksFile описывает структуру hooks.json внутри директории плагина.
type PluginHooksFile struct {
	AgyGuard struct {
		Enabled    bool            `json:"enabled"`
		PreToolUse []ToolHookGroup `json:"PreToolUse"`
	} `json:"agy-guard"`
}

// PluginDir возвращает путь к директории плагина ~/.gemini/config/plugins/agy-guard.
func PluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gemini", "config", "plugins", "agy-guard"), nil
}

// InstallPlugin устанавливает agy-guard как нативный плагин в ~/.gemini/config/plugins/agy-guard.
func InstallPlugin(absBinPath string) (string, error) {
	dir, err := PluginDir()
	if err != nil {
		return "", err
	}

	return InstallPluginToDir(dir, absBinPath)
}

// InstallPluginToDir сохраняет манифест plugin.json и хуки hooks.json в указанную директорию.
func InstallPluginToDir(targetDir, absBinPath string) (string, error) {
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return "", fmt.Errorf("create plugin dir: %w", err)
	}

	// 1. Создаем plugin.json
	manifest := PluginManifest{
		Name:        "agy-guard",
		Description: "Ultra-fast zero-dependency security classifier and PreToolUse hook for Antigravity CLI (agy)",
		Version:     "0.2.0",
		Homepage:    "https://github.com/agy-guard/agy-guard",
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal plugin.json: %w", err)
	}
	manifestPath := filepath.Join(targetDir, "plugin.json")
	if err := os.WriteFile(manifestPath, manifestData, 0600); err != nil {
		return "", fmt.Errorf("write plugin.json: %w", err)
	}

	// 2. Создаем hooks.json для плагина
	pluginHooks := PluginHooksFile{}
	pluginHooks.AgyGuard.Enabled = true
	pluginHooks.AgyGuard.PreToolUse = []ToolHookGroup{
		{
			Matcher: ".*",
			Hooks: []HookHandler{
				{
					Type:    "command",
					Command: absBinPath,
					Timeout: 15,
				},
			},
		},
	}
	hooksData, err := json.MarshalIndent(pluginHooks, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal plugin hooks.json: %w", err)
	}
	hooksPath := filepath.Join(targetDir, "hooks.json")
	if err := os.WriteFile(hooksPath, hooksData, 0600); err != nil {
		return "", fmt.Errorf("write plugin hooks.json: %w", err)
	}

	return targetDir, nil
}

// RemovePlugin удаляет директорию нативного плагина ~/.gemini/config/plugins/agy-guard.
func RemovePlugin() (string, error) {
	dir, err := PluginDir()
	if err != nil {
		return "", err
	}
	return RemovePluginDir(dir)
}

// RemovePluginDir удаляет переданную директорию плагина.
func RemovePluginDir(dir string) (string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return dir, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("remove plugin dir %q: %w", dir, err)
	}
	return dir, nil
}
