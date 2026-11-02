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
	SessionID             string   `json:"sessionId,omitempty"`
	ConversationID        string   `json:"conversationId,omitempty"`
	TranscriptPath        string   `json:"transcriptPath,omitempty"`
	ArtifactDirectoryPath string   `json:"artifactDirectoryPath,omitempty"`
	WorkspacePaths        []string `json:"workspacePaths,omitempty"`
	Cwd                   string   `json:"cwd,omitempty"`
	HookEventName         string   `json:"hookEventName,omitempty"`
	LastUserInput         string   `json:"lastUserInput,omitempty"`
	StepIdx               int      `json:"stepIdx,omitempty"`
	ToolCall              ToolCall `json:"toolCall"`
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

// IsWithinWorkspaces проверяет, находится ли targetFile внутри хотя бы одного из воркспейсов или cwd.
func IsWithinWorkspaces(workspaces []string, cwd, targetFile string) bool {
	cleanTarget := strings.TrimSpace(targetFile)
	if cleanTarget == "" {
		return true
	}

	// Относительные пути без выхода за пределы (например, "foo.go", "./src/app.go") считаются внутри воркспейса
	if !filepath.IsAbs(cleanTarget) && !strings.HasPrefix(cleanTarget, "..") {
		return true
	}

	absTarget := filepath.Clean(cleanTarget)

	roots := make([]string, 0, len(workspaces)+1)
	for _, ws := range workspaces {
		if trimmed := strings.TrimSpace(ws); trimmed != "" {
			if abs, err := filepath.Abs(trimmed); err == nil {
				roots = append(roots, abs)
			}
		}
	}
	if trimmedCwd := strings.TrimSpace(cwd); trimmedCwd != "" {
		if abs, err := filepath.Abs(trimmedCwd); err == nil {
			roots = append(roots, abs)
		}
	}

	if len(roots) == 0 {
		if current, err := os.Getwd(); err == nil && current != "" {
			if abs, err := filepath.Abs(current); err == nil {
				roots = append(roots, abs)
			}
		}
	}

	// Проверяем, входит ли absTarget хотя бы в один из корней
	for _, root := range roots {
		rel, err := filepath.Rel(root, absTarget)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// IsWithinWorkspace проверяет, находится ли targetFile внутри рабочей директории cwd (для обратной совместимости).
func IsWithinWorkspace(cwd, targetFile string) bool {
	var ws []string
	if cwd != "" {
		ws = []string{cwd}
	}
	return IsWithinWorkspaces(ws, cwd, targetFile)
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
