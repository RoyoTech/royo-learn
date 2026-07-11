package evidence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	DefaultCommandOutputBytes int64 = 1 << 20
	DefaultCommandInputBytes  int64 = 1 << 20
	DefaultCommandTimeout           = 60 * time.Second
)

// CommandResult holds the captured output and metadata from running a command.
type CommandResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Redacted  bool
	Truncated bool
}

// CommandRunner executes allowlisted commands directly, without a shell.
type CommandRunner struct {
	MaxOutputBytes  int64
	MaxInputBytes   int64
	Timeout         time.Duration
	AllowedCommands []string
	Root            string
	Dir             string
	Environment     []string
	KnownSecrets    []string
}

// Run executes cmd with separate arguments and bounded inputs, time, and
// combined output. A nil allowlist permits only git.
func (r *CommandRunner) Run(ctx context.Context, cmd []string) (*CommandResult, error) {
	if len(cmd) == 0 || cmd[0] == "" {
		return nil, fmt.Errorf("command runner: empty command")
	}
	for _, value := range cmd {
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("command runner: NUL byte is forbidden")
		}
	}
	if !r.commandAllowed(cmd[0]) {
		return nil, fmt.Errorf("command runner: command %q is not allowlisted", cmd[0])
	}
	maxInput := r.MaxInputBytes
	if maxInput <= 0 {
		maxInput = DefaultCommandInputBytes
	}
	if size := commandInputBytes(cmd); size > maxInput {
		return nil, fmt.Errorf("command runner: input too large: %d bytes exceeds %d", size, maxInput)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execCmd := exec.CommandContext(runCtx, cmd[0], cmd[1:]...)
	if r.Dir != "" {
		if r.Root == "" {
			return nil, fmt.Errorf("command runner: root is required with working directory")
		}
		dir, err := ResolvePath(r.Root, r.Dir)
		if err != nil {
			return nil, fmt.Errorf("command runner: working directory: %w", err)
		}
		execCmd.Dir = dir
	}
	execCmd.Env = append(minimalEnvironment(), r.Environment...)

	maxOutput := r.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = DefaultCommandOutputBytes
	}
	output := newCappedOutput(maxOutput)
	execCmd.Stdout = output.writer(&output.stdout)
	execCmd.Stderr = output.writer(&output.stderr)
	runErr := execCmd.Run()
	if runCtx.Err() != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("command runner: timeout: %w", runCtx.Err())
		}
		return nil, fmt.Errorf("command runner: %w", runCtx.Err())
	}

	stdout, stderr, truncated := output.result()
	if truncated {
		// A truncated token or private key cannot be reliably classified. Drop
		// partial output rather than risk returning a secret fragment.
		stdout, stderr = nil, nil
	}
	redactedStdout := Redact(stdout, r.KnownSecrets)
	redactedStderr := Redact(stderr, r.KnownSecrets)
	result := &CommandResult{
		Stdout:    string(redactedStdout),
		Stderr:    string(redactedStderr),
		Redacted:  truncated || !bytes.Equal(stdout, redactedStdout) || !bytes.Equal(stderr, redactedStderr),
		Truncated: truncated,
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else if runErr != nil {
		return nil, fmt.Errorf("command runner: execute: %w", runErr)
	}
	return result, nil
}

func (r *CommandRunner) commandAllowed(command string) bool {
	allowed := r.AllowedCommands
	if allowed == nil {
		allowed = []string{"git"}
	}
	for _, candidate := range allowed {
		if command == candidate {
			return true
		}
	}
	return false
}

func commandInputBytes(cmd []string) int64 {
	var size int64
	for _, value := range cmd {
		size += int64(len(value))
	}
	return size
}

func minimalEnvironment() []string {
	keys := []string{"PATH", "SYSTEMROOT", "TMP", "TEMP"}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

type cappedOutput struct {
	mu        sync.Mutex
	remaining int64
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	truncated bool
}

type cappedWriter struct {
	output *cappedOutput
	target *bytes.Buffer
}

func newCappedOutput(limit int64) *cappedOutput { return &cappedOutput{remaining: limit} }

func (o *cappedOutput) writer(target *bytes.Buffer) *cappedWriter {
	return &cappedWriter{output: o, target: target}
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.output.mu.Lock()
	defer w.output.mu.Unlock()
	written := len(p)
	if int64(len(p)) > w.output.remaining {
		p = p[:w.output.remaining]
		w.output.truncated = true
	}
	_, _ = w.target.Write(p)
	w.output.remaining -= int64(len(p))
	return written, nil
}

func (o *cappedOutput) result() ([]byte, []byte, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]byte(nil), o.stdout.Bytes()...), append([]byte(nil), o.stderr.Bytes()...), o.truncated
}
