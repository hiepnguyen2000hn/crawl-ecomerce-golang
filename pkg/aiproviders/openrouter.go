package aiproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// OpenRouter implements Provider against the OpenRouter chat completions API.
type OpenRouter struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewOpenRouter(apiKey, model, baseURL string, httpClient *http.Client) *OpenRouter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenRouter{apiKey: apiKey, model: model, baseURL: baseURL, httpClient: httpClient}
}

type openRouterRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

type openRouterResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (o *OpenRouter) Complete(ctx context.Context, prompt string) (Result, error) {
	reqBody := openRouterRequest{Model: o.model}
	reqBody.Messages = []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{{Role: "user", Content: prompt}}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Result{}, fmt.Errorf("aiproviders: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("aiproviders: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("aiproviders: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("aiproviders: openrouter returned status %d", resp.StatusCode)
	}

	var out openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, fmt.Errorf("aiproviders: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return Result{}, fmt.Errorf("aiproviders: openrouter returned no choices")
	}
	return Result{Model: out.Model, Content: out.Choices[0].Message.Content}, nil
}
