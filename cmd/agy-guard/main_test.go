package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agy-guard/agy-guard/internal/classifier"
	"github.com/agy-guard/agy-guard/internal/config"
	"github.com/agy-guard/agy-guard/internal/hook"
)

func TestApplyPolicy_Allow(t *testing.T) {
	cfg := config.DefaultConfig()
	v := hook.OutputVerdict{Decision: hook.DecisionAllow, Reason: "Safe command"}

	res := applyPolicy(v, "go test ./...", "sess-1", cfg, nil)
	if res.Decision != hook.DecisionAllow {
		t.Errorf("expected allow, got %v", res.Decision)
	}
	if res.Reason != "Safe command" {
		t.Errorf("expected reason 'Safe command', got %q", res.Reason)
	}
}

func TestApplyPolicy_UnattendedMode_AskToDeny(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UnattendedMode = true

	v := hook.OutputVerdict{Decision: hook.DecisionAsk, Reason: "Deploying code"}
	res := applyPolicy(v, "git push", "sess-1", cfg, nil)

	if res.Decision != hook.DecisionDeny {
		t.Errorf("expected ask to convert to deny in unattended mode, got %v", res.Decision)
	}
	if !strings.Contains(res.Reason, "User is AFK") {
		t.Errorf("expected AFK instruction in reason, got %q", res.Reason)
	}
	if !strings.Contains(res.Reason, "[AGY-GUARD SECURITY BLOCK]") {
		t.Errorf("expected security block in reason, got %q", res.Reason)
	}
}

func TestApplyPolicy_LoopBreaker_Strike2Escalation(t *testing.T) {
	tmpDir := t.TempDir()
	breaker := classifier.NewLoopBreaker(filepath.Join(tmpDir, "state.json"))

	cfg := config.DefaultConfig()
	cfg.MaxRetriesBeforeAsk = 2

	cmd1 := "git push origin main"
	cmd2 := "git push -u origin HEAD"

	denyVerdict := hook.OutputVerdict{Decision: hook.DecisionDeny, Reason: "Dangerous mutation"}

	// Attempt 1: Strike 1 -> should stay Deny with Stop Directive
	res1 := applyPolicy(denyVerdict, cmd1, "sess-loop", cfg, breaker)
	if res1.Decision != hook.DecisionDeny {
		t.Errorf("attempt 1: expected deny, got %v", res1.Decision)
	}
	if !strings.Contains(res1.Reason, "[AGY-GUARD SECURITY BLOCK]") {
		t.Errorf("attempt 1: expected FormatDenyReason wrapping, got %q", res1.Reason)
	}

	// Attempt 2: Strike 2 -> MUST escalate to Ask
	res2 := applyPolicy(denyVerdict, cmd2, "sess-loop", cfg, breaker)
	if res2.Decision != hook.DecisionAsk {
		t.Errorf("attempt 2: expected escalation to ask, got %v", res2.Decision)
	}
	if !strings.Contains(res2.Reason, "Loop Breaker: Agent repeated a blocked command (git push)") {
		t.Errorf("attempt 2: expected loop breaker escalation reason, got %q", res2.Reason)
	}
}

func TestApplyPolicy_UnattendedMode_WithLoopBreaker(t *testing.T) {
	tmpDir := t.TempDir()
	breaker := classifier.NewLoopBreaker(filepath.Join(tmpDir, "state.json"))

	cfg := config.DefaultConfig()
	cfg.UnattendedMode = true
	cfg.MaxRetriesBeforeAsk = 2

	askVerdict := hook.OutputVerdict{Decision: hook.DecisionAsk, Reason: "State changing"}

	// Attempt 1: Converted to Deny with AFK reason
	res1 := applyPolicy(askVerdict, "rm -rf build", "sess-afk", cfg, breaker)
	if res1.Decision != hook.DecisionDeny {
		t.Errorf("attempt 1: expected deny, got %v", res1.Decision)
	}
	if !strings.Contains(res1.Reason, "User is AFK") {
		t.Errorf("attempt 1: expected AFK instruction, got %q", res1.Reason)
	}

	// Attempt 2: Agent tries again -> Loop Breaker escalates to Ask to pause execution
	res2 := applyPolicy(askVerdict, "rm -rf ./build", "sess-afk", cfg, breaker)
	if res2.Decision != hook.DecisionAsk {
		t.Errorf("attempt 2: expected escalation to ask, got %v", res2.Decision)
	}
	if !strings.Contains(res2.Reason, "Loop Breaker") {
		t.Errorf("attempt 2: expected loop breaker reason, got %q", res2.Reason)
	}
}

