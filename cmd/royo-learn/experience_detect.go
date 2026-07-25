// experience detect — Hito 5 slice 5.3.
//
// Thin orchestration command over the detector registry. It accepts a
// detector-specific payload via --input (or stdin), runs the matching
// detector from the registry, and prints the emitted CandidateEvents
// to stdout as a single JSON object.
//
// No database access in this slice: the command is a pure
// orchestrator that converts a JSON payload into a DetectInput, runs
// the detector, and emits CandidateEvents. Persistence via
// capture.Service is the responsibility of slice 5.4.
//
// Flags:
//
//	--kind <kind>          required; must match a registered detector
//	--project-root <path>  optional; recorded in DetectInput (defaults
//	                       to the current working directory)
//	--input <path>         optional; path to JSON file with the
//	                       detector-specific payload (defaults to
//	                       stdin)
//
// Output: a single JSON object on stdout. Schema is stable:
//
//	{
//	  "kind": "retry",
//	  "version": "0.1.0",
//	  "status": "ok",
//	  "detected_events": [ ... CandidateEvent ... ],
//	  "total_events": N
//	}
//
// Errors land on stderr through writeExperienceError with the
// project's stable error envelope. Exit codes follow the convention
// in cmd/royo-learn/main.go: 0 success, 2 invalid arguments, 1
// failure / internal error.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"agent-royo-learn/internal/experience/detectors"
)

// runExperienceDetect dispatches `experience detect`. The subcommand
// is intentionally small: the orchestration logic lives in
// executeDetect so the tests can drive it directly with a synthetic
// stream of bytes.
func runExperienceDetect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience detect", flag.ContinueOnError)
	kind := fs.String("kind", "", "detector kind to invoke (e.g. retry)")
	projectRoot := fs.String("project-root", "", "project root recorded in DetectInput (defaults to current directory)")
	input := fs.String("input", "", "path to JSON file with the detector-specific payload (defaults to stdin)")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience detect: %v", err)
	}
	if *kind == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience detect: --kind is required")
	}

	root := *projectRoot
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return writeExperienceError(stderr, "internal_error", "experience detect: cannot determine working directory: %v", err)
		}
		root = cwd
	}

	var r io.Reader
	if *input == "" {
		r = os.Stdin
	} else {
		f, err := os.Open(*input)
		if err != nil {
			return writeExperienceError(stderr, "invalid_argument", "experience detect: cannot open --input: %v", err)
		}
		defer f.Close()
		r = f
	}

	return executeDetect(stdout, stderr, *kind, root, r)
}

// executeDetect is the testable inner core of runExperienceDetect.
// It builds the registry, resolves the detector, decodes the
// payload, runs Detect, and encodes the output.
//
// The payload type is dispatched on the detector's Kind; today only
// "retry" has a registered payload type. Adding a new detector
// without extending this switch fails closed with an explicit error,
// which keeps the contract honest.
func executeDetect(stdout, stderr io.Writer, kind, projectRoot string, payload io.Reader) int {
	reg := detectors.NewRegistry()
	retryDet, err := detectors.NewRetryDetector(3, 5*time.Minute)
	if err != nil {
		return writeExperienceError(stderr, "internal_error", "experience detect: cannot construct retry detector: %v", err)
	}
	if err := reg.Register(retryDet); err != nil {
		return writeExperienceError(stderr, "internal_error", "experience detect: cannot register retry detector: %v", err)
	}

	det, ok := reg.Get(kind)
	if !ok {
		return writeExperienceError(stderr, "not_found",
			"experience detect: no detector registered for kind %q (registered: %v)", kind, reg.Kinds())
	}

	payloadValue, err := decodePayload(kind, payload)
	if err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience detect: %v", err)
	}

	in := detectors.DetectInput{
		Source:      "cli",
		ProjectRoot: projectRoot,
		Payload:     payloadValue,
		Timestamp:   time.Now().UTC(),
	}

	events, err := det.Detect(context.Background(), in)
	if err != nil {
		return writeExperienceError(stderr, "detector_error", "experience detect: %v", err)
	}

	out := experienceDetectOutput{
		Kind:           det.Kind(),
		Version:        det.Version(),
		Status:         "ok",
		DetectedEvents: events,
		TotalEvents:    len(events),
	}
	return encodeExperienceDetectOutput(stdout, out)
}

// decodePayload maps a detector Kind to its concrete payload type
// and unmarshals the supplied reader into it. Centralising the
// dispatch here keeps executeDetect linear and the test surface
// small.
func decodePayload(kind string, r io.Reader) (any, error) {
	switch kind {
	case "retry":
		var p detectors.RetryPayload
		if err := json.NewDecoder(r).Decode(&p); err != nil {
			return nil, fmt.Errorf("cannot decode retry payload: %v", err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("no decoder registered for kind %q", kind)
	}
}

// experienceDetectOutput is the stable JSON shape produced by
// `experience detect`. Schema is pinned by Hito 5; consumers gating
// on these field names should be updated only with a versioned
// contract change.
type experienceDetectOutput struct {
	Kind           string                     `json:"kind"`
	Version        string                     `json:"version"`
	Status         string                     `json:"status"`
	DetectedEvents []detectors.CandidateEvent `json:"detected_events"`
	TotalEvents    int                        `json:"total_events"`
}

// encodeExperienceDetectOutput writes the JSON object to stdout and
// returns the exit code. Encoding errors after the first write are
// not recoverable, so they collapse to exitFailure.
func encodeExperienceDetectOutput(stdout io.Writer, out experienceDetectOutput) int {
	enc := json.NewEncoder(stdout)
	if err := enc.Encode(out); err != nil {
		return exitFailure
	}
	return exitSuccess
}
