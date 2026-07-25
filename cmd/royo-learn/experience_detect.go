// experience detect — Hito 5 slices 5.3 and 5.4.
//
// Orchestration command over the detector registry. It accepts a
// detector-specific payload via --input (or stdin), runs the matching
// detector from the registry, and prints the emitted CandidateEvents
// to stdout as a single JSON object.
//
// Slice 5.4 adds --persist: when set, the command opens the
// canonical experience store for --project-root and forwards every
// emitted CandidateEvent through the detector persistence layer
// (detectors.Persist), which maps the event into an ExperienceEnvelope
// and runs the existing ingestion pipeline. Re-runs are idempotent
// via the synthetic Turn.ExternalID derived from the event content.
//
// Flags:
//
//	--kind <kind>          required; must match a registered detector
//	--project-root <path>  optional; recorded in DetectInput (defaults
//	                       to the current working directory). Required
//	                       when --persist is set.
//	--input <path>         optional; path to JSON file with the
//	                       detector-specific payload (defaults to
//	                       stdin)
//	--persist              optional; when set, persist emitted events
//	                       to the canonical experience store
//
// Output: a single JSON object on stdout. Schema is stable:
//
//	{
//	  "kind": "retry",
//	  "version": "0.1.0",
//	  "status": "ok",
//	  "detected_events": [ ... CandidateEvent ... ],
//	  "total_events": N,
//	  "persisted_count": M,           // only when --persist
//	  "persisted": [                  // only when --persist
//	    {"event_id": "...", "fingerprint": "...", "duplicate": false},
//	    ...
//	  ]
//	}
//
// Errors land on stderr through writeExperienceError with the
// project's stable error envelope. Exit codes follow the convention
// in cmd/royo-learn/main.go: 0 success, 2 invalid arguments, 1
// failure / internal error.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/experience"
	"agent-royo-learn/internal/experience/detectors"
	"agent-royo-learn/internal/storage"
)

// runExperienceDetect dispatches `experience detect`. The subcommand
// is intentionally small: the orchestration logic lives in
// executeDetect (pure) and executeDetectAndPersist (with DB), so the
// tests can drive either path directly with a synthetic stream of
// bytes.
func runExperienceDetect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience detect", flag.ContinueOnError)
	kind := fs.String("kind", "", "detector kind to invoke (e.g. retry)")
	projectRoot := fs.String("project-root", "", "project root recorded in DetectInput (defaults to current directory)")
	input := fs.String("input", "", "path to JSON file with the detector-specific payload (defaults to stdin)")
	persist := fs.Bool("persist", false, "when set, persist emitted events to the canonical experience store (requires --project-root)")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience detect: %v", err)
	}
	if *kind == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience detect: --kind is required")
	}

	root := *projectRoot
	if *persist && root == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience detect: --project-root is required when --persist is set")
	}
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

	if !*persist {
		return executeDetect(stdout, stderr, *kind, root, r)
	}

	_, db, projectID, exitCode := resolvePublishContext(root, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	return executeDetectAndPersist(stdout, stderr, *kind, root, projectID, db, r)
}

// executeDetect is the testable inner core of runExperienceDetect in
// its pure (no-DB) form. It builds the registry, resolves the
// detector, decodes the payload, runs Detect, and encodes the
// output. Adding a new detector without extending decodePayload
// fails closed with an explicit error.
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

// executeDetectAndPersist is the slice 5.4 path. It runs the
// detector in its pure form (via a buffer) and then forwards every
// emitted CandidateEvent through detectors.Persist, which maps it
// into an ExperienceEnvelope and runs the existing ingestion
// pipeline. The output includes the persisted count and per-event
// metadata so the operator can audit the write and the idempotency
// outcome.
func executeDetectAndPersist(
	stdout, stderr io.Writer,
	kind, projectRoot string,
	projectID domain.ProjectID,
	db *storage.DB,
	payload io.Reader,
) int {
	// Run the detector in pure form first so any decode/validation
	// error surfaces before we open the DB transaction.
	var detectedBuf bytes.Buffer
	if code := executeDetect(&detectedBuf, stderr, kind, projectRoot, payload); code != exitSuccess {
		return code
	}

	var detected experienceDetectOutput
	if err := json.Unmarshal(detectedBuf.Bytes(), &detected); err != nil {
		return writeExperienceError(stderr, "internal_error", "experience detect: cannot decode detector output: %v", err)
	}

	svc := experience.NewService(db)
	ctx := context.Background()
	persisted := make([]persistedEvent, 0, len(detected.DetectedEvents))

	for _, ev := range detected.DetectedEvents {
		result, err := detectors.Persist(ctx, svc, projectID, projectRoot, ev, time.Now().UTC())
		if err != nil {
			return writeExperienceError(stderr, "persistence_error", "experience detect: persist failed for kind %q: %v", ev.Kind, err)
		}
		persisted = append(persisted, persistedEvent{
			EventID:     string(result.Turn.ID),
			Fingerprint: result.Turn.Fingerprint,
			Duplicate:   !result.Created,
		})
	}

	out := experienceDetectOutput{
		Kind:           detected.Kind,
		Version:        detected.Version,
		Status:         "ok",
		DetectedEvents: detected.DetectedEvents,
		TotalEvents:    detected.TotalEvents,
		PersistedCount: len(persisted),
		Persisted:      persisted,
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
// contract change. Persisted and PersistedCount are populated only
// when --persist is set; the JSON encoder drops them via omitempty
// when the slice or counter is empty.
type experienceDetectOutput struct {
	Kind           string                     `json:"kind"`
	Version        string                     `json:"version"`
	Status         string                     `json:"status"`
	DetectedEvents []detectors.CandidateEvent `json:"detected_events"`
	TotalEvents    int                        `json:"total_events"`
	PersistedCount int                        `json:"persisted_count,omitempty"`
	Persisted      []persistedEvent           `json:"persisted,omitempty"`
}

// persistedEvent is the per-event metadata returned by --persist.
// EventID is the canonical turn id assigned by the ingestion
// service; Fingerprint is the detector's deterministic fingerprint
// over the event content; Duplicate reports whether the call hit
// the (session_id, external_turn_id) uniqueness constraint and the
// row already existed.
type persistedEvent struct {
	EventID     string `json:"event_id"`
	Fingerprint string `json:"fingerprint"`
	Duplicate   bool   `json:"duplicate"`
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