func TestEvaluateNonCommandTool_WorkspaceFile(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "src", "app.go")
	payload := &hook.InputPayload{
		Cwd: tmpDir,
		ToolCall: hook.ToolCall{
			Name: "edit_file",
			Args: map[string]any{
				"TargetFile": target,
			},
		},
	}

	verdict, tier, handled := evaluateNonCommandTool(payload, cfg, nil)
	if !handled {
		t.Fatalf("expected tool to be handled by non-command filter")
	}
	if verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected DecisionAllow for workspace file, got %v", verdict.Decision)
	}
	if tier != "file_fast_path" {
		t.Errorf("expected tier 'file_fast_path', got %q", tier)
	}
}

func TestEvaluateNonCommandTool_WorkspacePathsWithoutCwd(t *testing.T) {
	cfg := config.DefaultConfig()
	workspaceDir := t.TempDir()
	pluginDir := t.TempDir() // Cwd points to plugin directory, not workspace!
	target := filepath.Join(workspaceDir, "src", "app.go")

	payload := &hook.InputPayload{
		Cwd:            pluginDir,
		WorkspacePaths: []string{workspaceDir},
		ToolCall: hook.ToolCall{
			Name: "edit_file",
			Args: map[string]any{
				"TargetFile": target,
			},
		},
	}

	verdict, tier, handled := evaluateNonCommandTool(payload, cfg, nil)
	if !handled {
		t.Fatalf("expected tool to be handled by non-command filter")
	}
	if verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected DecisionAllow for WorkspacePaths, got %v", verdict.Decision)
	}
	if tier != "file_fast_path" {
		t.Errorf("expected tier 'file_fast_path', got %q", tier)
	}
}

func TestEvaluateNonCommandTool_SensitiveFile(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, ".env")
	payload := &hook.InputPayload{
		Cwd: tmpDir,
		ToolCall: hook.ToolCall{
			Name: "write_file",
			Args: map[string]any{
				"Path": target,
			},
		},
	}

	verdict, tier, handled := evaluateNonCommandTool(payload, cfg, nil)
	if !handled {
		t.Fatalf("expected tool to be handled")
	}
	if verdict.Decision != hook.DecisionAsk {
		t.Errorf("expected DecisionAsk for .env modification, got %v", verdict.Decision)
	}
	if tier != "file_sensitive" {
		t.Errorf("expected tier 'file_sensitive', got %q", tier)
	}
	if !strings.Contains(verdict.Reason, "sensitive") {
		t.Errorf("expected sensitive warning in reason, got %q", verdict.Reason)
	}
}

func TestEvaluateNonCommandTool_OutsideWorkspace(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	outsideDir := filepath.Join(filepath.Dir(tmpDir), "other_dir", "outside.go")
	payload := &hook.InputPayload{
		Cwd: tmpDir,
		ToolCall: hook.ToolCall{
			Name: "write_file",
			Args: map[string]any{
				"file_path": outsideDir,
			},
		},
	}

	verdict, tier, handled := evaluateNonCommandTool(payload, cfg, nil)
	if !handled {
		t.Fatalf("expected tool to be handled")
	}
	if verdict.Decision != hook.DecisionAsk {
		t.Errorf("expected DecisionAsk for outside workspace file, got %v", verdict.Decision)
	}
	if tier != "file_outside_workspace" {
		t.Errorf("expected tier 'file_outside_workspace', got %q", tier)
	}
	if !strings.Contains(verdict.Reason, "outside current workspace") {
		t.Errorf("expected outside workspace warning, got %q", verdict.Reason)
	}
}

func TestEvaluateNonCommandTool_NoTargetFile(t *testing.T) {
	cfg := config.DefaultConfig()
	payload := &hook.InputPayload{
		Cwd: "/Users/test/project",
		ToolCall: hook.ToolCall{
			Name: "list_directory",
			Args: map[string]any{},
		},
	}

	verdict, tier, handled := evaluateNonCommandTool(payload, cfg, nil)
	if !handled {
		t.Fatalf("expected tool to be handled")
	}
	if verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected DecisionAllow for tool without target file, got %v", verdict.Decision)
	}
	if tier != "tool_fast_path" {
		t.Errorf("expected tier 'tool_fast_path', got %q", tier)
	}
}

func TestEvaluateNonCommandTool_CommandToolPassthrough(t *testing.T) {
	cfg := config.DefaultConfig()
	payload := &hook.InputPayload{
		ToolCall: hook.ToolCall{
			Name: "run_command",
			Args: map[string]any{
				"CommandLine": "git status",
			},
		},
	}

	_, _, handled := evaluateNonCommandTool(payload, cfg, nil)
	if handled {
		t.Errorf("expected run_command NOT to be handled by non-command filter")
	}
}
