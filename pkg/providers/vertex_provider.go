// Weaver - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Weaver contributors

package providers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// VertexAIProvider wraps HTTPProvider but auto-refreshes GCP access tokens
// using Application Default Credentials (ADC). This eliminates the need
// for manual token management when using Vertex AI endpoints.
type VertexAIProvider struct {
	base        *HTTPProvider
	tokenSource oauth2.TokenSource
	mu          sync.RWMutex
}

// NewVertexAIProvider creates a provider that uses GCP ADC for authentication.
// The apiBase should be the Vertex AI endpoint (e.g.,
// https://us-central1-aiplatform.googleapis.com/v1beta1/projects/PROJECT/locations/REGION/endpoints/openapi)
func NewVertexAIProvider(apiBase, proxy string) (*VertexAIProvider, error) {
	ctx := context.Background()

	// Find default credentials with the cloud-platform scope
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to find GCP default credentials: %w", err)
	}

	ts := oauth2.ReuseTokenSource(nil, creds.TokenSource)

	// Get an initial token to verify credentials work
	token, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get initial GCP access token: %w", err)
	}

	base := NewHTTPProvider(token.AccessToken, apiBase, proxy)

	return &VertexAIProvider{
		base:        base,
		tokenSource: ts,
	}, nil
}

func (v *VertexAIProvider) refreshToken() error {
	token, err := v.tokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to refresh GCP access token: %w", err)
	}

	v.mu.Lock()
	v.base.apiKey = token.AccessToken
	v.mu.Unlock()

	return nil
}

func (v *VertexAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	// Refresh token before each call (ReuseTokenSource caches valid tokens,
	// so this is effectively a no-op unless the token is near expiry)
	if err := v.refreshToken(); err != nil {
		return nil, err
	}

	v.mu.RLock()
	base := v.base
	v.mu.RUnlock()

	// Retry once on auth failure in case of race condition
	resp, err := base.Chat(ctx, messages, tools, model, options)
	if err != nil && isAuthError(err) {
		// Force a fresh token
		if refreshErr := v.forceRefresh(); refreshErr != nil {
			return nil, refreshErr
		}
		v.mu.RLock()
		base = v.base
		v.mu.RUnlock()
		return base.Chat(ctx, messages, tools, model, options)
	}

	return resp, err
}

func (v *VertexAIProvider) forceRefresh() error {
	// Create a new token source to bypass the cache
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return fmt.Errorf("failed to refresh GCP credentials: %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to get fresh GCP token: %w", err)
	}

	v.mu.Lock()
	v.base.apiKey = token.AccessToken
	v.tokenSource = oauth2.ReuseTokenSource(token, creds.TokenSource)
	v.mu.Unlock()

	return nil
}

func (v *VertexAIProvider) GetDefaultModel() string {
	return ""
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "401") || contains(s, "403") || contains(s, "UNAUTHENTICATED") || contains(s, "PERMISSION_DENIED")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
