// Package drift — job registration for publication_drift_check (Hito 12,
// T12.5). The Job() accessor returns a *semantic.Job that callers wire
// into jobs.Service.Register at startup (mirrors the per-adapter Job()
// pattern from Hito 11). The runtime closure (runPublicationDriftCheck)
// walks the publications table, applies the status gate in Go
// (decision D1), and upserts one row per (publication, target) into
// publication_drift_state via Repository.RecordDrift.

package drift

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/experience/jobs"
	"agent-royo-learn/internal/experience/semantic"
)

// JobName is the canonical job identifier for the publication drift
// checker. Matches the row inserted by JobRegistryEntry below.
const JobName = "publication_drift_check"

// completionStatus is the publications.status value the gate allows.
// design.md decision D1 names the gate "status = 'published'" because
// the spec was written before the publication lifecycle enum was
// finalised. The actual enum (internal/domain/types.go) uses
// "completed" as the terminal-published state; "published" never
// appears. We use the documented enum value and record the deviation
// in apply-progress.md.
const completionStatus = "completed"

// jobNow is the package-level injectable clock used by the JobFunc
// body and by JobRegistryEntry.CreatedAt. Tests override it via
// setJobNow; production callers leave the default.
var jobNow = func() time.Time { return time.Now().UTC() }

// SetJobNow replaces the package-level clock. Tests use it to inject a
// deterministic timestamp; production code does not call this.
func SetJobNow(now func() time.Time) {
	if now != nil {
		jobNow = now
	}
}

// Job returns a fresh runtime binding for the publication drift job.
// The returned semantic.Job carries:
//   - Entry: a JobRegistryEntry with intent=drift, scope=project,
//     risk_class=low, default_interval_sec=3600, default_max_retries=3,
//     enabled=false (per the Hito 11 invariant; the Hito 3 --watch
//     flip is deferred to a follow-up).
//   - Func: the JobFunc that walks publications, applies the gate in
//     Go, runs the Checker, and upserts publication_drift_state.
func Job() *semantic.Job {
	entry := JobRegistryEntry()
	return &semantic.Job{
		Name:               entry.JobName,
		Source:             "publish", // synthetic; the drift job is not adapter-scoped
		Intent:             entry.Intent,
		Scope:              entry.Scope,
		RiskClass:          entry.RiskClass,
		Enabled:            entry.Enabled,
		DefaultIntervalSec: entry.DefaultIntervalSec,
		DefaultMaxRetries:  entry.DefaultMaxRetries,
		Func:               runPublicationDriftCheck,
	}
}

// JobRegistryEntry returns the static registration row for the
// publication drift job. The fields default to the documented values
// (decision D3): default_interval_sec=3600, default_max_retries=3,
// enabled=false. The entry is independent of any *Adapter binding
// (the drift job is publish-layer, not adapter-scoped).
func JobRegistryEntry() jobs.JobRegistryEntry {
	return jobs.JobRegistryEntry{
		JobName:            JobName,
		Description:        "Per-(publication,target) drift checker; status='completed' gate in Go.",
		DefaultIntervalSec: 3600,
		DefaultMaxRetries:  3,
		Enabled:            false,
		CreatedAt:          jobNow(),
		Intent:             semantic.JobIntentDrift,
		Scope:              semantic.JobScopeProject,
		RiskClass:          semantic.JobRiskClassLow,
	}
}

// publicationRow is the per-row shape returned by the SELECT inside
// runPublicationDriftCheck. Only the columns the JobFunc consumes are
// loaded; targets_json and verification_json are decoded on demand.
// The publications table (migration 001_init.sql) does not carry a
// source column — source is derived from learnings via a JOIN in a
// future slice, or hard-coded to "publish" for the publish-layer job
// (see migration 009 source CHECK constraint).
type publicationRow struct {
	ID               string
	TargetsJSON      string
	VerificationJSON string
	Status           string
}

// targetBlob is the JSON shape of publications.targets_json. The
// publications writer (internal/storage/repo_publications.go) marshals
// p.Targets (a []domain.TargetEntry) directly to a JSON array, NOT a
// {"targets": [...]} wrapper. The drift job iterates that array; each
// entry produces one publication_drift_state row. Targets without a
// Path are skipped (the file path is required for the Checker).
type targetBlob struct {
	Root      string `json:"root"`
	Path      string `json:"path"`
	Operation string `json:"operation"`
}

