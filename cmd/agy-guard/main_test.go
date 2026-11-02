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

	// Attempt 2: Agent tries again -> In AFK mode, MUST stay Deny to avoid prompting user!
	res2 := applyPolicy(askVerdict, "rm -rf ./build", "sess-afk", cfg, breaker)
	if res2.Decision != hook.DecisionDeny {
		t.Errorf("attempt 2: expected deny in AFK mode, got %v", res2.Decision)
	}
	if !strings.Contains(res2.Reason, "Loop Breaker") || !strings.Contains(res2.Reason, "User is AFK") {
		t.Errorf("attempt 2: expected loop breaker AFK reason, got %q", res2.Reason)
	}
}

func TestApplyPolicy_AllowResetsCounter(t *testing.T) {
	tmpDir := t.TempDir()
	breaker := classifier.NewLoopBreaker(filepath.Join(tmpDir, "state.json"))

	cfg := config.DefaultConfig()
	cfg.MaxRetriesBeforeAsk = 2

	denyVerdict := hook.OutputVerdict{Decision: hook.DecisionDeny, Reason: "Disallowed"}
	allowVerdict := hook.OutputVerdict{Decision: hook.DecisionAllow, Reason: "Allowed"}

	// Attempt 1: Deny (Strike 1)
	_ = applyPolicy(denyVerdict, "git push origin main", "sess-reset", cfg, breaker)
	if breaker.GetDenyCount("sess-reset", "git push origin main") != 1 {
		t.Errorf("expected count 1 after deny")
	}

	// Allowed execution resets count
	_ = applyPolicy(allowVerdict, "git push origin main", "sess-reset", cfg, breaker)
	if breaker.GetDenyCount("sess-reset", "git push origin main") != 0 {
		t.Errorf("expected count 0 after allow, got %d", breaker.GetDenyCount("sess-reset", "git push origin main"))
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

func TestEvaluateNonCommandTool_ReadOnlyOutsideWorkspace(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	outsideDir := filepath.Join(filepath.Dir(tmpDir), "other_dir", "doc.md")
	payload := &hook.InputPayload{
		Cwd: tmpDir,
		ToolCall: hook.ToolCall{
			Name: "read_file",
			Args: map[string]any{
				"Path": outsideDir,
			},
		},
	}

	verdict, tier, handled := evaluateNonCommandTool(payload, cfg, nil)
	if !handled {
		t.Fatalf("expected tool to be handled")
	}
	if verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected DecisionAllow for read_file outside workspace, got %v", verdict.Decision)
	}
	if tier != "file_read_fast_path" {
		t.Errorf("expected tier 'file_read_fast_path', got %q", tier)
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

func TestMatchDirectUserIntent(t *testing.T) {
	allowPrefixes := []string{"git add", "git commit", "git status"}

	// 1. Explicit push request in Russian
	verdict, ok := matchDirectUserIntent("git push origin main", "пуш коммиты и релиз 0.3.0", allowPrefixes)
	if !ok || verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected git push to match user intent 'пуш'")
	}

	// 2. English push request
	verdict, ok = matchDirectUserIntent("git push origin main", "please push these changes to github", allowPrefixes)
	if !ok || verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected git push to match user intent 'push'")
	}

	// 3. Dangerous force push NEVER matches direct intent
	_, ok = matchDirectUserIntent("git push -f origin main", "пушни с силой", allowPrefixes)
	if ok {
		t.Errorf("force push must not be allowed via direct intent")
	}

	// 4. Dangerous --force flag
	_, ok = matchDirectUserIntent("git push --force origin main", "push now", allowPrefixes)
	if ok {
		t.Errorf("force push must not be allowed via direct intent")
	}

	// 5. Unrelated intent
	_, ok = matchDirectUserIntent("git push origin main", "проверь тесты", allowPrefixes)
	if ok {
		t.Errorf("unrelated intent should not match")
	}

	// 6. Release tag
	verdict, ok = matchDirectUserIntent("git tag -a v0.3.0 -m 'Release v0.3.0'", "сделай релиз v0.3.0", allowPrefixes)
	if !ok || verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected git tag to match release intent")
	}

	// 7. GitHub release create with -F notes-file flag
	verdict, ok = matchDirectUserIntent("gh release create v0.3.0 -t 'v0.3.0' -F RELEASE_NOTES_v0.3.0.md", "пуш коммиты и релиз 0.3.0 вместе с текстом релиза", allowPrefixes)
	if !ok || verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected gh release with -F flag to match user intent 'релиз'")
	}

	// 8. Compound command combining whitelist prefixes with intent push
	verdict, ok = matchDirectUserIntent("git add . && git commit -m 'release' && git push origin main", "пуш коммиты", allowPrefixes)
	if !ok || verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected compound command with whitelist + push intent to be allowed")
	}
}
