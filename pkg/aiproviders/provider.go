package aiproviders

import "context"

// Result is a normalized response from any AI provider.
type Result struct {
	Model   string
	Content string
}

// Provider is implemented by each AI backend (OpenRouter, TokenRouter, ...).
type Provider interface {
	Complete(ctx context.Context, prompt string) (Result, error)
}
