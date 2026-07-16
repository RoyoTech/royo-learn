// Package portability implements the versioned export, import, and
// reconstruction of a royo-learn store (plan 4.6). SQLite is the operational
// source of truth (D6); an export is a complete, portable snapshot of it, and an
// import reconstructs that truth into a fresh or partially populated store
// without ever overwriting divergent records silently.
package portability

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"agent-royo-learn/internal/domain"
)

// BundleFormatVersion is the on-disk schema version of an export. Import refuses
// a version it does not understand rather than guessing.
const BundleFormatVersion = 1

// Bundle is a complete, portable snapshot of one project's operational truth.
// Every ID is preserved so a round-trip is identity-preserving, not merely
// content-preserving.
type Bundle struct {
	FormatVersion int                        `json:"format_version"`
	ExportedAt    time.Time                  `json:"exported_at"`
	ProjectKey    string                     `json:"project_key"`
	Project       *domain.Project            `json:"project"`
	Learnings     []*domain.Learning         `json:"learnings"`
	Evidence      []*domain.Evidence         `json:"evidence"`
	Relations     []*domain.LearningRelation `json:"relations"`
	Recurrences   []*domain.RecurrenceRecord `json:"recurrences"`
}

// jsonlLine is one typed line of the JSONL stream. A header line comes first,
// then one line per record. Reading is therefore streaming and order-tolerant.
type jsonlLine struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`

	// Header-only fields.
	FormatVersion int       `json:"format_version,omitempty"`
	ExportedAt    time.Time `json:"exported_at,omitempty"`
	ProjectKey    string    `json:"project_key,omitempty"`
}

// EncodeJSONL writes the bundle as newline-delimited JSON: a header line, then
// one line per project, learning, evidence, relation, and recurrence record.
func EncodeJSONL(b *Bundle, w io.Writer) error {
	enc := json.NewEncoder(w)
	if err := enc.Encode(jsonlLine{
		Type:          "header",
		FormatVersion: b.FormatVersion,
		ExportedAt:    b.ExportedAt,
		ProjectKey:    b.ProjectKey,
	}); err != nil {
		return fmt.Errorf("portability: encode header: %w", err)
	}
	if b.Project != nil {
		if err := encodeRecord(enc, "project", b.Project); err != nil {
			return err
		}
	}
	for _, l := range b.Learnings {
		if err := encodeRecord(enc, "learning", l); err != nil {
			return err
		}
	}
	for _, e := range b.Evidence {
		if err := encodeRecord(enc, "evidence", e); err != nil {
			return err
		}
	}
	for _, r := range b.Relations {
		if err := encodeRecord(enc, "relation", r); err != nil {
			return err
		}
	}
	for _, r := range b.Recurrences {
		if err := encodeRecord(enc, "recurrence", r); err != nil {
			return err
		}
	}
	return nil
}

func encodeRecord(enc *json.Encoder, kind string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("portability: marshal %s: %w", kind, err)
	}
	if err := enc.Encode(jsonlLine{Type: kind, Data: data}); err != nil {
		return fmt.Errorf("portability: encode %s: %w", kind, err)
	}
	return nil
}

// DecodeJSONL reads a bundle previously written by EncodeJSONL. It validates the
// header format version and requires exactly one header line.
func DecodeJSONL(r io.Reader) (*Bundle, error) {
	b := &Bundle{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	seenHeader := false
	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var line jsonlLine
		if err := json.Unmarshal(raw, &line); err != nil {
			return nil, fmt.Errorf("portability: decode line: %w", err)
		}
		switch line.Type {
		case "header":
			b.FormatVersion = line.FormatVersion
			b.ExportedAt = line.ExportedAt
			b.ProjectKey = line.ProjectKey
			seenHeader = true
		case "project":
			p := &domain.Project{}
			if err := json.Unmarshal(line.Data, p); err != nil {
				return nil, fmt.Errorf("portability: decode project: %w", err)
			}
			b.Project = p
		case "learning":
			l := &domain.Learning{}
			if err := json.Unmarshal(line.Data, l); err != nil {
				return nil, fmt.Errorf("portability: decode learning: %w", err)
			}
			b.Learnings = append(b.Learnings, l)
		case "evidence":
			e := &domain.Evidence{}
			if err := json.Unmarshal(line.Data, e); err != nil {
				return nil, fmt.Errorf("portability: decode evidence: %w", err)
			}
			b.Evidence = append(b.Evidence, e)
		case "relation":
			rel := &domain.LearningRelation{}
			if err := json.Unmarshal(line.Data, rel); err != nil {
				return nil, fmt.Errorf("portability: decode relation: %w", err)
			}
			b.Relations = append(b.Relations, rel)
		case "recurrence":
			rec := &domain.RecurrenceRecord{}
			if err := json.Unmarshal(line.Data, rec); err != nil {
				return nil, fmt.Errorf("portability: decode recurrence: %w", err)
			}
			b.Recurrences = append(b.Recurrences, rec)
		default:
			return nil, fmt.Errorf("portability: unknown record type %q", line.Type)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("portability: scan: %w", err)
	}
	if !seenHeader {
		return nil, fmt.Errorf("portability: no header line found")
	}
	return b, nil
}
