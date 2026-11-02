package classifier

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionStateStore хранит историю отказов команд по сессиям.
type SessionStateStore struct {
	Version   int                       `json:"version"`
	Sessions  map[string]map[string]int `json:"sessions"` // sessionID -> commandClass -> count
	UpdatedAt int64                     `json:"updated_at"`
}

// LoopBreaker отслеживает повторные попытки выполнения заблокированных команд
// и принудительно эскалирует вердикт в "ask" для предотвращения зацикливания агента.
type LoopBreaker struct {
	filePath string
	mu       sync.Mutex
}

// DefaultSessionStatePath возвращает путь к ~/.agy-guard/session_state.json.
func DefaultSessionStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".agy-guard", "session_state.json"), nil
}

// NewLoopBreaker создает новый экземпляр LoopBreaker.
func NewLoopBreaker(filePath string) *LoopBreaker {
	return &LoopBreaker{
		filePath: filePath,
	}
}

// NormalizeCommandClass нормализует строку команды до базового 2-токенного класса действия.
// Например:
//   "git push origin main"     -> "git push"
//   "git push -u origin HEAD"  -> "git push"
//   "rm -rf ./dist"            -> "rm -rf"
//   "Remove-Item -Recurse dir" -> "remove-item -recurse"
//   "reboot"                   -> "reboot"
func NormalizeCommandClass(cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return ""
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}

	// Пропускаем переменные окружения вида VAR=val
	for len(fields) > 1 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-") && !strings.HasPrefix(fields[0], "/") {
		fields = fields[1:]
	}

	// Пропускаем sudo
	if len(fields) > 1 && strings.ToLower(fields[0]) == "sudo" {
		fields = fields[1:]
	}

	if len(fields) == 1 {
		return strings.ToLower(fields[0])
	}

	return strings.ToLower(fields[0] + " " + fields[1])
}

func (b *LoopBreaker) loadStore() SessionStateStore {
	store := SessionStateStore{
		Version:  1,
		Sessions: make(map[string]map[string]int),
	}

	if b.filePath == "" {
		return store
	}

	data, err := os.ReadFile(b.filePath)
	if err != nil {
		return store
	}

	if err := json.Unmarshal(data, &store); err != nil || store.Sessions == nil {
		store.Sessions = make(map[string]map[string]int)
	}
	return store
}

func (b *LoopBreaker) saveStore(store SessionStateStore) error {
	if b.filePath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(b.filePath), 0700); err != nil {
		return fmt.Errorf("create session state dir: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session state: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", b.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write tmp session state: %w", err)
	}

	if err := os.Rename(tmpFile, b.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename session state file: %w", err)
	}

	return nil
}

// RecordDeny регистрирует отказ (deny) для команды в рамках сессии и возвращает обновленный счетчик.
func (b *LoopBreaker) RecordDeny(sessionID, cmd string) (int, string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cmdClass := NormalizeCommandClass(cmd)
	if cmdClass == "" {
		return 0, ""
	}

	if strings.TrimSpace(sessionID) == "" {
		sessionID = "default"
	}

	store := b.loadStore()
	if store.Sessions[sessionID] == nil {
		store.Sessions[sessionID] = make(map[string]int)
	}

	store.Sessions[sessionID][cmdClass]++
	count := store.Sessions[sessionID][cmdClass]
	store.UpdatedAt = time.Now().UnixNano()

	_ = b.saveStore(store)
	return count, cmdClass
}

// GetDenyCount возвращает текущее количество отказов команды в сессии.
func (b *LoopBreaker) GetDenyCount(sessionID, cmd string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	cmdClass := NormalizeCommandClass(cmd)
	if cmdClass == "" {
		return 0
	}

	if strings.TrimSpace(sessionID) == "" {
		sessionID = "default"
	}

	store := b.loadStore()
	if store.Sessions[sessionID] == nil {
		return 0
	}

	return store.Sessions[sessionID][cmdClass]
}

// RecordDenyAndCheck регистрирует отказ и проверяет, превышен ли лимит повторов (Strike 2+).
// Если count >= maxRetries, возвращает escalate = true.
func (b *LoopBreaker) RecordDenyAndCheck(sessionID, cmd string, maxRetries int) (bool, int, string) {
	if maxRetries <= 0 {
		maxRetries = 2
	}

	count, cmdClass := b.RecordDeny(sessionID, cmd)
	if count >= maxRetries {
		return true, count, cmdClass
	}
	return false, count, cmdClass
}

// Reset очищает состояние сессии (или все сессии при пустом sessionID).
func (b *LoopBreaker) Reset(sessionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	store := b.loadStore()
	if strings.TrimSpace(sessionID) == "" {
		store.Sessions = make(map[string]map[string]int)
	} else {
		delete(store.Sessions, sessionID)
	}
	store.UpdatedAt = time.Now().UnixNano()
	return b.saveStore(store)
}
