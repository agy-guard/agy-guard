package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// DefaultMaxTranscriptBytes — максимальный объем читаемого хвоста лога (2 MB).
	DefaultMaxTranscriptBytes int64 = 2 * 1024 * 1024
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
	return string(ReadRawTailBytes(path, maxBytes))
}

// ReadRawTailBytes читает до maxBytes байт с конца файла в виде сырых байт.
func ReadRawTailBytes(path string, maxBytes int64) []byte {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxTranscriptBytes
	}
	return readRawTail(path, maxBytes)
}

func readRawTail(path string, maxBytes int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() {
		_ = f.Close()
	}()

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

// ExtractLatestUserRequest находит последнюю команду или запрос пользователя в переданных сырых байтах транскрипта.
func ExtractLatestUserRequest(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	lines := bytes.Split(raw, []byte{'\n'})

	// Ищем с конца первое сообщение от пользователя
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var entry TranscriptEntry
		if err := json.Unmarshal(line, &entry); err == nil {
			if entry.Type == "USER_INPUT" || entry.Source == "USER_EXPLICIT" {
				content := cleanContent(strings.TrimSpace(entry.Content))
				if content != "" {
					if len(content) > 1000 {
						content = content[:1000] + "... (truncated)"
					}
					return content
				}
			}
		}
	}

	return ""
}

func formatRecentTurns(raw []byte, maxTurns int) string {
	if len(raw) == 0 {
		return ""
	}

	rawLines := bytes.Split(raw, []byte{'\n'})
	var lines []string
	for _, l := range rawLines {
		trimmed := strings.TrimSpace(string(l))
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	if len(lines) == 0 {
		return ""
	}

	latestUserReq := ExtractLatestUserRequest(raw)

	recentLines := lines
	if maxTurns > 0 && len(recentLines) > maxTurns {
		recentLines = recentLines[len(recentLines)-maxTurns:]
	}

	var formattedTurns []string
	for _, l := range recentLines {
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

	turnsText := strings.Join(formattedTurns, "\n")
	if latestUserReq != "" {
		if len(formattedTurns) > 0 {
			return fmt.Sprintf("Latest User Request:\n%s\n\nRecent Turns:\n%s", latestUserReq, turnsText)
		}
		return fmt.Sprintf("Latest User Request:\n%s", latestUserReq)
	}

	return turnsText
}

func cleanContent(s string) string {
	// Убираем обертки тегов вида <USER_REQUEST> и т.д.
	s = strings.ReplaceAll(s, "<USER_REQUEST>", "")
	s = strings.ReplaceAll(s, "</USER_REQUEST>", "")
	return strings.TrimSpace(s)
}
