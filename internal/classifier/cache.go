package classifier

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agy-guard/agy-guard/internal/hook"
)

// CacheEntry хранит закешированный вердикт классификатора и время создания.
type CacheEntry struct {
	Command   string        `json:"command"`
	Decision  hook.Decision `json:"decision"`
	Reason    string        `json:"reason"`
	CreatedAt int64         `json:"created_at"` // Unix nanoseconds
}

// CacheStore — корневая структура файла кеша ~/.agy-guard/cache.json.
type CacheStore struct {
	Version int                   `json:"version"`
	Entries map[string]CacheEntry `json:"entries"`
}

// Cache управляет локальным кешем решений LLM.
type Cache struct {
	filePath string
	ttl      time.Duration
	mu       sync.RWMutex
}

// DefaultCachePath возвращает путь к ~/.agy-guard/cache.json.
func DefaultCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".agy-guard", "cache.json"), nil
}

// NewCache создает новый экземпляр Cache.
func NewCache(filePath string, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Cache{
		filePath: filePath,
		ttl:      ttl,
	}
}

// ComputeKey вычисляет SHA256-хеш от провайдера, модели и команды.
func ComputeKey(provider, model, cmd string) string {
	h := sha256.New()
	keySource := fmt.Sprintf("%s:%s:%s", strings.TrimSpace(provider), strings.TrimSpace(model), strings.TrimSpace(cmd))
	h.Write([]byte(keySource))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) loadStore() CacheStore {
	store := CacheStore{
		Version: 1,
		Entries: make(map[string]CacheEntry),
	}

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return store
	}

	if err := json.Unmarshal(data, &store); err != nil || store.Entries == nil {
		store.Entries = make(map[string]CacheEntry)
	}
	return store
}

func (c *Cache) saveStore(store CacheStore) error {
	if err := os.MkdirAll(filepath.Dir(c.filePath), 0700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache store: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", c.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write tmp cache file: %w", err)
	}

	if err := os.Rename(tmpFile, c.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename cache file: %w", err)
	}

	return nil
}

// Get проверяет наличие неистекшего вердикта в кеше.
func (c *Cache) Get(provider, model, cmd string) (hook.OutputVerdict, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	store := c.loadStore()
	key := ComputeKey(provider, model, cmd)

	entry, exists := store.Entries[key]
	if !exists {
		return hook.OutputVerdict{}, false
	}

	entryAge := time.Since(time.Unix(0, entry.CreatedAt))
	if entryAge > c.ttl {
		return hook.OutputVerdict{}, false
	}

	reason := entry.Reason
	if !strings.Contains(reason, "(cached)") {
		reason = fmt.Sprintf("%s (cached)", reason)
	}

	return hook.OutputVerdict{
		Decision: entry.Decision,
		Reason:   reason,
	}, true
}

// Put сохраняет решение классификатора в кеш.
func (c *Cache) Put(provider, model, cmd string, verdict hook.OutputVerdict) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	store := c.loadStore()
	key := ComputeKey(provider, model, cmd)

	// Очищаем устаревшие записи, если их накопилось много
	now := time.Now()
	if len(store.Entries) > 500 {
		active := make(map[string]CacheEntry)
		for k, v := range store.Entries {
			if now.Sub(time.Unix(0, v.CreatedAt)) <= c.ttl {
				active[k] = v
			}
		}
		store.Entries = active
	}

	store.Entries[key] = CacheEntry{
		Command:   cmd,
		Decision:  verdict.Decision,
		Reason:    verdict.Reason,
		CreatedAt: now.UnixNano(),
	}

	return c.saveStore(store)
}
