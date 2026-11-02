package classifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agy-guard/agy-guard/internal/hook"
)

func TestOllamaClassifier_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}
		resp := `{
			"model": "llama3.2",
			"response": "{\"decision\": \"ask\", \"reason\": \"Modifies packages\"}",
			"done": true
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	o := NewOllamaClassifier(ts.URL, "llama3.2", 5)

	verdict, err := o.Evaluate(context.Background(), "npm install express", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.Decision != hook.DecisionAsk {
		t.Errorf("expected ask, got %v", verdict.Decision)
	}
	if verdict.Reason != "Modifies packages" {
		t.Errorf("unexpected reason: %q", verdict.Reason)
	}
}

func TestOllamaClassifier_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "model 'llama3.2' not found"}`))
	}))
	defer ts.Close()

	o := NewOllamaClassifier(ts.URL, "llama3.2", 5)

	_, err := o.Evaluate(context.Background(), "ls", "")
	if err == nil {
		t.Fatalf("expected error on HTTP 404, got nil")
	}
}

func TestOllamaClassifier_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{
			"model": "llama3.2",
			"response": "plain-text-not-json",
			"done": true
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	o := NewOllamaClassifier(ts.URL, "llama3.2", 5)

	_, err := o.Evaluate(context.Background(), "ls", "")
	if err == nil {
		t.Fatalf("expected error on malformed json, got nil")
	}
}

func TestOllamaClassifier_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	o := NewOllamaClassifier(ts.URL, "llama3.2", 5)

	_, err := o.Evaluate(ctx, "ls", "")
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}
