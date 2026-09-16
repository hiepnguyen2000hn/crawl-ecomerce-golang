package aiproviders

import (
	"context"
	"encoding/json"
)

// Result is a normalized response from any AI provider.
type Result struct {
	Model   string
	Content string
}

// Provider is implemented by each AI backend (OpenRouter, TokenRouter, ...).
type Provider interface {
	Complete(ctx context.Context, prompt string) (Result, error)

	// CompleteJSON asks the model to respond strictly conforming to the
	// given JSON Schema (via the provider's structured-output feature) and
	// returns the raw JSON response content.
	CompleteJSON(ctx context.Context, prompt string, schemaName string, schema json.RawMessage) (json.RawMessage, error)
}
