package engram

import (
	"errors"
	"testing"
)

// TestClientInterface validates the Client interface contract.
// These tests exercise the types defined in client.go.
// RED phase: client.go does not exist yet or has incomplete types.
// The test MUST compile against the real production file.

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status HealthStatus
		want   string
	}{
		{name: "healthy", status: HealthHealthy, want: "healthy"},
		{name: "degraded", status: HealthDegraded, want: "degraded"},
		{name: "unavailable", status: HealthUnavailable, want: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("HealthStatus.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHealthResult_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		result HealthResult
		want   bool
	}{
		{name: "healthy", result: HealthResult{Status: HealthHealthy}, want: true},
		{name: "degraded", result: HealthResult{Status: HealthDegraded}, want: false},
		{name: "unavailable", result: HealthResult{Status: HealthUnavailable}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsHealthy(); got != tt.want {
				t.Errorf("HealthResult.IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHealthResult_IsAvailable(t *testing.T) {
	tests := []struct {
		name   string
		result HealthResult
		want   bool
	}{
		{name: "healthy", result: HealthResult{Status: HealthHealthy}, want: true},
		{name: "degraded", result: HealthResult{Status: HealthDegraded}, want: true},
		{name: "unavailable", result: HealthResult{Status: HealthUnavailable}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsAvailable(); got != tt.want {
				t.Errorf("HealthResult.IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDegradedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "degraded error", err: &DegradedError{Operation: "health"}, want: true},
		{name: "nil", err: nil, want: false},
		{name: "standard error", err: errors.New("something"), want: false},
		{name: "wrapped degraded", err: &DegradedError{Operation: "search", Cause: errors.New("timeout")}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDegradedError(tt.err)
			if got != tt.want {
				t.Errorf("IsDegradedError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDegradedError_Error(t *testing.T) {
	err := &DegradedError{Operation: "search", Cause: errors.New("timeout")}
	msg := err.Error()
	if msg == "" {
		t.Error("DegradedError.Error() returned empty string")
	}
	if !containsStr(msg, "search") {
		t.Errorf("DegradedError.Error() = %q, should contain 'search'", msg)
	}
}

func TestDegradedError_Unwrap(t *testing.T) {
	cause := errors.New("timeout")
	err := &DegradedError{Operation: "save", Cause: cause}
	if !errors.Is(err, cause) {
		t.Error("DegradedError.Unwrap() should return the cause")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
