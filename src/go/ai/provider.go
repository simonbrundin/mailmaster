package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"mailagent/config"
)

// Provider defines the interface for AI providers
type Provider interface {
	Process(ctx context.Context, prompt string) (*Response, error)
}

// Response represents the AI's decision
type Response struct {
	Action     string                 `json:"action"`       // "read", "archive", "move", "forward", "reply", "uncertain"
	Motivation string                 `json:"motivation"`   // Why this action was chosen
	ActionArgs map[string]interface{} `json:"action_args"`  // Additional args for the action
	Confidence float64                `json:"confidence"`   // 0.0 - 1.0
}

// OpenAIProvider implements Provider using OpenAI-compatible API
type OpenAIProvider struct {
	Endpoint        string
	Model          string
	ReasoningEffort string // "low", "medium", "high" (GPT-5.6 Luna)
	APIKey          string
	HTTPClient      *http.Client
}

func NewOpenAIProvider(cfg *config.AIConfig, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		Endpoint:        cfg.Endpoint,
		Model:          cfg.Model,
		ReasoningEffort: cfg.ReasoningEffort,
		APIKey:          apiKey,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// --- API request/response types ---

type chatRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type responsesRequest struct {
	Model     string `json:"model"`
	Input     string `json:"input"`
	Reasoning *struct {
		Effort string `json:"effort,omitempty"`
	} `json:"reasoning,omitempty"`
}

type responsesResponse struct {
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// --- Process entry point ---

func (p *OpenAIProvider) Process(ctx context.Context, prompt string) (*Response, error) {
	if p.ReasoningEffort != "" {
		return p.processResponses(ctx, prompt)
	}
	return p.processChat(ctx, prompt)
}

func (p *OpenAIProvider) processChat(ctx context.Context, prompt string) (*Response, error) {
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": "Du är en mailhanterare. Svara ENDAST i det begärda formatet utan förklaringar.",
		},
		{
			"role":    "user",
			"content": prompt,
		},
	}

	var chatResp chatResponse
	if err := p.doJSONRequest(ctx, p.Endpoint+"/chat/completions", chatRequest{
		Model:    p.Model,
		Messages: messages,
		Stream:   false,
	}, nil, &chatResp); err != nil {
		return nil, err
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("AI API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return parseResponse(chatResp.Choices[0].Message.Content), nil
}

func (p *OpenAIProvider) processResponses(ctx context.Context, prompt string) (*Response, error) {
	reqBody := responsesRequest{
		Model: p.Model,
		Input: prompt,
	}
	if p.ReasoningEffort != "" {
		reqBody.Reasoning = &struct {
			Effort string `json:"effort,omitempty"`
		}{Effort: p.ReasoningEffort}
	}

	var apiResp responsesResponse
	if err := p.doJSONRequest(ctx, p.Endpoint+"/responses", reqBody, map[string]string{
		"OpenAI-Beta": "responses=2025-02-01",
	}, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("AI API error: %s", apiResp.Error.Message)
	}

	var content string
	for _, output := range apiResp.Output {
		if output.Type == "message" {
			for _, item := range output.Content {
				if item.Type == "output_text" {
					content = item.Text
					break
				}
			}
		}
	}
	if content == "" {
		return nil, fmt.Errorf("no text output in response")
	}

	return parseResponse(content), nil
}

func (p *OpenAIProvider) doJSONRequest(ctx context.Context, url string, body interface{}, extraHeaders map[string]string, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var apiErr struct {
		Error struct{ Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Error.Message != "" {
		return fmt.Errorf("AI API error: %s", apiErr.Error.Message)
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}
