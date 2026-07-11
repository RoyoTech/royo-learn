package engram

import (
	"context"
	"sync"
)

// FakeClient is a test double that implements Client with configurable
// responses. It never touches a real Engram instance.
type FakeClient struct {
	mu sync.RWMutex

	health        HealthResult
	searchResults []SearchResult
	searchErr     error
	contextResult *ContextResult
	saveDegraded  bool
}

// NewFakeClient returns a FakeClient with sensible defaults (healthy).
func NewFakeClient() *FakeClient {
	return &FakeClient{
		health:        HealthResult{Status: HealthHealthy},
		contextResult: &ContextResult{},
	}
}

// SetHealth configures the health status and message.
func (c *FakeClient) SetHealth(status HealthStatus, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health = HealthResult{Status: status, Message: message}
}

// SetSearchResults configures what Search returns.
func (c *FakeClient) SetSearchResults(results []SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.searchResults = results
}

// SetSearchError configures Search to return an error.
func (c *FakeClient) SetSearchError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.searchErr = err
}

// SetContextResult configures what Context returns.
func (c *FakeClient) SetContextResult(result *ContextResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contextResult = result
}

// SetSaveDegraded configures Save to report degraded.
func (c *FakeClient) SetSaveDegraded(degraded bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saveDegraded = degraded
}

// Health returns the configured health status.
func (c *FakeClient) Health(_ context.Context) (HealthResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health, nil
}

// Search returns the configured search results or error.
func (c *FakeClient) Search(_ context.Context, _ string) ([]SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.searchErr != nil {
		return nil, c.searchErr
	}
	return c.searchResults, nil
}

// Context returns the configured context result.
func (c *FakeClient) Context(_ context.Context) (*ContextResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.contextResult, nil
}

// Save reports success unless configured as degraded.
func (c *FakeClient) Save(_ context.Context, input *SaveInput) (*SaveResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.saveDegraded {
		return &SaveResult{Saved: false, Degraded: true}, nil
	}
	return &SaveResult{Saved: true}, nil
}
