package engram

import "context"

// DegradedClient wraps a Client and ensures graceful degradation.
// Before every operation, it checks health. If Engram is unavailable,
// it returns empty/neutral results instead of errors. This satisfies
// the acceptance criterion "Engram apagado no impide capture".
type DegradedClient struct {
	inner Client
}

// NewDegradedClient wraps inner with health-aware degradation.
func NewDegradedClient(inner Client) *DegradedClient {
	return &DegradedClient{inner: inner}
}

// Health delegates to the inner client.
func (c *DegradedClient) Health(ctx context.Context) (HealthResult, error) {
	result, err := c.inner.Health(ctx)
	if err != nil {
		return HealthResult{Status: HealthUnavailable, Message: err.Error()}, nil
	}
	return result, nil
}

// Search delegates to the inner client if available; returns empty results otherwise.
func (c *DegradedClient) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if !c.isAvailable(ctx) {
		return nil, nil
	}
	return c.inner.Search(ctx, query)
}

// Context delegates to the inner client if available; returns empty context otherwise.
func (c *DegradedClient) Context(ctx context.Context) (*ContextResult, error) {
	if !c.isAvailable(ctx) {
		return &ContextResult{}, nil
	}
	return c.inner.Context(ctx)
}

// Save delegates if available; returns a degraded result otherwise.
func (c *DegradedClient) Save(ctx context.Context, input *SaveInput) (*SaveResult, error) {
	if !c.isAvailable(ctx) {
		return &SaveResult{Saved: false, Degraded: true}, nil
	}
	return c.inner.Save(ctx, input)
}

// isAvailable returns true when Engram is reachable (healthy or degraded).
func (c *DegradedClient) isAvailable(ctx context.Context) bool {
	result, err := c.inner.Health(ctx)
	if err != nil {
		return false
	}
	return result.IsAvailable()
}
