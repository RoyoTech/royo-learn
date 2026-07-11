package evidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandHelper(t *testing.T) {
	if os.Getenv("ROYO_COMMAND_HELPER") != "1" {
		return
	}
	args := helperArgs(os.Args)
	if len(args) == 0 {
		os.Exit(2)
	}
	switch args[0] {
	case "print":
		fmt.Print(strings.Join(args[1:], " "))
	case "both":
		fmt.Print(args[1])
		fmt.Fprint(os.Stderr, args[2])
	case "sleep":
		time.Sleep(time.Second)
	case "fail":
		os.Exit(7)
	}
	os.Exit(0)
}

func helperArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func helperCommand(args ...string) []string {
	return append([]string{os.Args[0], "-test.run=TestCommandHelper", "--"}, args...)
}

func testRunner() *CommandRunner {
	return &CommandRunner{
		AllowedCommands: []string{os.Args[0]},
		Environment:     []string{"ROYO_COMMAND_HELPER=1"},
	}
}

func TestCommandRunnerExecutesDirectlyAndPreservesArguments(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "injected")
	payload := "; touch " + marker
	result, err := testRunner().Run(context.Background(), helperCommand("print", payload))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Stdout != payload {
		t.Fatalf("Stdout = %q, want literal argument %q", result.Stdout, payload)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("injected command created %q", marker)
	}
}

func TestCommandRunnerRejectsCommandsOutsideAllowlist(t *testing.T) {
	runner := &CommandRunner{AllowedCommands: []string{"git"}}
	if _, err := runner.Run(context.Background(), helperCommand("print", "blocked")); err == nil {
		t.Fatal("Run accepted command outside allowlist")
	}
}

func TestCommandRunnerEnforcesInputBoundaryAndNUL(t *testing.T) {
	runner := testRunner()
	atLimit := helperCommand("print", "1234")
	runner.MaxInputBytes = commandInputBytes(atLimit)
	if _, err := runner.Run(context.Background(), atLimit); err != nil {
		t.Fatalf("Run at input limit: %v", err)
	}
	if _, err := runner.Run(context.Background(), helperCommand("print", "12345")); err == nil {
		t.Fatal("Run above input limit succeeded")
	}
	if _, err := runner.Run(context.Background(), helperCommand("print", "bad\x00arg")); err == nil {
		t.Fatal("Run accepted NUL argument")
	}
}

func TestCommandRunnerCapsCombinedOutputAndRedactsSecrets(t *testing.T) {
	runner := testRunner()
	runner.MaxOutputBytes = 8
	result, err := runner.Run(context.Background(), helperCommand("both", "123456", "abcdef"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Stdout)+len(result.Stderr) > 8 || !result.Truncated {
		t.Fatalf("combined output = %d, truncated = %v; want at most 8, true", len(result.Stdout)+len(result.Stderr), result.Truncated)
	}

	runner.MaxOutputBytes = 1024
	secret := "sk-proj-1234567890ABCDE"
	result, err = runner.Run(context.Background(), helperCommand("print", secret))
	if err != nil {
		t.Fatalf("Run secret output: %v", err)
	}
	if strings.Contains(result.Stdout, secret) || !result.Redacted {
		t.Fatalf("secret output was not redacted: %#v", result)
	}
}

func TestCommandRunnerUsesConfiguredTimeout(t *testing.T) {
	runner := testRunner()
	runner.Timeout = 50 * time.Millisecond
	if _, err := runner.Run(context.Background(), helperCommand("sleep")); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("Run timeout error = %v", err)
	}
}

func TestCommandRunnerValidatesWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	runner := testRunner()
	runner.Root = root
	runner.Dir = "../outside"
	if _, err := runner.Run(context.Background(), helperCommand("print", "blocked")); err == nil {
		t.Fatal("Run accepted working directory outside root")
	}

	runner.Dir = "work"
	if err := os.Mkdir(filepath.Join(root, "work"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	result, err := runner.Run(context.Background(), helperCommand("print", "ok"))
	if err != nil || result.Stdout != "ok" {
		t.Fatalf("Run in valid directory = %#v, %v", result, err)
	}
}

func TestCommandRunnerReportsExitCode(t *testing.T) {
	result, err := testRunner().Run(context.Background(), helperCommand("fail"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
}
