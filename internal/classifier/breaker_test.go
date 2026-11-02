package classifier

import (
	"path/filepath"
	"testing"
)

func TestNormalizeCommandClass(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"git push origin main", "git push"},
		{"git push -u origin HEAD", "git push"},
		{"git push", "git push"},
		{"GIT PUSH --force", "git push"},
		{"sudo git push", "git push"},
		{"FOO=bar git push", "git push"},
		{"rm -rf ./dist", "rm -rf"},
		{"rm -rf build", "rm -rf"},
		{"Remove-Item -Recurse -Force dist", "remove-item -recurse"},
		{"reboot", "reboot"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeCommandClass(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeCommandClass(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLoopBreaker_Escalation(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "session_state.json")
	breaker := NewLoopBreaker(statePath)

	sessionID := "sess-123"

	// First attempt: Strike 1 -> should NOT escalate (maxRetries = 2)
	escalate, count, cmdClass := breaker.RecordDenyAndCheck(sessionID, "git push origin main", 2)
	if escalate {
		t.Fatalf("expected escalate=false on attempt 1, got true")
	}
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
	if cmdClass != "git push" {
		t.Errorf("expected cmdClass='git push', got %q", cmdClass)
	}

	// Second attempt with slightly different flags: Strike 2 -> MUST escalate
	escalate, count, cmdClass = breaker.RecordDenyAndCheck(sessionID, "git push -u origin HEAD", 2)
	if !escalate {
		t.Fatalf("expected escalate=true on attempt 2 (Strike 2), got false")
	}
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}
	if cmdClass != "git push" {
		t.Errorf("expected cmdClass='git push', got %q", cmdClass)
	}
}

func TestLoopBreaker_SessionIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "session_state.json")
	breaker := NewLoopBreaker(statePath)

	// Session A
	breaker.RecordDeny("sess-A", "rm -rf ./dist")
	if count := breaker.GetDenyCount("sess-A", "rm -rf ./dist"); count != 1 {
		t.Errorf("expected count=1 for sess-A, got %d", count)
	}

	// Session B should have count 0 for the same command
	if count := breaker.GetDenyCount("sess-B", "rm -rf ./dist"); count != 0 {
		t.Errorf("expected count=0 for sess-B, got %d", count)
	}

	// Different command in Session A should have count 0
	if count := breaker.GetDenyCount("sess-A", "git push"); count != 0 {
		t.Errorf("expected count=0 for git push in sess-A, got %d", count)
	}
}

func TestLoopBreaker_PersistenceAndReset(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "session_state.json")
	b1 := NewLoopBreaker(statePath)

	b1.RecordDeny("sess-persist", "git push")
	b1.RecordDeny("sess-persist", "git push")

	// Create new instance pointing to same file
	b2 := NewLoopBreaker(statePath)
	if count := b2.GetDenyCount("sess-persist", "git push"); count != 2 {
		t.Errorf("expected persisted count=2, got %d", count)
	}

	// Reset session
	if err := b2.Reset("sess-persist"); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	if count := b2.GetDenyCount("sess-persist", "git push"); count != 0 {
		t.Errorf("expected count=0 after reset, got %d", count)
	}
}

func TestLoopBreaker_ResetCommand(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "session_state.json")
	b := NewLoopBreaker(statePath)

	b.RecordDeny("sess-cmd", "git add foo.go")
	b.RecordDeny("sess-cmd", "git add bar.go")
	b.RecordDeny("sess-cmd", "git push origin main")

	if count := b.GetDenyCount("sess-cmd", "git add"); count != 2 {
		t.Errorf("expected count=2 for git add, got %d", count)
	}

	// Reset only git add
	if err := b.ResetCommand("sess-cmd", "git add foo.go"); err != nil {
		t.Fatalf("ResetCommand failed: %v", err)
	}

	if count := b.GetDenyCount("sess-cmd", "git add"); count != 0 {
		t.Errorf("expected count=0 for git add after ResetCommand, got %d", count)
	}

	// git push should remain untouched
	if count := b.GetDenyCount("sess-cmd", "git push"); count != 1 {
		t.Errorf("expected count=1 for git push, got %d", count)
	}
}
