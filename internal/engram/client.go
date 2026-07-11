// Package engram provides an integration client for Engram persistent memory.
// It offers a Client interface with health-aware operations and graceful
// degradation when Engram is unreachable. The package implements Rule #6
// and Rule #7 from AGENTS.md: never access the SQLite DB directly and
// integrate only via Engram's public local API.
package engram

import (
	"context"
	"errors"
	"fmt"
)

// HealthStatus describes the reachability of the Engram service.
type HealthStatus int

const (
	// HealthHealthy means Engram is reachable and responsive.
	HealthHealthy HealthStatus = iota
	// HealthDegraded means Engram is reachable but slow or partial.
	HealthDegraded
	// HealthUnavailable means Engram is not reachable.
	HealthUnavailable
)

// String returns a human-readable representation of the health status.
func (s HealthStatus) String() string {
	switch s {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	default:
		return "unavailable"
	}
}

// HealthResult describes the outcome of a health check.
type HealthResult struct {
	Status  HealthStatus
	Message string
}

// IsHealthy returns true when Engram is fully healthy.
func (r HealthResult) IsHealthy() bool { return r.Status == HealthHealthy }

// IsAvailable returns true when Engram is reachable (healthy or degraded).
func (r HealthResult) IsAvailable() bool {
	return r.Status == HealthHealthy || r.Status == HealthDegraded
}

// DegradedError signals that an operation was skipped because Engram is
// unavailable. It carries the operation name and optional cause.
type DegradedError struct {
	Operation string
	Cause     error
}

// Error implements the error interface.
func (e *DegradedError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("engram: %s degraded: %v", e.Operation, e.Cause)
	}
	return fmt.Sprintf("engram: %s degraded (service unavailable)", e.Operation)
}

// Unwrap returns the underlying cause.
func (e *DegradedError) Unwrap() error { return e.Cause }

// IsDegradedError reports whether err is or wraps a DegradedError.
func IsDegradedError(err error) bool {
	var de *DegradedError
	return errors.As(err, &de)
}

// Client is the interface for Engram memory operations.
// All methods accept a context and return structured results.
// Implementations must degrade gracefully when Engram is unreachable.
type Client interface {
	// Health checks Engram reachability and returns status.
	Health(ctx context.Context) (HealthResult, error)
	// Search queries Engram for relevant context matching the query.
	Search(ctx context.Context, query string) ([]SearchResult, error)
	// Context fetches recent session context from Engram.
	Context(ctx context.Context) (*ContextResult, error)
	// Save optionally persists a learning to Engram if available.
	Save(ctx context.Context, input *SaveInput) (*SaveResult, error)
}

// SearchResult represents a single Engram search hit.
type SearchResult struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content string  `json:"content,omitempty"`
	Score   float64 `json:"score"`
	Type    string  `json:"type,omitempty"`
}

// SessionSummary is a condensed view of a past Engram session.
type SessionSummary struct {
	ID          string   `json:"id"`
	Goal        string   `json:"goal"`
	Discoveries []string `json:"discoveries,omitempty"`
}

// ContextResult wraps recent session context from Engram.
type ContextResult struct {
	RecentSessions []SessionSummary `json:"recent_sessions"`
}

// SaveInput is the payload for saving a learning to Engram.
type SaveInput struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Type    string `json:"type,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

// SaveResult reports the outcome of a save operation.
type SaveResult struct {
	Saved    bool   `json:"saved"`
	ID       string `json:"id,omitempty"`
	Degraded bool   `json:"degraded,omitempty"`
}
