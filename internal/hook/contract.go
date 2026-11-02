package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
