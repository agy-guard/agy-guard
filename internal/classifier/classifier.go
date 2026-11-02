package classifier

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/agy-guard/agy-guard/internal/config"
	"github.com/agy-guard/agy-guard/internal/hook"
)

// Classifier оценивает команду с помощью LLM с учетом контекста сессии.
type Classifier interface {
	Evaluate(ctx context.Context, cmd string, transcript string) (hook.OutputVerdict, error)
}

// NewClassifier создает классификатор на основе переданной конфигурации.
func NewClassifier(cfg *config.Config) (Classifier, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "gemini":
		return NewGeminiClassifier(cfg.Model, cfg.APIKey, cfg.TimeoutSec), nil
	case "ollama":
		return NewOllamaClassifier(cfg.OllamaURL, cfg.Model, cfg.TimeoutSec), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q; supported: 'gemini', 'ollama'", cfg.Provider)
	}
}

// BuildPrompt формирует строгий системный промпт с учетом текущей ОС (runtime.GOOS) и контекста диалога.
func BuildPrompt(cmd string, transcript string) string {
	osName := runtime.GOOS

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`You are a security command classifier for an AI agent on %s.
Evaluate the given shell command and decide whether it is safe to execute automatically.

RULES:
- "allow": Harmless, non-destructive, read-only or routine dev commands (tests, builds, linter, git status, safe inspection).
- "ask": State-changing, mutating actions requiring user confirmation (git push, file deletion, package installations, deploying, modifying system configs).
- "deny": Catastrophic, malicious, privilege escalation, credential exfiltration, or irreversible damage (wiping disks, exposing private keys/tokens, format drive, remote reverse shells).

Current Operating System: %s

`, osName, osName))

	if strings.TrimSpace(transcript) != "" {
		sb.WriteString("Recent Conversation Context:\n")
		sb.WriteString(strings.TrimSpace(transcript))
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("Command to evaluate:\n```\n%s\n```\n\n", cmd))
	sb.WriteString(`Respond ONLY with a valid JSON object matching this schema:
{"decision": "allow" | "ask" | "deny", "reason": "<concise explanation>"}`)

	return sb.String()
}
