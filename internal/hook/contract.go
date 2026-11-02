package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

// InputPayload описывает входящий JSON от Antigravity CLI на событии PreToolUse.
type InputPayload struct {
	SessionID      string   `json:"sessionId,omitempty"`
	ConversationID string   `json:"conversationId,omitempty"`
	TranscriptPath string   `json:"transcriptPath,omitempty"`
	Cwd            string   `json:"cwd,omitempty"`
	HookEventName  string   `json:"hookEventName,omitempty"`
	ToolCall       ToolCall `json:"toolCall"`
}

type ToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// OutputVerdict — плоский JSON-ответ верхнего уровня, который ждет Antigravity CLI.
type OutputVerdict struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
}

// ParseStdin читает и парсит JSON из stdin или любого io.Reader.
func ParseStdin(r io.Reader) (*InputPayload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	var payload InputPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal hook payload: %w", err)
	}
	return &payload, nil
}

// ExtractCommand достает строку команды из аргументов вызова тула.
// В agy run_command использует "CommandLine", но проверяем и фоллбэки.
func (p *InputPayload) ExtractCommand() string {
	if p == nil || p.ToolCall.Args == nil {
		return ""
	}
	keys := []string{"CommandLine", "commandLine", "command", "cmd", "script"}
	for _, k := range keys {
		if val, ok := p.ToolCall.Args[k]; ok {
			if str, ok := val.(string); ok && strings.TrimSpace(str) != "" {
				return strings.TrimSpace(str)
			}
		}
	}
	return ""
}

// ExtractTargetFile достает целевой путь к файлу из аргументов тулов редактирования/чтения/поиска.
func (p *InputPayload) ExtractTargetFile() string {
	if p == nil || p.ToolCall.Args == nil {
		return ""
	}
	keys := []string{
		"TargetFile", "target_file", "targetFile",
		"file_path", "filePath", "FilePath",
		"Path", "path",
		"file", "File",
		"AbsolutePath", "absolute_path",
		"DirectoryPath", "directory_path", "directoryPath",
	}
	for _, k := range keys {
		if val, ok := p.ToolCall.Args[k]; ok {
			if str, ok := val.(string); ok && strings.TrimSpace(str) != "" {
				return strings.TrimSpace(str)
			}
		}
	}
	return ""
}

// IsSensitiveFile проверяет, относится ли файл к критическим секретам или конфигам хука.
func IsSensitiveFile(targetFile string) bool {
	clean := strings.TrimSpace(targetFile)
	if clean == "" {
		return false
	}
	lower := strings.ToLower(filepath.ToSlash(clean))
	base := strings.ToLower(filepath.Base(clean))

	// Проверяем файлы .env
	if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env") {
		return true
	}

	sensitivePatterns := []string{
		"hooks.json",
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"id_dsa",
		".ssh/",
		"/.ssh",
		".git/config",
		".bashrc",
		".zshrc",
		".profile",
	}

	for _, p := range sensitivePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	return false
}

// IsWithinWorkspace проверяет, находится ли targetFile внутри рабочей директории cwd.
func IsWithinWorkspace(cwd, targetFile string) bool {
	cleanTarget := strings.TrimSpace(targetFile)
	cleanCwd := strings.TrimSpace(cwd)
	if cleanTarget == "" {
		return true
	}
	if cleanCwd == "" {
		var err error
		cleanCwd, err = os.Getwd()
		if err != nil || cleanCwd == "" {
			return true
		}
	}

	absCwd, err := filepath.Abs(cleanCwd)
	if err != nil {
		return true
	}

	var absTarget string
	if filepath.IsAbs(cleanTarget) {
		absTarget = filepath.Clean(cleanTarget)
	} else {
		absTarget = filepath.Clean(filepath.Join(absCwd, cleanTarget))
	}

	rel, err := filepath.Rel(absCwd, absTarget)
	if err != nil {
		return false
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}

// FormatDenyReason оборачивает причину отказа в строгую стоп-инструкцию для AI-агента,
// предотвращающую бесконечные повторные попытки выполнения заблокированной команды.
func FormatDenyReason(baseReason string) string {
	baseReason = strings.TrimSpace(baseReason)
	if baseReason == "" {
		baseReason = "Execution of this command was rejected by security policy"
	}
	if strings.Contains(baseReason, "[AGY-GUARD SECURITY BLOCK]") {
		return baseReason
	}
	return fmt.Sprintf(
		"[AGY-GUARD SECURITY BLOCK] %s. "+
			"CRITICAL INSTRUCTION FOR AGENT: Do NOT retry this command or variations of it. "+
			"Skip this operation immediately and inform the user in chat that manual execution is required.",
		baseReason,
	)
}

// FormatVerdict сериализует вердикт в валидный JSON-байтсрез с гарантией допустимого Decision.
func FormatVerdict(verdict OutputVerdict) []byte {
	if verdict.Decision != DecisionAllow && verdict.Decision != DecisionAsk && verdict.Decision != DecisionDeny {
		verdict.Decision = DecisionAsk
	}
	out, err := json.Marshal(verdict)
	if err != nil {
		// Железный фоллбэк, если маршалинг дал сбой
		return []byte(`{"decision":"ask","reason":"internal marshaling error in agy-guard"}`)
	}
	return out
}

// Emit печатает валидный JSON в указанный io.Writer.
func Emit(w io.Writer, verdict OutputVerdict) {
	fmt.Fprintln(w, string(FormatVerdict(verdict)))
}

// EmitAndExit печатает валидный JSON в stdout и ВСЕГДА завершает процесс с кодом 0.
// Любой ненулевой код выхода вызовет ошибку "pre-tool hook failed" в agy.
func EmitAndExit(verdict OutputVerdict) {
	Emit(os.Stdout, verdict)
	os.Exit(0)
}
