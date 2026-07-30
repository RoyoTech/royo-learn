// Hito 12 — unified drift CLI surface.
// runExperienceDrift implements `royo-learn experience drift` which gives
// the operator a single view of drift across the three experience adapters
// and the publication-level drift table.
//
// PII contract: target_path is redacted to filepath.Base() before serializing
// to JSON (per drift-cli-mcp spec REQ-DCM-3). The full path lives only in
// publication_drift_state and is not exposed through this surface.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/publish/drift"
)

// experienceDriftSourceSummary is the per-source row in the JSON envelope's
// "sources" section. It aggregates drift outcomes per source so the
// operator can see at a glance which adapter produced drift in the last
// check cycle.
type experienceDriftSourceSummary struct {
	Source      string `json:"source"`
	TotalChecks int    `json:"total_checks"`
	OK          int    `json:"ok"`
	Drifted     int    `json:"drifted"`
	Missing     int    `json:"target_missing"`
	Unreadable  int    `json:"target_unreadable"`
}

// experienceDriftPublicationRow is the per-publication row in the JSON
// envelope's "publications" section. The TargetPath field is the basename
// of the on-disk target (PII redaction — see package docstring).
type experienceDriftPublicationRow struct {
	PublicationID string `json:"publication_id"`
	Source        string `json:"source"`
	TargetPath    string `json:"target_path"` // basename only
	ExpectedHash  string `json:"expected_hash"`
	ActualHash    string `json:"actual_hash,omitempty"`
	Status        string `json:"status"`
	CheckedAt     string `json:"checked_at"`
	RunID         string `json:"run_id,omitempty"`
}

// experienceDriftOutput is the top-level JSON envelope. Schema is stable
// and pinned by the drift-cli-mcp spec (REQ-DCM-2): every consumer that
// gates on these field names should be updated only with a versioned
// contract change.
type experienceDriftOutput struct {
	Status       string                          `json:"status"`
	Sources      []experienceDriftSourceSummary  `json:"sources"`
	Publications []experienceDriftPublicationRow `json:"publications"`
	Total        int                             `json:"total"`
}

// runExperienceDrift orchestrates the unified drift query. Flags:
//
//	--project-root <path>   root to scope drift queries; required
//	--all-sources           query drift across all 3 sources (default true)
//	--source=<s>            filter to one source: opencode|claudecode|codex
//	                        (overrides --all-sources)
//
// Output: a single JSON object on stdout (see experienceDriftOutput).
// Errors land on stderr through logging.WriteError. Exit codes follow
// domain.ErrorCode.ExitCode().
func runExperienceDrift(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("experience drift", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root to scope drift queries")
	allSources := fs.Bool("all-sources", true, "query drift across all 3 sources (default true)")
	source := fs.String("source", "", "filter to one source: opencode|claudecode|codex")
	if err := fs.Parse(args); err != nil {
		return writeExperienceError(stderr, "invalid_argument", "experience drift: %v", err)
	}
	if *projectRoot == "" {
		return writeExperienceError(stderr, "invalid_argument", "experience drift: --project-root is required")
	}
	if *source != "" {
		switch *source {
		case string(domain.SourceOpenCode), string(domain.SourceClaudeCode), string(domain.SourceCodex):
			// ok
		default:
			return writeExperienceError(stderr, "invalid_argument", "experience drift: invalid --source %q; allowed values: opencode, claudecode, codex", *source)
		}
		*allSources = false
	}

	_, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	repo := drift.NewRepository(db.DB, nil)
	ctx := context.Background()

	filter := drift.ListFilter{}
	if !*allSources && *source != "" {
		filter.Source = *source
	}

	rows, err := repo.ListDrift(ctx, filter)
	if err != nil {
		return writeExperienceError(stderr, "internal_error", "experience drift: list: %v", err)
	}

	output := experienceDriftOutput{
		Status:       "ok",
		Sources:      aggregateBySource(rows),
		Publications: redactPII(rows),
		Total:        len(rows),
	}

	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		fmt.Fprintf(stderr, "experience drift: encode: %v\n", err)
		return exitFailure
	}
	return exitSuccess
}

// aggregateBySource groups the drift rows by source and produces the
// "sources" section of the envelope. Empty input still yields the four
// canonical sources with zero counts, so consumers can rely on the
// shape without conditional rendering.
func aggregateBySource(rows []drift.DriftRow) []experienceDriftSourceSummary {
	buckets := map[string]*experienceDriftSourceSummary{
		string(domain.SourceOpenCode):   {Source: string(domain.SourceOpenCode)},
		string(domain.SourceClaudeCode): {Source: string(domain.SourceClaudeCode)},
		string(domain.SourceCodex):      {Source: string(domain.SourceCodex)},
	}
	for _, r := range rows {
		s, ok := buckets[r.Source]
		if !ok {
			// Unknown source: surface it under its literal name so the
			// operator can spot drift from a not-yet-cataloged adapter.
			s = &experienceDriftSourceSummary{Source: r.Source}
			buckets[r.Source] = s
		}
		s.TotalChecks++
		switch r.Status {
		case drift.StatusOK:
			s.OK++
		case drift.StatusDrifted:
			s.Drifted++
		case drift.StatusTargetMissing:
			s.Missing++
		case drift.StatusTargetUnreadable:
			s.Unreadable++
		}
	}
	// Stable order: opencode, claudecode, codex, then any extras.
	order := []string{string(domain.SourceOpenCode), string(domain.SourceClaudeCode), string(domain.SourceCodex)}
	out := make([]experienceDriftSourceSummary, 0, len(buckets))
	seen := map[string]bool{}
	for _, src := range order {
		if s, ok := buckets[src]; ok {
			out = append(out, *s)
			seen[src] = true
		}
	}
	for src, s := range buckets {
		if !seen[src] {
			out = append(out, *s)
		}
	}
	return out
}

// redactPII turns the persisted drift rows into the JSON envelope's
// "publications" section with target_path redacted to its basename.
// The full path lives only in publication_drift_state.
func redactPII(rows []drift.DriftRow) []experienceDriftPublicationRow {
	out := make([]experienceDriftPublicationRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, experienceDriftPublicationRow{
			PublicationID: r.PublicationID,
			Source:        r.Source,
			TargetPath:    filepath.Base(r.TargetPath),
			ExpectedHash:  r.ExpectedHash,
			ActualHash:    r.ActualHash,
			Status:        string(r.Status),
			CheckedAt:     r.CheckedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			RunID:         r.RunID,
		})
	}
	return out
}
