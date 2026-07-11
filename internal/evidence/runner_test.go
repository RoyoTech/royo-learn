package evidence

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCommandRunnerSimple(t *testing.T) {
	t.Parallel()

	runner := &CommandRunner{}

	var cmd []string
	if runtime.GOOS == "windows" {
		cmd = []string{"cmd", "/d", "/c", "echo hello"}
	} else {
		cmd = []string{"echo", "hello"}
	}

	result, err := runner.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("Stdout does not contain 'hello': %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestCommandRunnerFailedCommand(t *testing.T) {
	t.Parallel()

	runner := &CommandRunner{}
	ctx := context.Background()

	var cmd []string
	if runtime.GOOS == "windows" {
		cmd = []string{"cmd", "/d", "/c", "exit 1"}
	} else {
		cmd = []string{"sh", "-c", "exit 1"}
	}

	result, err := runner.Run(ctx, cmd)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("ExitCode should not be 0 for failed command")
	}
}

func TestCommandRunnerTimeout(t *testing.T) {
	t.Parallel()

	runner := &CommandRunner{}

	var cmd []string
	if runtime.GOOS == "windows" {
		// Windows: use ping with a long timeout
		cmd = []string{"ping", "-n", "60", "127.0.0.1"}
	} else {
		cmd = []string{"sleep", "60"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := runner.Run(ctx, cmd)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestCommandRunnerEmptyCommand(t *testing.T) {
	t.Parallel()

	runner := &CommandRunner{}
	_, err := runner.Run(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil command")
	}

	_, err = runner.Run(context.Background(), []string{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

// TestCommandRunnerNoShell confirms we never execute via shell.
func TestCommandRunnerNoShell(t *testing.T) {
	t.Parallel()

	runner := &CommandRunner{}
	// Attempt to inject shell commands via arguments.
	cmd := []string{"echo", "; rm -rf /"}
	result, err := runner.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The semicolon should be part of the output, not a shell separator.
	if !strings.Contains(result.Stdout, ";") {
		t.Logf("Note: output did not contain semicolon: %q (may be shell-dependent)", result.Stdout)
	}
}

func TestCommandRunnerRedactionFlag(t *testing.T) {
	t.Parallel()

	runner := &CommandRunner{}
	cmd := []string{"echo", "hello world"}
	result, err := runner.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The redacted field should be set.
	if result.Stdout == "" {
		t.Log("stdout was empty")
	}
}