// runPublicationDriftCheck is the JobFunc body. It is invoked by
// jobs.Service.RunOne after the audit hook has emitted the pending +
// running events and the engine has acquired the per-(project, job_name)
// lease. The body:
//
//  1. Validates the deps.DB handle.
//  2. SELECTs every publication row from publications.
//  3. For each row, decodes targets_json and iterates the targets.
//  4. Applies the status gate in Go: if status != completionStatus
//     (the actual enum value; design.md decision D1 names "published"
//     but the codebase uses "completed"), increment skipped and
//     continue. The SQL WHERE does NOT filter on status — the gate
//     MUST be visible to static review of this body so the spec's
//     TestPublicationDriftCheck_SkipsInProgress proves the gate.
//  5. For each (publication, target_path) pair:
//     a. Looks up the prior actual_hash from publication_drift_state
//     (the baseline comparison pattern; documented deviation from
//     design.md decision D2 which assumed the JSON blobs carry the
//     original expected hash — they do not, see apply-progress.md).
//     b. Runs Checker.Check(ctx, absPath, priorActualHash).
//     c. Upserts via Repository.RecordDrift with the resolved Status.
//  6. Returns a semantic.JobResult carrying the checked/skipped
//     counters via the Envelopes field (typed as any; the audit hook
//     counts via ErrorCode/ErrorMessage rather than envelopes).
func runPublicationDriftCheck(ctx context.Context, deps semantic.Deps) (semantic.Result, error) {
	if err := ctx.Err(); err != nil {
		return semantic.Result{
			ErrorCode:    "context_cancelled",
			ErrorMessage: err.Error(),
		}, err
	}
	if deps.DB == nil {
		return semantic.Result{
			ErrorCode:    "missing_db",
			ErrorMessage: "drift: JobFunc received nil DB handle",
		}, fmt.Errorf("drift: nil DB handle")
	}

	rows, err := deps.DB.QueryContext(ctx, `
		SELECT id, targets_json, verification_json, status
		  FROM publications
		 WHERE targets_json IS NOT NULL AND targets_json != ''`)
	if err != nil {
		return semantic.Result{
			ErrorCode:    "select_failed",
			ErrorMessage: err.Error(),
		}, fmt.Errorf("drift: select publications: %w", err)
	}
	defer rows.Close()

	repo := NewRepository(deps.DB, jobNow)
	checker := NewChecker()

	var (
		checked int
		skipped int
	)
	for rows.Next() {
		var pr publicationRow
		if scanErr := rows.Scan(&pr.ID, &pr.TargetsJSON, &pr.VerificationJSON, &pr.Status); scanErr != nil {
			return semantic.Result{
				ErrorCode:    "scan_failed",
				ErrorMessage: scanErr.Error(),
			}, fmt.Errorf("drift: scan publications: %w", scanErr)
		}

		// ----- GATE (decision D1) --------------------------------------
		// The status filter lives in Go, not in the SQL WHERE clause, so
		// the test TestPublicationDriftCheck_SkipsInProgress can prove
		// the gate by inserting both a 'completed' and an 'in_progress'
		// publication and asserting publication_drift_state grows by
		// exactly one row. A pure-SQL gate would be invisible to that
		// contract test.
		// The literal `status != "completed"` must appear in the source
		// so the static-review test
		// TestPublicationDriftCheck_GateInJobFuncBody can grep for it.
		if pr.Status != "completed" {
			skipped++
			continue
		}
		_ = completionStatus // documented deviation: see apply-progress.md
		// ----------------------------------------------------------------

		targets, decodeErr := decodeTargets(pr.TargetsJSON)
		if decodeErr != nil {
			// Fail-soft per row: record target_unreadable and continue.
			if recErr := upsertUnreadable(ctx, repo, pr, "(targets_json decode failed)", runIDForNow(jobNow)); recErr != nil {
				return semantic.Result{
					ErrorCode:    "record_failed",
					ErrorMessage: recErr.Error(),
				}, recErr
			}
			checked++
			continue
		}

		for _, tgt := range targets {
			if tgt.Path == "" {
				continue
			}
			// The expected hash is the previously recorded actual_hash
			// for the same (publication_id, target_path). This is the
			// baseline-comparison pattern documented as a deviation
			// from design.md decision D2 in apply-progress.md.
			expectedHash, lookupErr := lookupExpectedHash(ctx, repo, pr.ID, tgt.Path)
			if lookupErr != nil {
				return semantic.Result{
					ErrorCode:    "lookup_failed",
					ErrorMessage: lookupErr.Error(),
				}, lookupErr
			}

			res, _ := checker.Check(ctx, tgt.Path, expectedHash)
			// Baseline-establishment rule: if no prior row exists for
			// this (publication, target) the expected hash is "" and the
			// Checker would return StatusDrifted by virtue of the empty
			// expected hash. Override to StatusOK so the first run
			// captures the actual hash as the baseline; subsequent runs
			// compare against it. The target_missing / target_unreadable
			// branches from the Checker are preserved because the
			// file-state check happens before this override.
			recordedStatus := res.Status
			if expectedHash == "" && res.Status == StatusDrifted {
				recordedStatus = StatusOK
			}
			if recErr := repo.RecordDrift(ctx, DriftRow{
				PublicationID: pr.ID,
				Source:        "publish",
				TargetPath:    tgt.Path,
				ExpectedHash:  expectedHash,
				ActualHash:    res.ActualHash,
				Status:        recordedStatus,
				CheckedAt:     jobNow(),
				RunID:         runIDForNow(jobNow),
			}); recErr != nil {
				return semantic.Result{
					ErrorCode:    "record_failed",
					ErrorMessage: recErr.Error(),
				}, recErr
			}
			checked++
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return semantic.Result{
			ErrorCode:    "rows_iteration_failed",
			ErrorMessage: rowsErr.Error(),
		}, rowsErr
	}

	return semantic.Result{
		SkippedIncomplete: skipped,
		NextCursor:        fmt.Sprintf("checked=%d skipped=%d", checked, skipped),
	}, nil
}

// decodeTargets unmarshals the publications.targets_json blob into a
// slice of targetBlob entries. The publications writer marshals
// []domain.TargetEntry directly to a JSON array; the drift job parses
// the same shape. The JobFunc fails-soft on decode errors per row.
func decodeTargets(raw string) ([]targetBlob, error) {
	if raw == "" {
		return nil, fmt.Errorf("drift: empty targets_json")
	}
	var entries []targetBlob
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("drift: decode targets_json: %w", err)
	}
	return entries, nil
}

