package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HooksConfigPath возвращает путь к ~/.gemini/config/hooks.json.
func HooksConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gemini", "config", "hooks.json"), nil
}

// HookHandler описывает отдельный обработчик команды хука.
type HookHandler struct {
	Type    string `json:"type,omitempty"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// ToolHookGroup описывает группу хуков с матчером инструмента.
type ToolHookGroup struct {
	Matcher string        `json:"matcher"`
	Hooks   []HookHandler `json:"hooks"`
}

// NamedHook описывает именованную конфигурацию хуков.
type NamedHook struct {
	PreToolUse []ToolHookGroup `json:"PreToolUse,omitempty"`
}

// PatchHooksJSON безопасно добавляет или обновляет запись "agy-guard" в ~/.gemini/config/hooks.json,
// сохраняя все существующие хуки пользователя.
func PatchHooksJSON(binPath string) (string, error) {
	path, err := HooksConfigPath()
	if err != nil {
		return "", err
	}

	return PatchHooksFile(path, binPath)
}

// PatchHooksFile применяет патч хука к конкретному файлу.
func PatchHooksFile(path, binPath string) (string, error) {
	var hooksMap map[string]any

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			hooksMap = make(map[string]any)
		} else {
			return "", fmt.Errorf("read hooks.json: %w", err)
		}
	} else {
		hooksMap = make(map[string]any)
		if len(data) > 0 {
			if err := json.Unmarshal(data, &hooksMap); err != nil {
				return "", fmt.Errorf("parse existing hooks.json: %w", err)
			}
		}
	}

	// Формируем блок конфигурации agy-guard для PreToolUse
	agyGuardEntry := NamedHook{
		PreToolUse: []ToolHookGroup{
			{
				Matcher: "run_command",
				Hooks: []HookHandler{
					{
						Type:    "command",
						Command: binPath,
					},
				},
			},
		},
	}

	hooksMap["agy-guard"] = agyGuardEntry

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	out, err := json.MarshalIndent(hooksMap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal hooks.json: %w", err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return "", fmt.Errorf("write hooks.json: %w", err)
	}

	return path, nil
}
