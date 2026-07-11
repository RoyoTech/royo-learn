package config

import "testing"

func TestDefaultConfigProvidesEvidenceCommandGuardrails(t *testing.T) {
	limits := DefaultConfig().Limits
	if limits.EvidenceBytes != DefaultMaxPayloadBytes {
		t.Fatalf("EvidenceBytes = %d, want %d", limits.EvidenceBytes, DefaultMaxPayloadBytes)
	}
	if limits.CommandOutputBytes != DefaultMaxPayloadBytes {
		t.Fatalf("CommandOutputBytes = %d, want %d", limits.CommandOutputBytes, DefaultMaxPayloadBytes)
	}
	if limits.CommandInputBytes != DefaultMaxPayloadBytes {
		t.Fatalf("CommandInputBytes = %d, want %d", limits.CommandInputBytes, DefaultMaxPayloadBytes)
	}
	if limits.CommandTimeoutSeconds != 60 {
		t.Fatalf("CommandTimeoutSeconds = %d, want 60", limits.CommandTimeoutSeconds)
	}

	cfg := DefaultConfig()
	cfg.Merge(&Config{Limits: Limits{
		EvidenceBytes:         8,
		CommandOutputBytes:    9,
		CommandInputBytes:     10,
		CommandTimeoutSeconds: 11,
	}})
	if cfg.Limits.EvidenceBytes != 8 || cfg.Limits.CommandOutputBytes != 9 ||
		cfg.Limits.CommandInputBytes != 10 || cfg.Limits.CommandTimeoutSeconds != 11 {
		t.Fatalf("merged limits = %#v", cfg.Limits)
	}
}
