package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Provider                 string      `json:"provider"` // "gemini" или "ollama"
	Model                    string      `json:"model"`
	APIKey                   string      `json:"api_key"`
	OllamaURL                string      `json:"ollama_url"`
	TimeoutSec               int         `json:"timeout_sec"`
	IncludeTranscriptContext bool        `json:"include_transcript_context"`
	EnableAuditLog           bool        `json:"enable_audit_log"`
	EnableCache              bool        `json:"enable_cache"`
	CacheTTLMinutes          int         `json:"cache_ttl_minutes"`
	UnattendedMode           bool        `json:"unattended_mode"`
	MaxRetriesBeforeAsk      int         `json:"max_retries_before_ask"`
	StaticRules              StaticRules `json:"static_rules"`
}

type StaticRules struct {
	AllowPrefixes  []string `json:"allow_prefixes"`
	DenySubstrings []string `json:"deny_substrings"`
}

// DefaultConfig возвращает безопасный дефолтный конфиг для Windows, macOS и Linux.
func DefaultConfig() *Config {
	return &Config{
		Provider:                 "gemini",
		Model:                    "gemini-3.5-flash-lite",
		APIKey:                   "YOUR_GEMINI_API_KEY_HERE",
		OllamaURL:                "http://localhost:11434",
		TimeoutSec:               8,
		IncludeTranscriptContext: true,
		EnableAuditLog:           true,
		EnableCache:              true,
		CacheTTLMinutes:          10,
		UnattendedMode:           false,
		MaxRetriesBeforeAsk:      2,
		StaticRules: StaticRules{
			AllowPrefixes: []string{
				// Go
				"go test",
				"go build",
				"go vet",
				"go list",
				"go mod tidy",
				"go fmt",
				"go env",
				"go version",
				"go doc",
				// Git read-only
				"git status",
				"git diff",
				"git log",
				"git branch",
				"git show",
				"git rev-parse",
				"git remote",
				"git tag",
				"git ls-files",
				"git describe",
				"git blame",
				// Rust
				"cargo check",
				"cargo test",
				"cargo build",
				"cargo clippy",
				"cargo fmt",
				"cargo doc",
				// Node / JS
				"npm test",
				"npm run test",
				"npm run lint",
				"npm run build",
				"npm run typecheck",
				"npm ls",
				"yarn test",
				"pnpm test",
				"npx eslint",
				"npx tsc",
				"npx prettier --check",
				"bun test",
				// Python
				"pytest",
				"python -m unittest",
				"python3 -m unittest",
				"python -m pytest",
				"python3 -m pytest",
				"ruff check",
				"black --check",
				"mypy",
				"flake8",
				// Read-only inspection & utility tools
				"ls",
				"cat",
				"head",
				"tail",
				"grep",
				"egrep",
				"fgrep",
				"rg",
				"ag",
				"find",
				"pwd",
				"echo",
				"printf",
				"wc",
				"uname",
				"which",
				"stat",
				"file",
				"diff",
				"jq",
				"sort",
				"uniq",
				"tree",
			},
			DenySubstrings: []string{
				// Secrets, tokens & credentials
				".env",
				"id_rsa",
				"id_ed25519",
				"id_ecdsa",
				"gh auth token",
				"aws_secret_access_key",
				"google_api_key",
				"gemini_api_key",
				"openai_api_key",
				"anthropic_api_key",
				"authorization: bearer",
				"bearer eyj",
				"private key-----",
				".git-credentials",
				"/etc/shadow",
				"/etc/sudoers",
				// Privilege escalation
				"sudo ",
				"doas ",
				"su -",
				"su ",
				"runas ",
				"gsudo ",
				// Piped execution into shell
				"| sh",
				"| bash",
				"| zsh",
				"| dash",
				"| powershell",
				"| pwsh",
				"| cmd",
				"|sh",
				"|bash",
				"|zsh",
				"|pwsh",
				// Destructive Git commands
				"git push -f",
				"git push --force",
				"git reset --hard",
				"git reset --merge",
				"git clean -f",
				"git clean -fd",
				"git clean -fx",
				"git checkout .",
				"git restore .",
				"git branch -d",
				"git branch -d",
				"git stash drop",
				"git stash clear",
				// Repository & shell profile tampering
				".git/",
				".bashrc",
				".zshrc",
				".profile",
				".bash_profile",
				"config.fish",
				// Destructive disk / system operations (Unix & Windows)
				"rm -rf /",
				"rm -rf ~",
				"rm -rf $home",
				"rm -rf *",
				"rm -fr /",
				"rm -fr ~",
				"rm -fr $home",
				"rm -fr *",
				"mkfs",
				"dd if=",
				"format c:",
				"Format-Volume",
				"Remove-Item -Recurse -Force C:\\",
				"Stop-Computer",
				"shutdown ",
				"reboot ",
				":(){ :|:& };:",
			},
		},
	}
}

// ConfigPath возвращает кроссплатформенный путь к ~/.agy-guard/config.json.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".agy-guard", "config.json"), nil
}

// Load читает конфиг с диска. Если файла нет — возвращает DefaultConfig без ошибки.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}
	return LoadFromFile(path)
}

// LoadFromFile читает конфиг из конкретного пути файла.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("parse config.json: %w", err)
	}
	if cfg.MaxRetriesBeforeAsk <= 0 {
		cfg.MaxRetriesBeforeAsk = 2
	}
	return cfg, nil
}

// SaveDefault создает ~/.agy-guard/config.json, если он еще не существует.
func SaveDefault() (string, error) {
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return SaveDefaultToPath(path)
}

// SaveDefaultToPath сохраняет DefaultConfig по указанному пути, если файл еще не существует.
func SaveDefaultToPath(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(DefaultConfig(), "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write config file: %w", err)
	}
	return path, nil
}

// Save сохраняет переданную конфигурацию в ~/.agy-guard/config.json.
func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return SaveToPath(cfg, path)
}

// SaveToPath сохраняет переданную конфигурацию по указанному пути с правами 0600.
func SaveToPath(cfg *Config, path string) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}
