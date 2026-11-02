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

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"

type GeminiClassifier struct {
	BaseURL    string
	Model      string
	APIKey     string
	TimeoutSec int
	client     *http.Client
}

func NewGeminiClassifier(model, apiKey string, timeoutSec int) *GeminiClassifier {
	if timeoutSec <= 0 {
		timeoutSec = 8
	}
	if strings.TrimSpace(model) == "" {
		model = "gemini-3.5-flash-lite"
	}
	return &GeminiClassifier{
		BaseURL:    defaultGeminiBaseURL,
		Model:      model,
		APIKey:     strings.TrimSpace(apiKey),
		TimeoutSec: timeoutSec,
		client:     &http.Client{},
	}
}

type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature      float64 `json:"temperature"`
	ResponseMIMEType string  `json:"responseMimeType,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (g *GeminiClassifier) Evaluate(ctx context.Context, cmd string, transcript string) (hook.OutputVerdict, error) {
	if g.APIKey == "" || g.APIKey == "YOUR_GEMINI_API_KEY_HERE" {
		return hook.OutputVerdict{}, fmt.Errorf("gemini api_key is not configured in ~/.agy-guard/config.json")
	}

	evalCtx, cancel := context.WithTimeout(ctx, time.Duration(g.TimeoutSec)*time.Second)
	defer cancel()

	prompt := BuildPrompt(cmd, transcript)
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			Temperature:      0.0,
			ResponseMIMEType: "application/json",
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("marshal gemini request: %w", err)
	}

	baseURL := g.BaseURL
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, g.Model, g.APIKey)

	httpReq, err := http.NewRequestWithContext(evalCtx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("create gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("gemini http request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("read gemini response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return hook.OutputVerdict{}, fmt.Errorf("gemini returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("unmarshal gemini response: %w", err)
	}

	if geminiResp.Error != nil {
		return hook.OutputVerdict{}, fmt.Errorf("gemini error (%d): %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return hook.OutputVerdict{}, fmt.Errorf("empty candidates returned from gemini")
	}

	rawVerdictJSON := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
	var verdict hook.OutputVerdict
	if err := json.Unmarshal([]byte(rawVerdictJSON), &verdict); err != nil {
		return hook.OutputVerdict{}, fmt.Errorf("parse verdict json from gemini: %w (raw: %q)", err, rawVerdictJSON)
	}

	if verdict.Decision != hook.DecisionAllow && verdict.Decision != hook.DecisionAsk && verdict.Decision != hook.DecisionDeny {
		verdict.Decision = hook.DecisionAsk
	}

	return verdict, nil
}
