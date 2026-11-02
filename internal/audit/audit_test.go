package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeSessionID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal-session-123", "normal-session-123"},
		{"../../../etc/passwd", "etc_passwd"},
		{"session with spaces & special!@#$", "session_with_spaces___special"},
		{"", time.Now().UTC().Format("2006-01-02")},
		{"   \t  ", time.Now().UTC().Format("2006-01-02")},
	}

	for _, tt := range tests {
		got := SanitizeSessionID(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeSessionID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLogRecord(t *testing.T) {
	tmpDir := t.TempDir()

	rec1 := Record{
		SessionID: "test-session-1",
		Command:   "go test ./...",
		Cwd:       "/workspace",
		Tier:      "tier1_static",
		Decision:  "allow",
		Reason:    "Matched safe prefix",
		LatencyMS: 1,
	}

	rec2 := Record{
		SessionID: "test-session-1",
		Command:   "git push -f",
		Cwd:       "/workspace",
		Tier:      "tier1_static",
		Decision:  "deny",
		Reason:    "Blocked by static rule",
		LatencyMS: 0,
	}

	if err := LogRecord(tmpDir, rec1); err != nil {
		t.Fatalf("LogRecord(rec1) failed: %v", err)
	}
	if err := LogRecord(tmpDir, rec2); err != nil {
		t.Fatalf("LogRecord(rec2) failed: %v", err)
	}

	expectedFile := filepath.Join(tmpDir, "test-session-1.jsonl")
	f, err := os.Open(expectedFile)
	if err != nil {
		t.Fatalf("Failed to open expected audit file %s: %v", expectedFile, err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("Failed to unmarshal audit JSON line %q: %v", line, err)
		}
		records = append(records, r)
	}

	if len(records) != 2 {
		t.Fatalf("Expected 2 audit records, got %d", len(records))
	}

	if records[0].Command != "go test ./..." || records[0].Decision != "allow" {
		t.Errorf("Record 0 mismatch: %+v", records[0])
	}
	if records[1].Command != "git push -f" || records[1].Decision != "deny" {
		t.Errorf("Record 1 mismatch: %+v", records[1])
	}
}
