package classifier

import (
	"runtime"
	"strings"
	"testing"

	"github.com/agy-guard/agy-guard/internal/config"
)

func TestBuildPrompt(t *testing.T) {
	cmd := "git push origin main"
	transcript := "[USER]: Deploy latest changes"

	prompt := BuildPrompt(cmd, transcript)

	if !strings.Contains(prompt, runtime.GOOS) {
		t.Errorf("prompt does not contain runtime.GOOS %q", runtime.GOOS)
	}
	if !strings.Contains(prompt, cmd) {
		t.Errorf("prompt does not contain command %q", cmd)
	}
	if !strings.Contains(prompt, transcript) {
		t.Errorf("prompt does not contain transcript context")
	}
	if !strings.Contains(prompt, `"decision": "allow" | "ask" | "deny"`) {
		t.Errorf("prompt does not contain expected JSON schema instructions")
	}
	if !strings.Contains(prompt, "CRITICAL INTENT RULE") {
		t.Errorf("prompt does not contain CRITICAL INTENT RULE")
	}
	if !strings.Contains(prompt, "Latest User Request") {
		t.Errorf("prompt does not reference Latest User Request")
	}
}

func TestNewClassifier(t *testing.T) {
	cfgGemini := &config.Config{
		Provider: "gemini",
		Model:    "gemini-3.8-flash",
		APIKey:   "test-key",
	}
	c1, err := NewClassifier(cfgGemini)
	if err != nil {
		t.Fatalf("unexpected error creating gemini classifier: %v", err)
	}
	if _, ok := c1.(*GeminiClassifier); !ok {
		t.Errorf("expected *GeminiClassifier, got %T", c1)
	}

	cfgOllama := &config.Config{
		Provider:  "ollama",
		Model:     "llama3.2",
		OllamaURL: "http://localhost:11434",
	}
	c2, err := NewClassifier(cfgOllama)
	if err != nil {
		t.Fatalf("unexpected error creating ollama classifier: %v", err)
	}
	if _, ok := c2.(*OllamaClassifier); !ok {
		t.Errorf("expected *OllamaClassifier, got %T", c2)
	}

	cfgInvalid := &config.Config{
		Provider: "unsupported-provider",
	}
	_, err = NewClassifier(cfgInvalid)
	if err == nil {
		t.Fatalf("expected error for unsupported provider, got nil")
	}
}
