package engram

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

// HTTPClient implements Client by connecting to Engram's HTTP API.
// When the API is unreachable, it returns HealthUnavailable, and all
// operations degrade gracefully.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPClient creates a client targeting the Engram HTTP API at baseURL.
// baseURL should be like "http://localhost:8765". Uses a 2-second timeout.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// Health checks Engram reachability by attempting a TCP dial to the host.
// If the host is reachable (even if Engram HTTP itself errors), it returns
// HealthDegraded. A connection-refused error returns HealthUnavailable.
func (c *HTTPClient) Health(ctx context.Context) (HealthResult, error) {
	host, err := hostFromURL(c.baseURL)
	if err != nil {
		return HealthResult{Status: HealthUnavailable, Message: "invalid base URL"}, nil
	}

	conn, dialErr := net.DialTimeout("tcp", host, 2*time.Second)
	if dialErr != nil {
		return HealthResult{Status: HealthUnavailable, Message: "engram unreachable: " + dialErr.Error()}, nil
	}
	conn.Close()

	// TCP reachable, try the health endpoint.
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return HealthResult{Status: HealthDegraded, Message: "cannot build health request"}, nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return HealthResult{Status: HealthDegraded, Message: "engram reachable but health endpoint failed: " + err.Error()}, nil
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return HealthResult{Status: HealthHealthy, Message: "engram healthy"}, nil
	}
	return HealthResult{Status: HealthDegraded, Message: "engram health endpoint returned " + resp.Status}, nil
}

// Search is a stub — currently not implemented via HTTP for v1.
func (c *HTTPClient) Search(ctx context.Context, query string) ([]SearchResult, error) {
	return nil, errors.New("engram HTTP search not implemented")
}

// Context is a stub — currently not implemented via HTTP for v1.
func (c *HTTPClient) Context(ctx context.Context) (*ContextResult, error) {
	return &ContextResult{}, errors.New("engram HTTP context not implemented")
}

// Save is a stub — currently not implemented via HTTP for v1.
func (c *HTTPClient) Save(ctx context.Context, input *SaveInput) (*SaveResult, error) {
	return &SaveResult{Saved: false}, errors.New("engram HTTP save not implemented")
}

// hostFromURL extracts the host:port from an HTTP URL.
func hostFromURL(rawURL string) (string, error) {
	s := rawURL
	if after, found := strings.CutPrefix(s, "http://"); found {
		s = after
	} else if after, found := strings.CutPrefix(s, "https://"); found {
		s = after
	} else {
		return "", errors.New("unsupported URL scheme")
	}
	if s == "" {
		return "", errors.New("empty host")
	}
	return s, nil
}
