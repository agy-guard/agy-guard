package classifier

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agy-guard/agy-guard/internal/hook"
)

func TestCache_GetPut(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	c := NewCache(cachePath, 5*time.Minute)

	// Cache miss on empty
	_, hit := c.Get("gemini", "gemini-3.8-flash", "make run-tests")
	if hit {
		t.Fatalf("Expected cache miss, got hit")
	}

	verdict := hook.OutputVerdict{
		Decision: hook.DecisionAllow,
		Reason:   "Safe build script",
	}

	// Cache put
	if err := c.Put("gemini", "gemini-3.8-flash", "make run-tests", verdict); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Cache hit
	cachedVerdict, hit := c.Get("gemini", "gemini-3.8-flash", "make run-tests")
	if !hit {
		t.Fatalf("Expected cache hit, got miss")
	}
	if cachedVerdict.Decision != hook.DecisionAllow {
		t.Errorf("Cached decision = %v, want %v", cachedVerdict.Decision, hook.DecisionAllow)
	}
	if cachedVerdict.Reason != "Safe build script (cached)" {
		t.Errorf("Cached reason = %q, want %q", cachedVerdict.Reason, "Safe build script (cached)")
	}

	// Provider isolation
	_, hit = c.Get("ollama", "gemini-3.8-flash", "make run-tests")
	if hit {
		t.Errorf("Expected cache miss for different provider, got hit")
	}

	// Model isolation
	_, hit = c.Get("gemini", "gemini-1.5-pro", "make run-tests")
	if hit {
		t.Errorf("Expected cache miss for different model, got hit")
	}
}

func TestCache_TTL_Expiration(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	// 50ms TTL
	c := NewCache(cachePath, 50*time.Millisecond)

	verdict := hook.OutputVerdict{
		Decision: hook.DecisionDeny,
		Reason:   "Dangerous script",
	}

	if err := c.Put("gemini", "gemini-3.8-flash", "docker run evil", verdict); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Immediate get -> hit
	_, hit := c.Get("gemini", "gemini-3.8-flash", "docker run evil")
	if !hit {
		t.Fatalf("Expected immediate hit, got miss")
	}

	// Wait for TTL to expire
	time.Sleep(70 * time.Millisecond)

	_, hit = c.Get("gemini", "gemini-3.8-flash", "docker run evil")
	if hit {
		t.Fatalf("Expected cache miss after TTL expiration, got hit")
	}
}

func TestCache_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	if err := os.WriteFile(cachePath, []byte("NOT_A_VALID_JSON"), 0600); err != nil {
		t.Fatalf("Write corrupted file: %v", err)
	}

	c := NewCache(cachePath, 5*time.Minute)

	// Must not panic, must return cache miss
	_, hit := c.Get("gemini", "gemini-3.8-flash", "ls -la")
	if hit {
		t.Errorf("Expected cache miss on corrupted file, got hit")
	}

	// Must recover and allow Put
	verdict := hook.OutputVerdict{Decision: hook.DecisionAllow, Reason: "Ok"}
	if err := c.Put("gemini", "gemini-3.8-flash", "ls -la", verdict); err != nil {
		t.Fatalf("Put on corrupted cache should recover: %v", err)
	}

	_, hit = c.Get("gemini", "gemini-3.8-flash", "ls -la")
	if !hit {
		t.Errorf("Expected cache hit after recovery, got miss")
	}
}
