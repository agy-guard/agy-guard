package classifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agy-guard/agy-guard/internal/hook"
)

func TestGeminiClassifier_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("key") != "secret-key" {
			t.Errorf("expected key query param, got %s", r.URL.Query().Get("key"))
		}
		resp := `{
			"candidates": [
				{
					"content": {
						"parts": [
							{"text": "{\"decision\": \"allow\", \"reason\": \"Harmless test command\"}"}
						]
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
	defer ts.Close()

	g := NewGeminiClassifier("gemini-3.8-flash", "secret-key", 5)
	g.BaseURL = ts.URL

	verdict, err := g.Evaluate(context.Background(), "go test ./...", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.Decision != hook.DecisionAllow {
		t.Errorf("expected allow, got %v", verdict.Decision)
	}
	if verdict.Reason != "Harmless test command" {
		t.Errorf("unexpected reason: %q", verdict.Reason)
	}
}

func TestGeminiClassifier_MissingAPIKey(t *testing.T) {
	g := NewGeminiClassifier("gemini-3.8-flash", "YOUR_GEMINI_API_KEY_HERE", 5)
	_, err := g.Evaluate(context.Background(), "ls", "")
	if err == nil {
		t.Fatalf("expected error for unconfigured API key, got nil")
	}
}

func TestGeminiClassifier_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"code": 500, "message": "internal error"}}`))
	}))
	defer ts.Close()

	g := NewGeminiClassifier("gemini-3.8-flash", "valid-key", 5)
	g.BaseURL = ts.URL

	_, err := g.Evaluate(context.Background(), "ls", "")
	if err == nil {
		t.Fatalf("expected error on HTTP 500, got nil")
	}
}

func TestGeminiClassifier_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{
			"candidates": [
				{
					"content": {
						"parts": [
							{"text": "not-valid-json"}
						]
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
	defer ts.Close()

	g := NewGeminiClassifier("gemini-3.8-flash", "valid-key", 5)
	g.BaseURL = ts.URL

	_, err := g.Evaluate(context.Background(), "ls", "")
	if err == nil {
		t.Fatalf("expected error on malformed model json, got nil")
	}
}

func TestGeminiClassifier_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	g := NewGeminiClassifier("gemini-3.8-flash", "valid-key", 5)
	g.BaseURL = ts.URL

	_, err := g.Evaluate(ctx, "ls", "")
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}
