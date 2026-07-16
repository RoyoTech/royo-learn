package main

import (
	"bytes"
	"testing"
	"time"

	"agent-royo-learn/internal/buildinfo"
)

// TestRunSelfUpdateDevBuildRefused pins the CLI dispatch contract: on a
// development build (test binaries always report buildinfo.Version ==
// "dev") an implicit self-update must fail with the development_build
// envelope. The dev guard fires before any HTTP request in Update(), so
// the command returns without touching the network; the guard's
// no-network behavior itself is pinned by
// TestUpdateRefusesDevBuildWithoutExplicitVersion in internal/selfupdate.
func TestRunSelfUpdateDevBuildRefused(t *testing.T) {
	if buildinfo.Version != "dev" {
		t.Skipf("buildinfo.Version = %q, want a dev test build", buildinfo.Version)
	}

	start := time.Now()
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"self-update"}, &stdout, &stderr)
	elapsed := time.Since(start)

	if exitCode != exitFailure {
		t.Fatalf("self-update on dev build exit = %d, want %d", exitCode, exitFailure)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	assertErrorEnvelopeCode(t, stderr.Bytes(), "development_build")
	// A refusal that reached the GitHub API would take a network
	// round-trip; the guard must return effectively instantly.
	if elapsed > 2*time.Second {
		t.Errorf("dev-build refusal took %s, expected an immediate local error", elapsed)
	}
}

func TestRunSelfUpdateCheckRejectsExplicitVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"self-update", "--check", "--version", "v0.2.0"}, &stdout, &stderr)

	// invalid_argument maps to exit 2 (docs/04 §Exit codes), derived from the
	// error code by the one error model (§4.3).
	if exitCode != exitInvalidArguments {
		t.Fatalf("self-update --check --version exit = %d, want %d", exitCode, exitInvalidArguments)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	assertErrorEnvelopeCode(t, stderr.Bytes(), "invalid_argument")
	if !bytes.Contains(stderr.Bytes(), []byte("--check cannot be combined with --version")) {
		t.Errorf("stderr = %q, want it to explain the --check/--version exclusivity", stderr.String())
	}
}
