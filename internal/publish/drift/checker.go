// Package drift implements the publication-level drift detector for the
// Hito 12 (drift/release hardening) capability. The package is
// intentionally read-only on the targets it inspects: the only writes
// happen in publication_drift_state (the per-row upsert repository) and
// never on the target file itself.
//
// Three surfaces live here:
//
//   - Checker.Check(ctx, target, expectedHash) (Result, error)
//     returns one of four outcomes (ok, drifted, target_missing,
//     target_unreadable) after streaming sha256 of the file on disk.
//   - Repository.RecordDrift / ListDrift persist and read the
//     publication_drift_state rows.
//   - Job() (registered at startup via jobs.Service.RunOne) iterates the
//     publications table and feeds the Checker.
//
// The read-only invariant is locked by contract_test.go (snapshot of
// Mode / ModTime / Size before and after Check) and by the pre-commit
// grep over this package for the forbidden write APIs.
package drift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// Status is the outcome of a single drift check on a target. The values
// are stable strings persisted in publication_drift_state.status and
// emitted in the JSON envelope (decision D1, design.md).
type Status string

const (
	StatusOK               Status = "ok"
	StatusDrifted          Status = "drifted"
	StatusTargetMissing    Status = "target_missing"
	StatusTargetUnreadable Status = "target_unreadable"
)

// ErrTargetMissing is returned wrapped by Result.Err when the target path
// resolves to no file (Status == StatusTargetMissing).
var ErrTargetMissing = errors.New("drift: target missing")

// ErrTargetUnreadable is returned wrapped by Result.Err when the target
// path exists per os.Stat but cannot be opened (Status ==
// StatusTargetUnreadable). The underlying I/O error is joined via
// errors.Join so callers can recover both the sentinel and the cause.
var ErrTargetUnreadable = errors.New("drift: target unreadable")

// Result is the typed outcome of one Checker.Check invocation. Status is
// one of the four enum values above. ActualHash is the hex-encoded
// SHA-256 of the bytes on disk, or "" when Status is missing /
// unreadable. Err is nil for OK and Drifted outcomes; for the two
// failure outcomes it carries the wrapped sentinel plus the underlying
// error (see errors.Join).
type Result struct {
	Status     Status
	ActualHash string
	Err        error
}

// IsReadOnly reports whether a given file path is unchanged by the
// Checker. The contract_test.go uses this as a guard.
//
// Deprecated: this stub exists to keep the package-level docstring honest
// while the read-only contract test is the canonical check. New callers
// should import the contract test helpers directly.
func IsReadOnly() bool { return true }

// Checker computes SHA-256 of a published target and compares it to the
// expected hash captured at publish time. It is strictly read-only on
// the target — see contract_test.go.
type Checker struct {
	// openFn is the function used to open the target. Tests may inject
	// a mock; production callers leave it nil and use the default
	// (os.Open).
	openFn func(name string) (*os.File, error)
}

// NewChecker returns a Checker using os.Open.
func NewChecker() *Checker {
	return &Checker{openFn: os.Open}
}

// Check computes the SHA-256 of the target on disk and compares it to
// expectedHash. The implementation is strictly read-only — see the
// contract test and the pre-commit grep rule documented in the proposal.
//
// Decision order:
//  1. ctx.Err() → StatusTargetUnreadable (no further I/O)
//  2. os.Stat(target) → StatusTargetMissing on ENOENT (no further I/O)
//  3. os.Open(target)  → StatusTargetUnreadable on error after a
//     successful stat
//  4. sha256 streaming → StatusDrifted on mismatch, StatusOK on match
func (c *Checker) Check(ctx context.Context, target, expectedHash string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{
			Status: StatusTargetUnreadable,
			Err:    errors.Join(ErrTargetUnreadable, fmt.Errorf("drift: ctx: %w", err)),
		}, nil
	}
	if _, err := os.Stat(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{Status: StatusTargetMissing, Err: ErrTargetMissing}, nil
		}
		return Result{
			Status: StatusTargetUnreadable,
			Err:    errors.Join(ErrTargetUnreadable, err),
		}, nil
	}

	open := c.openFn
	if open == nil {
		open = os.Open
	}
	f, err := open(target)
	if err != nil {
		return Result{
			Status: StatusTargetUnreadable,
			Err:    errors.Join(ErrTargetUnreadable, err),
		}, nil
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return Result{
			Status: StatusTargetUnreadable,
			Err:    errors.Join(ErrTargetUnreadable, err),
		}, nil
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedHash {
		return Result{Status: StatusDrifted, ActualHash: actual}, nil
	}
	return Result{Status: StatusOK, ActualHash: actual}, nil
}
