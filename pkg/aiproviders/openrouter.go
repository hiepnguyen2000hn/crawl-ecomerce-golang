package aiproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// OpenRouter implements Provider against the OpenRouter chat completions API.
type OpenRouter struct {
	apiKey     string
	models     []string
	baseURL    string
	httpClient *http.Client
}

// NewOpenRouter builds a client that tries model first, then falls back to
// fallbackModels in order on failure. Free-tier models occasionally get
// overloaded (non-2xx status, or HTTP 200 with an empty choices array), so
// having fallbacks lets a job succeed on a different model instead of
// failing the whole extraction.
func NewOpenRouter(apiKey, model, baseURL string, httpClient *http.Client, fallbackModels ...string) *OpenRouter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	models := append([]string{model}, fallbackModels...)
	return &OpenRouter{apiKey: apiKey, models: models, baseURL: baseURL, httpClient: httpClient}
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

// Complete tries each configured model in order, returning the first
// success. If every model fails, it returns a joined error naming which
// model failed and why (including the response body OpenRouter sent).
func (o *OpenRouter) Complete(ctx context.Context, prompt string) (Result, error) {
	var errs []error
	for _, model := range o.models {
		result, err := o.completeOnce(ctx, model, prompt)
		if err == nil {
			return result, nil
		}
		errs = append(errs, err)
	}
	return Result{}, errors.Join(errs...)
}

func (o *OpenRouter) completeOnce(ctx context.Context, model, prompt string) (Result, error) {
	reqBody := openRouterRequest{Model: model}
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
		return Result{}, fmt.Errorf("aiproviders: model %s: request failed: %w", model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("aiproviders: model %s: openrouter returned status %d: %s", model, resp.StatusCode, readErrorBody(resp.Body))
	}

	var out openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, fmt.Errorf("aiproviders: model %s: decode response: %w", model, err)
	}
	if len(out.Choices) == 0 {
		return Result{}, fmt.Errorf("aiproviders: model %s: openrouter returned no choices", model)
	}
	return Result{Model: out.Model, Content: out.Choices[0].Message.Content}, nil
}

type openRouterJSONRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	ResponseFormat struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Name   string          `json:"name"`
			Strict bool            `json:"strict"`
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	} `json:"response_format"`
}

// CompleteJSON tries each configured model in order, returning the first
// success. If every model fails, it returns a joined error naming which
// model failed and why (including the response body OpenRouter sent).
func (o *OpenRouter) CompleteJSON(ctx context.Context, prompt string, schemaName string, schema json.RawMessage) (json.RawMessage, error) {
	var errs []error
	for _, model := range o.models {
		result, err := o.completeJSONOnce(ctx, model, prompt, schemaName, schema)
		if err == nil {
			return result, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

func (o *OpenRouter) completeJSONOnce(ctx context.Context, model, prompt string, schemaName string, schema json.RawMessage) (json.RawMessage, error) {
	reqBody := openRouterJSONRequest{Model: model}
	reqBody.Messages = []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{{Role: "user", Content: prompt}}
	reqBody.ResponseFormat.Type = "json_schema"
	reqBody.ResponseFormat.JSONSchema.Name = schemaName
	reqBody.ResponseFormat.JSONSchema.Strict = true
	reqBody.ResponseFormat.JSONSchema.Schema = schema

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("aiproviders: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("aiproviders: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aiproviders: model %s: request failed: %w", model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("aiproviders: model %s: openrouter returned status %d: %s", model, resp.StatusCode, readErrorBody(resp.Body))
	}

	var out openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("aiproviders: model %s: decode response: %w", model, err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("aiproviders: model %s: openrouter returned no choices", model)
	}
	return json.RawMessage(out.Choices[0].Message.Content), nil
}

// readErrorBody returns up to 1KB of the response body for inclusion in an
// error message, so failures against OpenRouter's API are diagnosable
// without needing to reproduce them with tracing enabled.
func readErrorBody(body io.Reader) string {
	b, err := io.ReadAll(io.LimitReader(body, 1024))
	if err != nil || len(b) == 0 {
		return "<no response body>"
	}
	return string(b)
}
