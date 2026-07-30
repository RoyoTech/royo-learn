// Hito 12 — drift status MCP surface helpers.
// These helpers format publication_drift_state rows for the JSON envelope
// emitted by the experience_drift_status MCP tool and the experience drift
// CLI. The PII redaction (filepath.Base on target_path) is shared by both
// surfaces.

package mcpserver

import (
	"path/filepath"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/publish/drift"
)

// driftSourceSummary mirrors cmd/royo-learn/experience_drift.go::experienceDriftSourceSummary.
// Kept separate from the CLI type so the mcpserver package does not import
// the cmd package (the dependency direction would be wrong).
type driftSourceSummary struct {
	Source      string `json:"source"`
	TotalChecks int    `json:"total_checks"`
	OK          int    `json:"ok"`
	Drifted     int    `json:"drifted"`
	Missing     int    `json:"target_missing"`
	Unreadable  int    `json:"target_unreadable"`
}

// driftPublicationRow mirrors cmd/royo-learn/experience_drift.go::experienceDriftPublicationRow.
type driftPublicationRow struct {
	PublicationID string `json:"publication_id"`
	Source        string `json:"source"`
	TargetPath    string `json:"target_path"` // basename only (PII redaction)
	ExpectedHash  string `json:"expected_hash"`
	ActualHash    string `json:"actual_hash,omitempty"`
	Status        string `json:"status"`
	CheckedAt     string `json:"checked_at"`
	RunID         string `json:"run_id,omitempty"`
}

// aggregateDriftBySource groups drift rows by source for the "sources"
// section of the envelope. Empty input still yields the four canonical
// sources with zero counts.
func aggregateDriftBySource(rows []drift.DriftRow) []driftSourceSummary {
	buckets := map[string]*driftSourceSummary{
		string(domain.SourceOpenCode):   {Source: string(domain.SourceOpenCode)},
		string(domain.SourceClaudeCode): {Source: string(domain.SourceClaudeCode)},
		string(domain.SourceCodex):      {Source: string(domain.SourceCodex)},
	}
	for _, r := range rows {
		s, ok := buckets[r.Source]
		if !ok {
			s = &driftSourceSummary{Source: r.Source}
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
	order := []string{string(domain.SourceOpenCode), string(domain.SourceClaudeCode), string(domain.SourceCodex)}
	out := make([]driftSourceSummary, 0, len(buckets))
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

// redactDriftForJSON converts persisted rows into the JSON envelope's
// "publications" section with target_path redacted to its basename.
func redactDriftForJSON(rows []drift.DriftRow) []driftPublicationRow {
	out := make([]driftPublicationRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, driftPublicationRow{
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
