package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	safeFilenameRegex = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
	mu                sync.Mutex
)

// Record представляет отдельную запись аудит-лога о классификации команды.
type Record struct {
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id,omitempty"`
	Command   string `json:"command"`
	Cwd       string `json:"cwd,omitempty"`
	Tier      string `json:"tier"` // e.g. "tier1_static", "tier3_gemini", "tier3_ollama", "cache", "fallback"
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

// DefaultAuditDir возвращает путь по умолчанию ~/.agy-guard/audit.
func DefaultAuditDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".agy-guard", "audit"), nil
}

// SanitizeSessionID очищает session_id от потенциальных спецсимволов пути (защита от path traversal).
func SanitizeSessionID(sessionID string) string {
	clean := strings.TrimSpace(sessionID)
	if clean == "" {
		return time.Now().UTC().Format("2006-01-02")
	}
	clean = safeFilenameRegex.ReplaceAllString(clean, "_")
	clean = strings.Trim(clean, "._")
	if clean == "" {
		return time.Now().UTC().Format("2006-01-02")
	}
	if len(clean) > 128 {
		clean = clean[:128]
	}
	return clean
}

// LogRecord записывает структуру Record в структурированный JSONL-файл в указанную директорию.
func LogRecord(auditDir string, rec Record) error {
	if strings.TrimSpace(rec.Timestamp) == "" {
		rec.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(auditDir, 0700); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}

	fileName := SanitizeSessionID(rec.SessionID) + ".jsonl"
	filePath := filepath.Join(auditDir, fileName)

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal audit record: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open audit file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write audit record: %w", err)
	}

	return nil
}

// Log записывает Record в стандартную директорию ~/.agy-guard/audit.
func Log(rec Record) error {
	dir, err := DefaultAuditDir()
	if err != nil {
		return err
	}
	return LogRecord(dir, rec)
}
