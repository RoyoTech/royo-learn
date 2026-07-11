package main

import (
	"fmt"
	"io"
	"os"

	"agent-royo-learn/internal/buildinfo"
	"agent-royo-learn/internal/logging"
)

const (
	exitSuccess          = 0
	exitInvalidArguments = 2
	exitFailure          = 1

	invalidArgumentsMessage    = `invalid arguments: expected "version --json"`
	invalidArgumentsNextAction = `run "royo-learn version --json"`
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 2 && args[0] == "version" && args[1] == "--json" {
		return writeVersionJSON(stdout, stderr)
	}

	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        "invalid_argument",
		Message:     invalidArgumentsMessage,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  invalidArgumentsNextAction,
	})
	return exitInvalidArguments
}

func writeVersionJSON(stdout, stderr io.Writer) int {
	document, err := buildinfo.VersionJSON()
	if err != nil {
		_ = logging.WriteDiagnostic(stderr, "could not encode version metadata")
		return exitFailure
	}
	if _, err := fmt.Fprint(stdout, document); err != nil {
		_ = logging.WriteDiagnostic(stderr, "could not write version metadata")
		return exitFailure
	}
	return exitSuccess
}
