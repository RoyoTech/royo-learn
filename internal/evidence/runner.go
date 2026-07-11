package evidence

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// CommandResult holds the captured output and metadata from running a command.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Redacted bool
}

// CommandRunner executes external commands safely.
// Zero value is ready to use.
type CommandRunner struct {
	// MaxOutputBytes limits the combined stdout+stderr. Zero means no limit.
	MaxOutputBytes int64
}

// Run executes cmd with the given context. The command is executed directly
// (never via shell). Arguments are passed separately as individual strings.
func (r *CommandRunner) Run(ctx context.Context, cmd []string) (*CommandResult, error) {
	if len(cmd) == 0 {
		return nil, fmt.Errorf("command runner: empty command")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	execCmd := exec.CommandContext(ctx, cmd[0], cmd[1:]...)

	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	runErr := execCmd.Run()

	result := &CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if runErr != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("command runner: %w", ctx.Err())
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	} else {
		result.ExitCode = 0
	}

	// Apply size limits.
	if r.MaxOutputBytes > 0 {
		totalLen := int64(len(result.Stdout)) + int64(len(result.Stderr))
		if totalLen > r.MaxOutputBytes {
			result.Stdout = result.Stdout[:min(int64(len(result.Stdout)), r.MaxOutputBytes/2)]
			result.Stderr = result.Stderr[:min(int64(len(result.Stderr)), r.MaxOutputBytes/2)]
		}
	}

	return result, nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