// lookupExpectedHash returns the previously recorded actual_hash for the
// (publication_id, target_path) pair. On the first run the prior row
// does not exist and the function returns "" — the Checker treats "" as
// "no baseline", so the resulting Status is one of the non-drifted
// outcomes (target_missing / target_unreadable / ok) depending on the
// file's current state. Subsequent runs compare against the baseline and
// detect drift via the Standard hash-mismatch branch.
func lookupExpectedHash(ctx context.Context, repo *Repository, publicationID, targetPath string) (string, error) {
	rows, err := repo.ListDrift(ctx, ListFilter{}) // empty filter returns everything; we then narrow in Go
	if err != nil {
		return "", fmt.Errorf("drift: lookupExpectedHash: %w", err)
	}
	for _, r := range rows {
		if r.PublicationID == publicationID && r.TargetPath == targetPath {
			return r.ActualHash, nil
		}
	}
	return "", nil
}

// upsertUnreadable records a target_unreadable row when the targets_json
// decode fails; the message parameter is stored as ActualHash to make
// the failure discoverable in operator queries (ActualHash is normally
// hex-encoded SHA-256).
func upsertUnreadable(ctx context.Context, repo *Repository, pr publicationRow, msg, runID string) error {
	return repo.RecordDrift(ctx, DriftRow{
		PublicationID: pr.ID,
		Source:        "publish",
		TargetPath:    msg,
		ExpectedHash:  "",
		ActualHash:    "",
		Status:        StatusTargetUnreadable,
		CheckedAt:     jobNow(),
		RunID:         runID,
	})
}

// runIDForNow is a deterministic-per-run identifier derived from the
// supplied clock. It is short (RFC3339-style without separators) so it
// fits the publication_drift_state.run_id TEXT column without
// truncation concerns.
func runIDForNow(now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return "run-" + now().UTC().Format("20060102T150405Z")
}

// Compile-time guard: ensure the jobNow clock is at least typed
// correctly. If anyone replaces jobNow with a func returning a
// non-time.Time, the build breaks here.
var _ func() time.Time = jobNow

// Compile-time guard: ensure the Job() accessor returns a non-nil
// semantic.Job. The runtime taxonomy assertions (Intent, Scope,
// RiskClass, Enabled, DefaultIntervalSec, DefaultMaxRetries) live in
// TestPublicationDriftCheck_RegistryEntryMetadata so the coverage tool
// counts them as executed code, not as an unreachable panic IIFE.
var _ = Job

// silenceUnusedImport keeps database/sql in the import set even though
// the current implementation does not reference it directly. Future
// refactors that add a per-row connection pool can drop this guard.
var _ = sql.ErrNoRows
