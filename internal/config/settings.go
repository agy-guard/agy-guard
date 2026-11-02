package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SettingsConfigPath возвращает путь к ~/.gemini/antigravity-cli/settings.json.
func SettingsConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"), nil
}

// DefaultSettingsAllowList возвращает базовый набор безопасных команд для Antigravity CLI,
// предотвращающий нативные модальные диалоги подтверждения для стандартных инструментов разработки.
func DefaultSettingsAllowList() []string {
	return []string{
		// Go toolchain
		"command(go test)",
		"command(go build)",
		"command(go vet)",
		"command(go run)",
		"command(go list)",
		"command(go mod)",
		"command(go fmt)",
		"command(go env)",
		"command(go version)",
		"command(golangci-lint)",
		// Git read-only
		"command(git status)",
		"command(git diff)",
		"command(git log)",
		"command(git branch)",
		"command(git show)",
		"command(git ls-files)",
		"command(git rev-parse)",
		"command(git describe)",
		"command(git blame)",
		// Shell core utilities
		"command(ls)",
		"command(cat)",
		"command(head)",
		"command(tail)",
		"command(grep)",
		"command(find)",
		"command(sed)",
		"command(awk)",
		"command(cut)",
		"command(tr)",
		"command(echo)",
		"command(pwd)",
		"command(which)",
		"command(uname)",
		"command(stat)",
		"command(file)",
		"command(wc)",
		"command(diff)",
		"command(sort)",
		"command(uniq)",
		"command(tree)",
		"command(jq)",
		// Other language ecosystems
		"command(python3)",
		"command(python)",
		"command(pytest)",
		"command(cargo)",
		"command(npm)",
		"command(yarn)",
		"command(pnpm)",
		"command(npx)",
	}
}

// EnsureSettingsPermissions синхронизирует переданный список разрешений с ~/.gemini/antigravity-cli/settings.json,
// добавляя недостающие команды без перезаписи существующих настроек (model, statusLine, trustedWorkspaces).
func EnsureSettingsPermissions(path string, commands []string) (string, error) {
	if path == "" {
		var err error
		path, err = SettingsConfigPath()
		if err != nil {
			return "", err
		}
	}

	var root map[string]any

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			root = make(map[string]any)
		} else {
			return "", fmt.Errorf("read settings.json: %w", err)
		}
	} else {
		root = make(map[string]any)
		if len(data) > 0 {
			if err := json.Unmarshal(data, &root); err != nil {
				return "", fmt.Errorf("parse settings.json: %w", err)
			}
		}
	}

	permsRaw, ok := root["permissions"]
	var perms map[string]any
	if ok {
		if pm, ok := permsRaw.(map[string]any); ok {
			perms = pm
		} else {
			perms = make(map[string]any)
		}
	} else {
		perms = make(map[string]any)
	}

	existingAllowSet := make(map[string]bool)
	var allowList []string

	if allowRaw, ok := perms["allow"]; ok {
		if arr, ok := allowRaw.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && s != "" {
					existingAllowSet[s] = true
					allowList = append(allowList, s)
				}
			}
		}
	}

	added := false
	for _, cmd := range commands {
		if cmd != "" && !existingAllowSet[cmd] {
			existingAllowSet[cmd] = true
			allowList = append(allowList, cmd)
			added = true
		}
	}

	if !added && ok {
		// Ничего нового добавлять не нужно
		return path, nil
	}

	perms["allow"] = allowList
	root["permissions"] = perms

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create settings directory: %w", err)
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal settings.json: %w", err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return "", fmt.Errorf("write settings.json: %w", err)
	}

	return path, nil
}
