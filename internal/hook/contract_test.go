package hook

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseStdin(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		expectedCmd string
	}{
		{
			name: "valid CommandLine",
			input: `{
				"sessionId": "session-123",
				"transcriptPath": "/tmp/transcript.jsonl",
				"cwd": "/workspace",
				"hookEventName": "PreToolUse",
				"toolCall": {
					"name": "run_command",
					"args": {
						"CommandLine": "go test ./..."
					}
				}
			}`,
			wantErr:     false,
			expectedCmd: "go test ./...",
		},
		{
			name: "fallback to command",
			input: `{
				"toolCall": {
					"name": "run_command",
					"args": {
						"command": "ls -la"
					}
				}
			}`,
			wantErr:     false,
			expectedCmd: "ls -la",
		},
		{
			name:    "invalid json",
			input:   `{invalid-json`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := ParseStdin(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseStdin() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				cmd := payload.ExtractCommand()
				if cmd != tt.expectedCmd {
					t.Errorf("ExtractCommand() = %q, want %q", cmd, tt.expectedCmd)
				}
			}
		})
	}
}

func TestExtractCommandKeys(t *testing.T) {
	keys := []string{"CommandLine", "commandLine", "command", "cmd", "script"}
	for _, k := range keys {
		p := &InputPayload{
			ToolCall: ToolCall{
				Name: "run_command",
				Args: map[string]any{k: "echo test"},
			},
		}
		if got := p.ExtractCommand(); got != "echo test" {
			t.Errorf("ExtractCommand() for key %q = %q, want %q", k, got, "echo test")
		}
	}

	var nilPayload *InputPayload
	if got := nilPayload.ExtractCommand(); got != "" {
		t.Errorf("ExtractCommand() on nil = %q, want empty", got)
	}

	emptyArgs := &InputPayload{ToolCall: ToolCall{Name: "other"}}
	if got := emptyArgs.ExtractCommand(); got != "" {
		t.Errorf("ExtractCommand() with empty args = %q, want empty", got)
	}
}

func TestFormatVerdict(t *testing.T) {
	tests := []struct {
		name           string
		verdict        OutputVerdict
		wantDecision   Decision
		wantReasonPart string
	}{
		{
			name:         "allow",
			verdict:      OutputVerdict{Decision: DecisionAllow, Reason: "Safe"},
			wantDecision: DecisionAllow,
		},
		{
			name:         "deny",
			verdict:      OutputVerdict{Decision: DecisionDeny, Reason: "Dangerous"},
			wantDecision: DecisionDeny,
		},
		{
			name:         "ask",
			verdict:      OutputVerdict{Decision: DecisionAsk, Reason: "Confirmation needed"},
			wantDecision: DecisionAsk,
		},
		{
			name:         "invalid decision defaults to ask",
			verdict:      OutputVerdict{Decision: "unknown", Reason: "something"},
			wantDecision: DecisionAsk,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := FormatVerdict(tt.verdict)
			var parsed OutputVerdict
			if err := json.Unmarshal(b, &parsed); err != nil {
				t.Fatalf("FormatVerdict output is not valid JSON: %v", err)
			}
			if parsed.Decision != tt.wantDecision {
				t.Errorf("parsed.Decision = %v, want %v", parsed.Decision, tt.wantDecision)
			}
		})
	}
}

func TestEmit(t *testing.T) {
	var buf bytes.Buffer
	verdict := OutputVerdict{Decision: DecisionAllow, Reason: "ok"}
	Emit(&buf, verdict)

	var parsed OutputVerdict
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &parsed); err != nil {
		t.Fatalf("Emit output is not valid JSON: %v", err)
	}
	if parsed.Decision != DecisionAllow || parsed.Reason != "ok" {
		t.Errorf("unexpected output: %v", parsed)
	}
}
