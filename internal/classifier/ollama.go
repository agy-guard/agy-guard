package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agy-guard/agy-guard/internal/hook"
)

type OllamaClassifier struct {
	BaseURL    string
	Model      string
	TimeoutSec int
	client     *http.Client
}

func NewOllamaClassifier(baseURL, model string, timeoutSec int) *OllamaClassifier {
	if timeoutSec <= 0 {
		timeoutSec = 8
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if strings.TrimSpace(model) == "" {
		model = "llama3.2"
	}
	return &OllamaClassifier{
		BaseURL:    baseURL,
		Model:      model,
		TimeoutSec: timeoutSec,
		client:     &http.Client{},
	}
}

type ollamaRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	Stream  bool          `json:"stream"`
	Format  string        `json:"format"`
	Options ollamaOptions `json:"options"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

func (o *OllamaClassifier) Evaluate(ctx context.Context, cmd string, transcript string) (hook.OutputVerdict, error) {
	evalCtx, cancel := context.WithTimeout(ctx, time.Duration(o.TimeoutSec)*time.Second)
	defer cancel()

	prompt := BuildPrompt(cmd, transcript)
	reqBody := ollamaRequest{
		Model:  o.Model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
		Options: ollamaOptions{
			Temperature: 0.0,
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("marshal ollama request: %w", err)
	}

	url := fmt.Sprintf("%s/api/generate", o.BaseURL)
	httpReq, err := http.NewRequestWithContext(evalCtx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("ollama http request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("read ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return hook.OutputVerdict{}, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(bodyBytes, &ollamaResp); err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("unmarshal ollama response: %w", err)
	}

	if ollamaResp.Error != "" {
		return hook.OutputVerdict{}, fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	rawVerdictJSON := strings.TrimSpace(ollamaResp.Response)
	if rawVerdictJSON == "" {
		return hook.OutputVerdict{}, fmt.Errorf("empty response received from ollama")
	}

	var verdict hook.OutputVerdict
	if err := json.Unmarshal([]byte(rawVerdictJSON), &verdict); err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("parse verdict json from ollama: %w (raw: %q)", err, rawVerdictJSON)
	}

	if verdict.Decision != hook.DecisionAllow && verdict.Decision != hook.DecisionAsk && verdict.Decision != hook.DecisionDeny {
		verdict.Decision = hook.DecisionAsk
	}

	return verdict, nil
}
