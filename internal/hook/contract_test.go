package hook

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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

func TestFormatDenyReason(t *testing.T) {
	reason := FormatDenyReason("Suspicious destructive action")
	if !strings.Contains(reason, "[AGY-GUARD SECURITY BLOCK]") {
		t.Errorf("expected security block prefix, got %q", reason)
	}
	if !strings.Contains(reason, "Suspicious destructive action") {
		t.Errorf("expected base reason preserved, got %q", reason)
	}
	if !strings.Contains(reason, "CRITICAL INSTRUCTION FOR AGENT: Do NOT retry this command") {
		t.Errorf("expected critical instruction, got %q", reason)
	}

	// Avoid double-wrapping
	doubleWrapped := FormatDenyReason(reason)
	if strings.Count(doubleWrapped, "[AGY-GUARD SECURITY BLOCK]") != 1 {
		t.Errorf("expected only one prefix when wrapped again, got %q", doubleWrapped)
	}
}

func TestExtractTargetFile(t *testing.T) {
	keys := []string{
		"TargetFile", "target_file", "targetFile",
		"file_path", "filePath", "FilePath",
		"Path", "path",
		"file", "File",
		"AbsolutePath", "DirectoryPath",
	}

	for _, k := range keys {
		p := &InputPayload{
			ToolCall: ToolCall{
				Name: "edit_file",
				Args: map[string]any{k: "src/main.go"},
			},
		}
		if got := p.ExtractTargetFile(); got != "src/main.go" {
			t.Errorf("ExtractTargetFile() for key %q = %q, want 'src/main.go'", k, got)
		}
	}

	var nilPayload *InputPayload
	if got := nilPayload.ExtractTargetFile(); got != "" {
		t.Errorf("expected empty for nil payload, got %q", got)
	}

	emptyArgs := &InputPayload{ToolCall: ToolCall{Name: "other"}}
	if got := emptyArgs.ExtractTargetFile(); got != "" {
		t.Errorf("expected empty for empty args, got %q", got)
	}
}

func TestIsSensitiveFile(t *testing.T) {
	sensitive := []string{
		".env",
		".env.local",
		".env.production",
		"/path/to/.env",
		"~/.gemini/config/hooks.json",
		"hooks.json",
		"~/.ssh/id_rsa",
		"~/.ssh/id_ed25519",
		"~/.ssh/config",
		".git/config",
		"~/.bashrc",
		"~/.zshrc",
	}

	for _, path := range sensitive {
		if !IsSensitiveFile(path) {
			t.Errorf("expected %q to be marked sensitive", path)
		}
	}

	safe := []string{
		"main.go",
		"src/index.ts",
		"README.md",
		"pkg/config/app.go",
		"internal/environment/env.go",
	}

	for _, path := range safe {
		if IsSensitiveFile(path) {
			t.Errorf("expected %q to be marked safe", path)
		}
	}
}

func TestIsWithinWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		target   string
		expected bool
	}{
		{"main.go", true},
		{"./src/app.go", true},
		{filepath.Join(tmpDir, "main.go"), true},
		{filepath.Join(tmpDir, "internal", "pkg", "foo.go"), true},
		{"../outside.go", false},
		{filepath.Join(filepath.Dir(tmpDir), "other_sibling", "main.go"), false},
	}

	for _, tt := range tests {
		got := IsWithinWorkspace(tmpDir, tt.target)
		if got != tt.expected {
			t.Errorf("IsWithinWorkspace(%q, %q) = %v, want %v", tmpDir, tt.target, got, tt.expected)
		}
	}
}
