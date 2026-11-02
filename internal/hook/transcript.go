package hook

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// DefaultMaxTranscriptBytes — максимальный объем читаемого хвоста лога (4 KB).
	DefaultMaxTranscriptBytes int64 = 4096
	// DefaultMaxTranscriptTurns — максимальное количество последних шагов контекста.
	DefaultMaxTranscriptTurns = 3
)

// TranscriptEntry описывает базовые поля шага транскрипта Antigravity CLI.
type TranscriptEntry struct {
	StepIndex int    `json:"step_index"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Content   string `json:"content"`
}

// ReadTranscriptTail безопасно читает хвост файла transcriptPath (до maxBytes, по умолчанию 4KB)
// и форматирует последние шаги диалога в компактный контекст для передачи в LLM.
// При отсутствии файла, ошибке доступа или блокировке возвращает пустую строку (fail gracefully).
func ReadTranscriptTail(path string, maxBytes int64) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxTranscriptBytes
	}

	raw := readRawTail(path, maxBytes)
	if len(raw) == 0 {
		return ""
	}

	return formatRecentTurns(raw, DefaultMaxTranscriptTurns)
}

// ReadRawTail читает до maxBytes байт с конца файла. В случае любых ошибок возвращает пустую строку.
func ReadRawTail(path string, maxBytes int64) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxTranscriptBytes
	}
	return string(readRawTail(path, maxBytes))
}

func readRawTail(path string, maxBytes int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return nil
	}

	size := stat.Size()
	offset := int64(0)
	if size > maxBytes {
		offset = size - maxBytes
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil
		}
	}

	buf, err := io.ReadAll(f)
	if err != nil || len(buf) == 0 {
		return nil
	}

	// Если читали со смещения в середине файла, пропускаем первый неполный кусок строки
	if offset > 0 {
		if idx := bytes.IndexByte(buf, '\n'); idx != -1 {
			buf = buf[idx+1:]
		}
	}

	return bytes.TrimSpace(buf)
}

func formatRecentTurns(raw []byte, maxTurns int) string {
	if len(raw) == 0 {
		return ""
	}

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return ""
	}

	if maxTurns > 0 && len(lines) > maxTurns {
		lines = lines[len(lines)-maxTurns:]
	}

	var formattedTurns []string
	for _, l := range lines {
		var entry TranscriptEntry
		if err := json.Unmarshal([]byte(l), &entry); err == nil && (entry.Type != "" || entry.Content != "") {
			role := "STEP"
			if entry.Type == "USER_INPUT" || entry.Source == "USER_EXPLICIT" {
				role = "USER"
			} else if entry.Type == "PLANNER_RESPONSE" || entry.Source == "MODEL" {
				role = "ASSISTANT"
			}

			content := cleanContent(strings.TrimSpace(entry.Content))
			if len(content) > 500 {
				content = content[:500] + "... (truncated)"
			}
			if content != "" {
				formattedTurns = append(formattedTurns, fmt.Sprintf("[%s]: %s", role, content))
			}
		} else {
			trimmed := l
			if len(trimmed) > 300 {
				trimmed = trimmed[:300] + "..."
			}
			formattedTurns = append(formattedTurns, trimmed)
		}
	}

	return strings.Join(formattedTurns, "\n")
}

func cleanContent(s string) string {
	// Убираем обертки тегов вида <USER_REQUEST> и т.д.
	s = strings.ReplaceAll(s, "<USER_REQUEST>", "")
	s = strings.ReplaceAll(s, "</USER_REQUEST>", "")
	return strings.TrimSpace(s)
}
