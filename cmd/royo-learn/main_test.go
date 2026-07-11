package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunVersionJSON(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if got := run([]string{"version", "--json"}, &stdout, &stderr); got != exitSuccess {
		t.Fatalf("run() exit code = %d, want %d", got, exitSuccess)
	}
	if stderr.Len() != 0 {
		t.Fatalf("successful stderr = %q, want empty", stderr.String())
	}

	var document map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
}

func TestRunRejectsInvalidArgumentsOnStderr(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if got := run([]string{"unknown"}, &stdout, &stderr); got != exitInvalidArguments {
		t.Fatalf("run() exit code = %d, want %d", got, exitInvalidArguments)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	assertInvalidArgumentsDiagnostic(t, stderr.Bytes())
}

func TestRunDoesNotCreateDatabaseState(t *testing.T) {
	directory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	before, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir before run: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("temporary directory is not empty: %v", before)
	}

	var stdout, stderr bytes.Buffer
	if got := run([]string{"version", "--json"}, &stdout, &stderr); got != exitSuccess {
		t.Fatalf("run() exit code = %d, want %d", got, exitSuccess)
	}
	after, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir after run: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("version command created files: %v", after)
	}
}

func TestVersionBinaryStreamContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
	}

	binary := filepath.Join(t.TempDir(), "royo-learn")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	tests := []struct {
		name      string
		args      []string
		exitCode  int
		assertion func(*testing.T, []byte, []byte)
	}{
		{
			name:     "version JSON",
			args:     []string{"version", "--json"},
			exitCode: exitSuccess,
			assertion: func(t *testing.T, stdout, stderr []byte) {
				t.Helper()
				if !json.Valid(stdout) {
					t.Errorf("stdout = %q, want valid JSON", stdout)
				}
				if len(stderr) != 0 {
					t.Errorf("stderr = %q, want empty", stderr)
				}
			},
		},
		{
			name:     "invalid arguments",
			args:     []string{"unknown"},
			exitCode: exitInvalidArguments,
			assertion: func(t *testing.T, stdout, stderr []byte) {
				t.Helper()
				if len(stdout) != 0 {
					t.Errorf("stdout = %q, want empty", stdout)
				}
				assertInvalidArgumentsDiagnostic(t, stderr)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := exec.Command(binary, tt.args...)
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if tt.exitCode == exitSuccess {
				if err != nil {
					t.Fatalf("command failed: %v", err)
				}
			} else if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != tt.exitCode {
				t.Fatalf("command error = %v, want exit code %d", err, tt.exitCode)
			}
			tt.assertion(t, stdout.Bytes(), stderr.Bytes())
		})
	}
}

func assertInvalidArgumentsDiagnostic(t *testing.T, output []byte) {
	t.Helper()

	var diagnostic map[string]json.RawMessage
	if err := json.Unmarshal(output, &diagnostic); err != nil {
		t.Fatalf("stderr = %q, want one JSON diagnostic: %v", output, err)
	}
	if len(diagnostic) != 5 {
		t.Errorf("diagnostic field count = %d, want 5: %s", len(diagnostic), output)
	}
	assertDiagnosticField(t, diagnostic, "code", `"invalid_argument"`)
	assertDiagnosticField(t, diagnostic, "message", `"invalid arguments: expected \"version --json\""`)
	assertDiagnosticField(t, diagnostic, "recoverable", "true")
	assertDiagnosticField(t, diagnostic, "details", "{}")
	assertDiagnosticField(t, diagnostic, "next_action", `"run \"royo-learn version --json\""`)
}

func assertDiagnosticField(t *testing.T, diagnostic map[string]json.RawMessage, field, want string) {
	t.Helper()
	if got, ok := diagnostic[field]; !ok {
		t.Errorf("diagnostic omitted %q", field)
	} else if string(got) != want {
		t.Errorf("diagnostic[%q] = %s, want %s", field, got, want)
	}
}
