// Package doctor runs health checks on the royo-learn project environment.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Check represents the result of a single health check.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Message string `json:"message"`
	Detail string `json:"detail,omitempty"`
}

// Report aggregates the results of all registered checks.
type Report struct {
	Ok      bool    `json:"ok"`
	Summary string  `json:"summary"`
	Checks  []Check `json:"checks"`
}

// Status constants for checks.
const (
	StatusPass     = "pass"
	StatusFail     = "fail"
	StatusDegraded = "degraded"
	StatusSkipped  = "skipped"
)

// CheckFn is a function that executes a single health check.
// It receives the Runner context so checks can access shared state
// (logger, project root, fix-safe mode).
type CheckFn func(ctx context.Context, r *Runner) *Check

// ---------------------------------------------------------------------------
// Runner
// ---------------------------------------------------------------------------

// Runner holds the state needed to execute health checks.
type Runner struct {
	projectRoot  string
	trustedRoots []string
	fixSafe      bool
	logger       *slog.Logger

	mu         sync.RWMutex
	registry   map[string]CheckFn
	checkOrder []string // deterministic ordering

	closeOnce sync.Once
	closed    bool
}

// RunnerOption is a functional option for configuring a Runner.
type RunnerOption func(*Runner)

// WithProjectRoot sets the project root directory.
func WithProjectRoot(root string) RunnerOption {
	return func(r *Runner) {
		r.projectRoot = root
	}
}

// WithTrustedRoots sets the trusted root directories.
func WithTrustedRoots(roots []string) RunnerOption {
	return func(r *Runner) {
		r.trustedRoots = roots
	}
}

// WithFixSafe enables automatic safe repairs (create missing dirs, fix permissions).
func WithFixSafe(enabled bool) RunnerOption {
	return func(r *Runner) {
		r.fixSafe = enabled
	}
}

// WithLogger sets a structured logger for diagnostic output.
func WithLogger(logger *slog.Logger) RunnerOption {
	return func(r *Runner) {
		r.logger = logger
	}
}

// NewRunner creates a Runner with the given options and registers
// all built-in checks. Callers must call Close() when done.
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		logger:   slog.Default(),
		registry: make(map[string]CheckFn),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.registerBuiltinChecks()
	return r
}

// logf writes a debug message if a logger is configured.
func (r *Runner) logf(format string, args ...interface{}) {
	if r.logger != nil {
		r.logger.Debug(fmt.Sprintf(format, args...))
	}
}

// Register adds a named check to the registry. Registered order is preserved
// for deterministic output.
func (r *Runner) Register(name string, fn CheckFn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.registry[name]; !exists {
		r.checkOrder = append(r.checkOrder, name)
	}
	r.registry[name] = fn
}

// Run executes all registered checks in order and returns the aggregated report.
func (r *Runner) Run(ctx context.Context) (*Report, error) {
	r.mu.RLock()
	order := make([]string, len(r.checkOrder))
	copy(order, r.checkOrder)
	r.mu.RUnlock()

	report := &Report{Ok: true}
	var failed int
	var degraded int

	for _, name := range order {
		c := r.executeCheck(ctx, name)
		report.Checks = append(report.Checks, *c)

		switch c.Status {
		case StatusFail:
			failed++
		case StatusDegraded:
			degraded++
		case StatusSkipped:
			// skipped checks don't affect ok status
		}
	}

	// ok is false only when a required check fails.
	// degraded checks do NOT make the report fail.
	report.Ok = failed == 0

	if failed > 0 {
		report.Summary = fmt.Sprintf("%d check(s) failed, %d degraded, %d passed",
			failed, degraded, len(report.Checks)-failed-degraded-countSkipped(report.Checks))
	} else if degraded > 0 {
		report.Summary = fmt.Sprintf("all required checks passed, %d check(s) degraded", degraded)
	} else {
		report.Summary = fmt.Sprintf("all %d checks passed", len(report.Checks))
	}

	return report, nil
}

func countSkipped(checks []Check) int {
	n := 0
	for _, c := range checks {
		if c.Status == StatusSkipped {
			n++
		}
	}
	return n
}

// RunCheck executes a single named check. Returns an error if the check is
// not registered.
func (r *Runner) RunCheck(ctx context.Context, name string) (*Check, error) {
	r.mu.RLock()
	_, exists := r.registry[name]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("check %q is not registered", name)
	}

	c := r.executeCheck(ctx, name)
	return c, nil
}

// executeCheck runs one check by name, handling panics gracefully.
func (r *Runner) executeCheck(ctx context.Context, name string) *Check {
	r.mu.RLock()
	fn, exists := r.registry[name]
	r.mu.RUnlock()

	if !exists {
		return &Check{
			Name:    name,
			Status:  StatusFail,
			Message: "check not found in registry",
		}
	}

	// Recover from panics in check functions.
	var check *Check
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				check = &Check{
					Name:    name,
					Status:  StatusFail,
					Message: fmt.Sprintf("check panicked: %v", rec),
				}
				r.logf("check %q panicked: %v", name, rec)
			}
		}()
		check = fn(ctx, r)
	}()

	if check == nil {
		check = &Check{
			Name:    name,
			Status:  StatusFail,
			Message: "check returned nil",
		}
	}

	// Clamp status to known values.
	switch check.Status {
	case StatusPass, StatusFail, StatusDegraded, StatusSkipped:
		// valid
	default:
		check.Status = StatusFail
		check.Message = "check returned invalid status: " + strings.TrimPrefix(check.Message, "")
	}

	return check
}

// Close releases resources held by the Runner. It is safe to call multiple times.
func (r *Runner) Close() error {
	r.closeOnce.Do(func() {
		r.closed = true
	})
	return nil
}

// ---------------------------------------------------------------------------
// JSON helpers (used by CLI when --json flag is active)
// ---------------------------------------------------------------------------

// ToJSON marshals the report to stable indented JSON.
func (r *Report) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

