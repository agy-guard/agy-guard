package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTranscriptTail_MissingAndEmpty(t *testing.T) {
	if got := ReadTranscriptTail("", 4096); got != "" {
		t.Errorf("expected empty string for empty path, got %q", got)
	}

	if got := ReadTranscriptTail("/non/existent/path/transcript.jsonl", 4096); got != "" {
		t.Errorf("expected empty string for non-existent file, got %q", got)
	}

	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.jsonl")
	if err := os.WriteFile(emptyFile, []byte(""), 0600); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	if got := ReadTranscriptTail(emptyFile, 4096); got != "" {
		t.Errorf("expected empty string for empty file, got %q", got)
	}
}

func TestReadTranscriptTail_Formatting(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "transcript.jsonl")

	lines := []string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"<USER_REQUEST>First prompt</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","content":"First response"}`,
		`{"step_index":2,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"Second prompt"}`,
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","content":"Second response"}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	result := ReadTranscriptTail(filePath, 4096)
	if !strings.Contains(result, "[USER]: Second prompt") {
		t.Errorf("expected Second prompt in context, got %q", result)
	}
	if !strings.Contains(result, "[ASSISTANT]: Second response") {
		t.Errorf("expected Second response in context, got %q", result)
	}
	// Should only take last 3 turns, so First prompt should NOT be in result
	if strings.Contains(result, "First prompt") {
		t.Errorf("expected First prompt to be excluded due to turns limit, got %q", result)
	}
}

func TestReadTranscriptTail_LargeFileSeek(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large_transcript.jsonl")

	// Generate ~10KB of lines
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf(`{"step_index":%d,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"Step %d padding data %s"}`+"\n",
			i, i, strings.Repeat("A", 100)))
	}
	sb.WriteString(`{"step_index":101,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"Final user instruction"}` + "\n")

	if err := os.WriteFile(filePath, []byte(sb.String()), 0600); err != nil {
		t.Fatalf("failed to write large transcript: %v", err)
	}

	result := ReadTranscriptTail(filePath, 512)
	if !strings.Contains(result, "Final user instruction") {
		t.Errorf("expected Final user instruction in tail, got %q", result)
	}
}

func TestReadTranscriptTail_NonJSONLines(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "plain.log")

	content := "Line 1: started\nLine 2: running\nLine 3: finished\n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write plain file: %v", err)
	}

	result := ReadTranscriptTail(filePath, 1024)
	if !strings.Contains(result, "Line 3: finished") {
		t.Errorf("expected plain text line in result, got %q", result)
	}
}

func TestReadRawTail(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello world tail test"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	got := ReadRawTail(filePath, 9)
	if got != "tail test" {
		t.Errorf("ReadRawTail got %q, want 'tail test'", got)
	}
}
