package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agent-royo-learn/internal/buildinfo"
	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/config"
	"agent-royo-learn/internal/curate"
	"agent-royo-learn/internal/doctor"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/engram"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/logging"
	"agent-royo-learn/internal/project"
	"agent-royo-learn/internal/publish"
	"agent-royo-learn/internal/recurrence"
	"agent-royo-learn/internal/selfupdate"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// Exit codes as documented in docs/04-CLI-SPEC.md.
const (
	exitSuccess          = 0
	exitFailure          = 1
	exitInvalidArguments = 2

	exitProjectNotFound  = 4
	exitAmbiguousProject = 5
)

const (
	invalidArgumentsMessage    = `invalid arguments: expected "version --json"`
	invalidArgumentsNextAction = `run "royo-learn version --json"`
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return printHelp(stdout)
	}

	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "capture":
		return runCapture(args[1:], stdout, stderr)
	case "curate":
		return runCurate(args[1:], stdout, stderr)
	case "get":
		return runGet(args[1:], stdout, stderr)
	case "search":
		return runSearch(args[1:], stdout, stderr)
	case "occurrence":
		return runOccurrence(args[1:], stdout, stderr)
	case "evidence":
		return runEvidence(args[1:], stdout, stderr)
	case "preview":
		return runPreview(args[1:], stdout, stderr)
	case "approve":
		return runApprove(args[1:], stdout, stderr)
	case "publish":
		return runPublish(args[1:], stdout, stderr)
	case "rollback":
		return runRollback(args[1:], stdout, stderr)
	case "mcp-serve":
		return runMCPServe(args[1:], stdout, stderr)
	case "engram-health":
		return runEngramHealth(args[1:], stdout, stderr)
	case "engram-search":
		return runEngramSearch(args[1:], stdout, stderr)
	case "recurrences":
		return runRecurrences(args[1:], stdout, stderr)
	case "metrics":
		return runMetrics(args[1:], stdout, stderr)
	case "e2e":
		return runE2E(args[1:], stdout, stderr)
	case "setup":
		return runSetup(args[1:], stdout, stderr)
	case "self-update":
		return runSelfUpdate(args[1:], stdout, stderr)
	default:
		return writeUnknownCommandError(stderr)
	}
}

func printHelp(stdout io.Writer) int {
	_, _ = fmt.Fprintf(stdout, `royo-learn — Capture, curate, and publish reusable learnings from AI-assisted development.

Usage:
  royo-learn <command> [flags]

Commands:
  init           Initialize a new royo-learn project
  mcp-serve      Start the MCP server over stdio
  capture        Capture a new learning
  curate         Curate an existing learning
  get            Retrieve a single learning by ID
  occurrence     Record a recurrence of a learning's pattern
  preview        Preview publication of a learning
  approve        Approve a publication preview (human authorization)
  publish        Publish a curated learning
  rollback       Rollback a published learning
  doctor         Run system diagnostics
  search         Search captured learnings
  engram-health  Check Engram connection health
  engram-search  Search Engram memory
  recurrences    List recurrence records
  metrics        Show learning metrics
  e2e            Run end-to-end tests
  setup          Configure the tool for first use
  version        Print version information
  self-update    Update to the latest or a specific version

Global flags:
  --project-root string   Explicit project root (most commands)

Run "royo-learn <command> --help" for command-specific flags.
`)
	return exitSuccess
}

func writeUnknownCommandError(stderr io.Writer) int {
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        "invalid_argument",
		Message:     invalidArgumentsMessage,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  invalidArgumentsNextAction,
	})
	return exitInvalidArguments
}

// ---------------------------------------------------------------------------
// version
// ---------------------------------------------------------------------------

func runVersion(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	if err := fs.Parse(args); err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:        "invalid_argument",
			Message:     err.Error(),
			Recoverable: true,
			Details:     map[string]any{},
			NextAction:  invalidArgumentsNextAction,
		})
		return exitInvalidArguments
	}

	if !*jsonFlag {
		return writeVersionHuman(stdout, stderr)
	}

	return writeVersionJSON(stdout, stderr)
}

func writeVersionHuman(stdout, stderr io.Writer) int {
	if _, err := fmt.Fprint(stdout, buildinfo.HumanString()); err != nil {
		_ = logging.WriteDiagnostic(stderr, "could not write version summary")
		return exitFailure
	}
	return exitSuccess
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

// ---------------------------------------------------------------------------
// init
// ---------------------------------------------------------------------------

const royoLearnDir = ".royo-learn"

func runInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root directory (required)")
	force := fs.Bool("force", false, "recreate generated files (config.yaml, .gitignore); never touches records")
	if err := fs.Parse(args); err != nil {
		return writeInitError(stderr, "invalid_argument", "init: %v", err)
	}

	if *projectRoot == "" {
		return writeInitError(stderr, "invalid_argument", "init: --project-root is required")
	}

	absRoot, err := filepath.Abs(*projectRoot)
	if err != nil {
		return writeInitError(stderr, "invalid_argument", "init: cannot resolve --project-root: %v", err)
	}

	royoPath := filepath.Join(absRoot, royoLearnDir)

	// Create subdirectories (records/, evidence/, backups/).
	for _, sub := range []string{"records", "evidence", "backups"} {
		dir := filepath.Join(royoPath, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return writeInitError(stderr, "invalid_argument", "init: cannot create %s: %v", dir, err)
		}
	}

	// Write config.yaml — only if missing or --force.
	configPath := filepath.Join(royoPath, "config.yaml")
	written, err := writeFileIfMissing(configPath, *force, func() ([]byte, error) {
		return marshalDefaultConfig()
	})
	if err != nil {
		return writeInitError(stderr, "invalid_argument", "init: cannot write config.yaml: %v", err)
	}
	if written {
		_, _ = fmt.Fprintf(stdout, "created %s\n", configPath)
	}

	// Write .gitignore — only if missing or --force.
	gitignorePath := filepath.Join(royoPath, ".gitignore")
	gitignoreContent := []byte("# royo-learn\nroyo-learn.db\n")
	written, err = writeFileIfMissing(gitignorePath, *force, func() ([]byte, error) {
		return gitignoreContent, nil
	})
	if err != nil {
		return writeInitError(stderr, "invalid_argument", "init: cannot write .gitignore: %v", err)
	}
	if written {
		_, _ = fmt.Fprintf(stdout, "created %s\n", gitignorePath)
	}

	return exitSuccess
}

// writeFileIfMissing writes content at path only when the file does not exist
// or force is true. Returns (true, nil) when the file was written,
// (false, nil) when skipped because the file already exists and !force,
// and (false, error) on I/O errors.
func writeFileIfMissing(path string, force bool, contentFn func() ([]byte, error)) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil // skip: file already exists
		}
	}

	content, err := contentFn()
	if err != nil {
		return false, err
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func marshalDefaultConfig() ([]byte, error) {
	cfg := config.DefaultConfig()
	return yamlMarshalConfig(cfg)
}

// yamlMarshalConfig writes the config as human-readable YAML.
func yamlMarshalConfig(cfg *config.Config) ([]byte, error) {
	return []byte(fmt.Sprintf(`# royo-learn project configuration
# Generated by: royo-learn init
version: %d

project:
  records_root: %s
  evidence_root: %s
  backup_root: %s

database:
  path: %s

records:
  dir: %s

evidence:
  dir: %s

publishing:
  dry_run_default: %t

limits:
  max_payload_bytes: %d
`, cfg.Version,
		cfg.Project.RecordsRoot,
		cfg.Project.EvidenceRoot,
		cfg.Project.BackupRoot,
		cfg.Database.Path,
		cfg.Records.Dir,
		cfg.Evidence.Dir,
		cfg.Publishing.DryRunDefault,
		cfg.Limits.MaxPayloadBytes,
	)), nil
}

func writeInitError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn init --project-root <dir>"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// doctor
// ---------------------------------------------------------------------------

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "explicit project root directory (optional)")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	fixSafe := fs.Bool("fix-safe", false, "auto-repair safe issues")
	// --check is repeatable; parse manually since flag.Visit + explicit
	// tracking avoids the limitation of flag.String (which only captures one value).
	checkFilters := make(map[string]bool)
	fs.Func("check", "filter to specific check(s) (repeatable)", func(val string) error {
		checkFilters[val] = true
		return nil
	})

	if err := fs.Parse(args); err != nil {
		return writeDoctorError(stderr, "invalid_argument", "doctor: %v", err)
	}

	// Resolve project root.
	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	req := &project.ResolveRequest{
		CWD:          cwd,
		ExplicitRoot: *projectRoot,
	}

	proj, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		return mapProjectError(stderr, err)
	}

	root := proj.Root

	// Verify the project root has a .royo-learn/config.yaml marker.
	// The resolver can succeed without a marker (e.g., explicit root
	// that just happens to be a valid directory). Doctor requires the
	// marker to confirm this is an initialized project.
	markerPath := filepath.Join(root, ".royo-learn", "config.yaml")
	if _, statErr := os.Stat(markerPath); statErr != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    project.ErrProjectNotFound,
			Message: fmt.Sprintf("no project marker found at %s", root),
		})
		return exitProjectNotFound
	}

	runner := doctor.NewRunner(
		doctor.WithProjectRoot(root),
		doctor.WithTrustedRoots([]string{root}),
		doctor.WithFixSafe(*fixSafe),
	)
	defer runner.Close()

	var report *doctor.Report
	if len(checkFilters) > 0 {
		// Run specific checks.
		report = &doctor.Report{Ok: true}
		for name := range checkFilters {
			check, checkErr := runner.RunCheck(context.Background(), name)
			if checkErr != nil {
				return writeDoctorError(stderr, "invalid_argument", "doctor: %v", checkErr)
			}
			if check != nil {
				report.Checks = append(report.Checks, *check)
				if check.Status == doctor.StatusFail {
					report.Ok = false
				}
			}
		}
		report.Summary = fmt.Sprintf("%d check(s) executed", len(report.Checks))
	} else {
		report, err = runner.Run(context.Background())
		if err != nil {
			return writeDoctorError(stderr, "invalid_argument", "doctor: %v", err)
		}
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return writeDoctorError(stderr, "invalid_argument", "doctor: cannot marshal report: %v", err)
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "%s\n", report.Summary)
		for _, c := range report.Checks {
			_, _ = fmt.Fprintf(stdout, "  [%s] %s: %s\n", c.Status, c.Name, c.Message)
		}
	}

	if report.Ok {
		return exitSuccess
	}
	return exitFailure
}

// mapProjectError converts a project.Error to the appropriate exit code
// and writes the error envelope to stderr.
func mapProjectError(stderr io.Writer, err error) int {
	var pErr *project.Error
	if errors.As(err, &pErr) {
		switch pErr.Code {
		case project.ErrProjectNotFound:
			_ = logging.WriteError(stderr, logging.ErrorEnvelope{
				Code:    project.ErrProjectNotFound,
				Message: pErr.Message,
			})
			return exitProjectNotFound
		case project.ErrAmbiguousProject:
			_ = logging.WriteError(stderr, logging.ErrorEnvelope{
				Code:    project.ErrAmbiguousProject,
				Message: pErr.Message,
			})
			return exitAmbiguousProject
		}
	}
	return writeDoctorError(stderr, "invalid_argument", "doctor: %v", err)
}

func writeDoctorError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn doctor"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// capture
// ---------------------------------------------------------------------------

func runCapture(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	title := fs.String("title", "", "learning title (required)")
	contextStr := fs.String("context", "", "learning context (required)")
	observation := fs.String("observation", "", "what was observed (required)")
	lesson := fs.String("lesson", "", "reusable lesson (required)")
	learningType := fs.String("type", "procedure", "learning type")
	pScope := fs.String("scope", "project", "scope guess")
	destination := fs.String("destination", "project", "proposed destination: none, project, shared, skill, agents_rule")
	evidenceLevel := fs.String("evidence-level", "insufficient", "declared evidence level: strong, moderate, weak, insufficient")
	confidence := fs.String("confidence", "medium", "confidence: low, medium, high")
	idempotencyKey := fs.String("idempotency-key", "", "the same key on a retry returns the existing learning without duplicating its evidence (D5)")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	// Evidence flags. Collectors are limited to supplied evidence, git status,
	// git diff, and an explicitly allowed command — not an open taxonomy.
	evidenceFile := fs.String("evidence-file", "", "path to a JSON array of evidence records {kind,summary,source,content}")
	evidenceSummary := fs.String("evidence-summary", "", "summary of a single inline evidence record")
	evidenceContent := fs.String("evidence-content", "", "literal content of a single inline evidence record")
	evidenceSource := fs.String("evidence-source", "", "origin of a single inline evidence record")
	evidenceKind := fs.String("evidence-kind", "text", "kind of a single inline evidence record")
	file := fs.String("file", "", "attach the contents of this file as an evidence record")
	stdinFlag := fs.Bool("stdin", false, "attach the content read from stdin as an evidence record")
	collectGitStatus := fs.Bool("collect-git-status", false, "attach `git status --porcelain` as evidence")
	collectGitDiff := fs.Bool("collect-git-diff", false, "attach the working-tree `git diff` as evidence")

	if err := fs.Parse(args); err != nil {
		return writeCaptureError(stderr, "invalid_argument", "capture: %v", err)
	}

	if *title == "" {
		return writeCaptureError(stderr, "invalid_argument", "capture: --title is required")
	}
	if *contextStr == "" {
		return writeCaptureError(stderr, "invalid_argument", "capture: --context is required")
	}
	if *observation == "" {
		return writeCaptureError(stderr, "invalid_argument", "capture: --observation is required")
	}
	if *lesson == "" {
		return writeCaptureError(stderr, "invalid_argument", "capture: --lesson is required")
	}

	// Resolve project root.
	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	req := &project.ResolveRequest{
		CWD:          cwd,
		ExplicitRoot: *projectRoot,
	}

	proj, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		return mapProjectError(stderr, err)
	}

	root := proj.Root

	// Open database.
	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		return writeCaptureError(stderr, "invalid_argument", "capture: cannot open database: %v", err)
	}
	defer db.Close()

	// Run migrations.
	if err := storage.Migrate(db); err != nil {
		return writeCaptureError(stderr, "invalid_argument", "capture: migration failed: %v", err)
	}

	// Save project if not yet registered.
	ctx := context.Background()
	var projectID domain.ProjectID
	tx, _ := db.DB.BeginTx(ctx, nil)
	if existing, _ := storage.GetProjectByKey(ctx, tx, proj.Key); existing == nil {
		projEntity := &domain.Project{
			ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
			ProjectKey:    proj.Key,
			DisplayName:   proj.Key,
			CanonicalPath: root,
			GitRemote:     proj.GitRemote,
			Fingerprint:   proj.Key,
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}
		if err := storage.SaveProject(ctx, tx, projEntity); err != nil {
			tx.Rollback()
			return writeCaptureError(stderr, "invalid_argument", "capture: save project: %v", err)
		}
		projectID = projEntity.ID
	} else {
		projectID = existing.ID
	}
	if err := tx.Commit(); err != nil {
		return writeCaptureError(stderr, "invalid_argument", "capture: commit project: %v", err)
	}

	recordsDir := filepath.Join(root, ".royo-learn", "records")
	evidenceSvc, err := evidence.NewService(root, nil)
	if err != nil {
		return writeCaptureError(stderr, "invalid_argument", "capture: init evidence: %v", err)
	}
	svc := capture.NewServiceWithEvidence(db, recordsDir, evidenceSvc)

	items, err := collectCaptureEvidence(ctx, evidenceSvc, root, captureEvidenceFlags{
		evidenceFile:     *evidenceFile,
		evidenceSummary:  *evidenceSummary,
		evidenceContent:  *evidenceContent,
		evidenceSource:   *evidenceSource,
		evidenceKind:     *evidenceKind,
		file:             *file,
		stdin:            *stdinFlag,
		collectGitStatus: *collectGitStatus,
		collectGitDiff:   *collectGitDiff,
	})
	if err != nil {
		return writeCaptureError(stderr, "invalid_argument", "capture: %v", err)
	}

	input := &capture.CaptureInput{
		Title:          *title,
		Context:        *contextStr,
		Observation:    *observation,
		Lesson:         *lesson,
		Type:           domain.LearningType(*learningType),
		Scope:          domain.Scope(*pScope),
		Destination:    domain.DestinationType(*destination),
		Confidence:     domain.Confidence(*confidence),
		EvidenceLevel:  domain.EvidenceLevel(*evidenceLevel),
		IdempotencyKey: *idempotencyKey,
		Evidence:       items,
		Actor: domain.Actor{
			Kind:      "human",
			Name:      "cli-user",
			Model:     "",
			SessionID: "",
		},
	}

	result, err := svc.Capture(ctx, projectID, input)
	if err != nil {
		return writeDomainError(stderr, err, "invalid_argument", `run "royo-learn capture --help"`, "capture: ")
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"learning_id":    result.LearningID,
			"status":         result.Status,
			"new":            result.New,
			"evidence_count": len(result.EvidenceIDs),
			"evidence_ids":   evidenceIDStrings(result.EvidenceIDs),
			"redacted":       result.Redacted,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		statusLabel := "new"
		if !result.New {
			statusLabel = "deduplicated (existing)"
		}
		_, _ = fmt.Fprintf(stdout, "captured %s learning %s (status: %s, evidence: %d)\n",
			statusLabel, result.LearningID, result.Status, len(result.EvidenceIDs))
	}

	return exitSuccess
}

// captureEvidenceFlags carries the evidence-related capture flags.
type captureEvidenceFlags struct {
	evidenceFile     string
	evidenceSummary  string
	evidenceContent  string
	evidenceSource   string
	evidenceKind     string
	file             string
	stdin            bool
	collectGitStatus bool
	collectGitDiff   bool
}

// collectCaptureEvidence assembles evidence items from the supplied flags. The
// only sources are: an evidence JSON file, a single inline record, a file, stdin,
// `git status` and `git diff`. Redaction happens later, inside the evidence
// service, before any write.
func collectCaptureEvidence(ctx context.Context, svc *evidence.Service, root string, f captureEvidenceFlags) ([]evidence.Item, error) {
	var items []evidence.Item

	if f.evidenceFile != "" {
		fileItems, err := readEvidenceFile(f.evidenceFile)
		if err != nil {
			return nil, err
		}
		items = append(items, fileItems...)
	}

	if f.evidenceSummary != "" || f.evidenceContent != "" {
		if f.evidenceSummary == "" {
			return nil, fmt.Errorf("--evidence-summary is required when --evidence-content is set")
		}
		items = append(items, evidence.Item{
			Kind:    domain.EvidenceKind(f.evidenceKind),
			Summary: f.evidenceSummary,
			Source:  f.evidenceSource,
			Content: f.evidenceContent,
		})
	}

	if f.file != "" {
		data, err := os.ReadFile(f.file)
		if err != nil {
			return nil, fmt.Errorf("read --file: %w", err)
		}
		items = append(items, evidence.Item{
			Kind:    domain.KindFile,
			Summary: fmt.Sprintf("contents of %s", filepath.Base(f.file)),
			Source:  f.file,
			Content: string(data),
		})
	}

	if f.stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read --stdin: %w", err)
		}
		items = append(items, evidence.Item{
			Kind:    domain.KindText,
			Summary: "content read from stdin",
			Content: string(data),
		})
	}

	if f.collectGitStatus {
		item, err := svc.CollectGitStatus(ctx, root)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if f.collectGitDiff {
		item, err := svc.CollectGitDiff(ctx, root)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

// evidenceFileRecord is the wire form of one record in an --evidence-file array.
type evidenceFileRecord struct {
	Kind    string `json:"kind"`
	Type    string `json:"type"`
	Summary string `json:"summary"`
	Source  string `json:"source"`
	Content string `json:"content"`
}

func readEvidenceFile(path string) ([]evidence.Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read --evidence-file: %w", err)
	}
	var records []evidenceFileRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse --evidence-file: %w", err)
	}
	items := make([]evidence.Item, 0, len(records))
	for i, r := range records {
		kind := r.Kind
		if kind == "" {
			kind = r.Type
		}
		if kind == "" {
			kind = string(domain.KindText)
		}
		if r.Summary == "" {
			return nil, fmt.Errorf("--evidence-file[%d]: summary is required", i)
		}
		items = append(items, evidence.Item{
			Kind:    domain.EvidenceKind(kind),
			Summary: r.Summary,
			Source:  r.Source,
			Content: r.Content,
		})
	}
	return items, nil
}

// evidenceIDStrings converts evidence IDs to their string form for JSON output.
func evidenceIDStrings(ids []domain.EvidenceID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func writeCaptureError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn capture --help"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// curate
// ---------------------------------------------------------------------------

func runCurate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("curate", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning ID to curate (required)")
	action := fs.String("action", "", "curation action: approve, approve_new_skill, approve_skill_update, reject, needs_evidence, relate (required)")
	targetID := fs.String("target-id", "", "target learning ID for relate action")
	relation := fs.String("relation", "related", "relation type for relate action")
	rationale := fs.String("rationale", "", "rationale for the curation decision")
	area := fs.String("area", "", "explicit skill area (sanitized+lowercased); overrides automatic derivation from retrieval_terms for skill decisions")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writeCurateError(stderr, "invalid_argument", "curate: %v", err)
	}

	if *learningID == "" {
		return writeCurateError(stderr, "invalid_argument", "curate: --learning-id is required")
	}
	if *action == "" {
		return writeCurateError(stderr, "invalid_argument", "curate: --action is required")
	}

	// Resolve project root.
	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	req := &project.ResolveRequest{
		CWD:          cwd,
		ExplicitRoot: *projectRoot,
	}

	proj, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		return mapProjectError(stderr, err)
	}

	root := proj.Root

	// Open database.
	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		return writeCurateError(stderr, "invalid_argument", "curate: cannot open database: %v", err)
	}
	defer db.Close()

	// Run migrations.
	if err := storage.Migrate(db); err != nil {
		return writeCurateError(stderr, "invalid_argument", "curate: migration failed: %v", err)
	}

	// Save project if not yet registered.
	ctx := context.Background()
	var projectID domain.ProjectID
	tx, txErr := db.DB.BeginTx(ctx, nil)
	if txErr != nil {
		return writeCurateError(stderr, "invalid_argument", "curate: begin tx: %v", txErr)
	}
	if existing, _ := storage.GetProjectByKey(ctx, tx, proj.Key); existing == nil {
		projEntity := &domain.Project{
			ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
			ProjectKey:    proj.Key,
			DisplayName:   proj.Key,
			CanonicalPath: root,
			GitRemote:     proj.GitRemote,
			Fingerprint:   proj.Key,
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}
		if err := storage.SaveProject(ctx, tx, projEntity); err != nil {
			tx.Rollback()
			return writeCurateError(stderr, "invalid_argument", "curate: save project: %v", err)
		}
		projectID = projEntity.ID
	} else {
		projectID = existing.ID
	}
	if err := tx.Commit(); err != nil {
		return writeCurateError(stderr, "invalid_argument", "curate: commit project: %v", err)
	}

	recordsDir := filepath.Join(root, ".royo-learn", "records")
	svc := curate.NewService(db, recordsDir)

	actor := domain.Actor{
		Kind:      "human",
		Name:      "cli-user",
		Model:     "",
		SessionID: "",
	}

	// Handle "relate" action.
	if *action == "relate" {
		if *targetID == "" {
			return writeCurateError(stderr, "invalid_argument", "curate: --target-id is required for relate action")
		}
		relateInput := &curate.RelateInput{
			SourceLearningID: domain.LearningID(*learningID),
			TargetLearningID: domain.LearningID(*targetID),
			RelationType:     domain.RelationType(*relation),
			Rationale:        *rationale,
			Actor:            actor,
		}
		result, relErr := svc.Relate(ctx, relateInput)
		if relErr != nil {
			return writeCurateError(stderr, "invalid_argument", "curate: %v", relErr)
		}
		if *jsonFlag {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"relation_id":        result.RelationID,
				"source_learning_id": *learningID,
				"target_learning_id": *targetID,
				"relation":           *relation,
			}, "", "  ")
			_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
		} else {
			_, _ = fmt.Fprintf(stdout, "relation %s created between %s and %s\n", result.RelationID, *learningID, *targetID)
		}
		return exitSuccess
	}

	// Handle curate action (approve / reject / needs_evidence).
	decision, decErr := parseCurateAction(*action)
	if decErr != nil {
		return writeCurateError(stderr, "invalid_argument", "curate: %v", decErr)
	}

	curateInput := &curate.CurateInput{
		LearningID: domain.LearningID(*learningID),
		Decision:   decision,
		Rationale:  *rationale,
		Actor:      actor,
		Area:       *area,
	}

	result, curErr := svc.Curate(ctx, projectID, curateInput)
	if curErr != nil {
		return writeDomainError(stderr, curErr, "invalid_argument", `run "royo-learn curate --help"`, "curate: ")
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"curation_id": result.CurationID,
			"learning_id": result.LearningID,
			"new_status":  result.NewStatus,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "curated learning %s: %s -> %s (curation: %s)\n",
			result.LearningID, *action, result.NewStatus, result.CurationID)
	}

	return exitSuccess
}

// parseCurateAction maps a CLI action string to a CurationDecision. Every value
// is validated against the single canonical allowlist in internal/domain
// (D11 §11.2), so the CLI and the MCP server accept exactly the same decisions.
// The historical shortcut "approve" is kept as a deprecated alias of
// approve_project_knowledge; "relate" is handled by the caller before this point.
func parseCurateAction(action string) (domain.CurationDecision, error) {
	if action == "approve" {
		return domain.CurationApproveProjectKnowledge, nil
	}
	return domain.ParseCurationDecision(action)
}

func writeCurateError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn curate --help"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// evidence (add / list)
// ---------------------------------------------------------------------------

func runEvidence(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeEvidenceError(stderr, "invalid_argument", "evidence: a subcommand is required: add, list")
	}
	switch args[0] {
	case "add":
		return runEvidenceAdd(args[1:], stdout, stderr)
	case "list":
		return runEvidenceList(args[1:], stdout, stderr)
	default:
		return writeEvidenceError(stderr, "invalid_argument", "evidence: unknown subcommand %q: must be add or list", args[0])
	}
}

// runEvidenceAdd attaches evidence to an existing learning through the public
// capture service. The learning ID is the first positional argument.
func runEvidenceAdd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return writeEvidenceError(stderr, "invalid_argument", "evidence add: a learning id is required as the first argument")
	}
	learningID := args[0]

	fs := flag.NewFlagSet("evidence add", flag.ContinueOnError)
	kind := fs.String("kind", "text", "evidence kind")
	summary := fs.String("summary", "", "human-readable summary (required)")
	source := fs.String("source", "", "origin: a path, a command or a URL")
	content := fs.String("content", "", "literal content; stored in the blob store after redaction")
	evidenceFile := fs.String("evidence-file", "", "path to a JSON array of evidence records")
	evidenceLevel := fs.String("evidence-level", "", "optionally raise the declared evidence level")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args[1:]); err != nil {
		return writeEvidenceError(stderr, "invalid_argument", "evidence add: %v", err)
	}

	var items []evidence.Item
	if *evidenceFile != "" {
		fileItems, err := readEvidenceFile(*evidenceFile)
		if err != nil {
			return writeEvidenceError(stderr, "invalid_argument", "evidence add: %v", err)
		}
		items = append(items, fileItems...)
	}
	if *summary != "" || *content != "" {
		if *summary == "" {
			return writeEvidenceError(stderr, "invalid_argument", "evidence add: --summary is required")
		}
		items = append(items, evidence.Item{
			Kind:    domain.EvidenceKind(*kind),
			Summary: *summary,
			Source:  *source,
			Content: *content,
		})
	}
	if len(items) == 0 {
		return writeEvidenceError(stderr, "invalid_argument", "evidence add: at least one evidence record is required")
	}

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	svc, err := newEvidenceCaptureSvc(root, db)
	if err != nil {
		return writeDomainError(stderr, err, "invalid_argument", `run "royo-learn evidence add --help"`, "evidence add: ")
	}

	result, err := svc.AddEvidence(ctx, projectID, &capture.AddEvidenceInput{
		LearningID:    domain.LearningID(learningID),
		Items:         items,
		EvidenceLevel: domain.EvidenceLevel(*evidenceLevel),
		Actor:         domain.Actor{Kind: "human", Name: "cli-user"},
	})
	if err != nil {
		return writeDomainError(stderr, err, "invalid_argument", `run "royo-learn evidence add --help"`, "evidence add: ")
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"learning_id":    string(result.LearningID),
			"evidence_count": result.Count,
			"evidence_ids":   evidenceIDStrings(result.EvidenceIDs),
			"evidence_level": string(result.EvidenceLevel),
			"redacted":       result.Redacted,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "attached %d evidence record(s) to learning %s\n", result.Count, result.LearningID)
	}

	return exitSuccess
}

// runEvidenceList lists the evidence attached to a learning. Every field it
// prints was redacted before it was persisted.
func runEvidenceList(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return writeEvidenceError(stderr, "invalid_argument", "evidence list: a learning id is required as the first argument")
	}
	learningID := args[0]

	fs := flag.NewFlagSet("evidence list", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args[1:]); err != nil {
		return writeEvidenceError(stderr, "invalid_argument", "evidence list: %v", err)
	}

	root, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	svc, err := newEvidenceCaptureSvc(root, db)
	if err != nil {
		return writeEvidenceError(stderr, "invalid_argument", "evidence list: %v", err)
	}

	records, err := svc.ListEvidence(ctx, domain.LearningID(learningID))
	if err != nil {
		return writeEvidenceError(stderr, "invalid_argument", "evidence list: %v", err)
	}

	if *jsonFlag {
		items := make([]map[string]any, 0, len(records))
		for _, r := range records {
			items = append(items, map[string]any{
				"id":           string(r.ID),
				"learning_id":  string(r.LearningID),
				"kind":         string(r.Kind),
				"summary":      r.Summary,
				"source":       r.URI,
				"sha256":       r.SHA256,
				"redacted":     r.Redacted,
				"size_bytes":   r.SizeBytes,
				"collected_at": r.CollectedAt.Format(time.RFC3339),
			})
		}
		data, _ := json.MarshalIndent(map[string]interface{}{
			"learning_id":    learningID,
			"evidence_count": len(records),
			"evidence":       items,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Evidence for learning %s (%d record(s)):\n", learningID, len(records))
		for _, r := range records {
			_, _ = fmt.Fprintf(stdout, "  [%s] %s\n", r.Kind, r.Summary)
		}
		if len(records) == 0 {
			_, _ = fmt.Fprintf(stdout, "  (none)\n")
		}
	}

	return exitSuccess
}

// newEvidenceCaptureSvc builds a capture service wired to the evidence layer.
func newEvidenceCaptureSvc(root string, db *storage.DB) (*capture.Service, error) {
	evidenceSvc, err := evidence.NewService(root, nil)
	if err != nil {
		return nil, fmt.Errorf("init evidence: %w", err)
	}
	recordsDir := filepath.Join(root, ".royo-learn", "records")
	return capture.NewServiceWithEvidence(db, recordsDir, evidenceSvc), nil
}

func writeEvidenceError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn evidence add <learning-id> --summary <text>"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// preview
// ---------------------------------------------------------------------------

func runPreview(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning ID to preview (required)")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writePublishError(stderr, "invalid_argument", "preview: %v", err)
	}

	if *learningID == "" {
		return writePublishError(stderr, "invalid_argument", "preview: --learning-id is required")
	}

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := publish.NewService(db, root,
		filepath.Join(root, ".royo-learn", "backups"),
		filepath.Join(root, ".royo-learn"),
		filepath.Join(root, ".royo-learn", "records"))

	ctx := context.Background()
	result, err := svc.Preview(ctx, projectID, &publish.PreviewInput{
		LearningID: domain.LearningID(*learningID),
		Actor: domain.Actor{
			Kind: "human", Name: "cli-user", Model: "", SessionID: "",
		},
	})
	if err != nil {
		return writeDomainError(stderr, err, "invalid_argument", `run "royo-learn preview --help"`, "preview: ")
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"preview_id":        result.Preview.ID,
			"preview_hash":      result.Preview.PreviewHash,
			"risk":              result.Preview.Risk,
			"requires_approval": result.Preview.RequiresApproval,
			"diff":              result.Diff,
			"policies":          result.Policies,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Preview %s for learning %s\n", result.Preview.ID, *learningID)
		_, _ = fmt.Fprintf(stdout, "  Hash: %s\n", result.Preview.PreviewHash)
		_, _ = fmt.Fprintf(stdout, "  Risk: %s\n", result.Preview.Risk)
		_, _ = fmt.Fprintf(stdout, "  Requires approval: %v\n", result.Preview.RequiresApproval)
		for _, p := range result.Policies {
			_, _ = fmt.Fprintf(stdout, "  Policy [%s]: passed=%v — %s\n", p.PolicyName, p.Passed, p.Reason)
		}
	}

	return exitSuccess
}

// ---------------------------------------------------------------------------
// approve
// ---------------------------------------------------------------------------

// runApprove records explicit human approval bound to a publication preview.
// The learning id is the first positional argument. In --json mode there are no
// interactive prompts and every field is required; the response carries the
// approval_id (D11 §11.1).
func runApprove(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return writePublishError(stderr, "invalid_argument", "approve: a learning id is required as the first argument")
	}
	learningID := args[0]

	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	previewHash := fs.String("preview-hash", "", "hash of the exact preview being authorized (required)")
	approvedBy := fs.String("approved-by", "", "identity of the human granting approval (required)")
	reason := fs.String("reason", "", "why the publication is authorized (required)")
	approvalEvidence := fs.String("approval-evidence", "", "reference to the consent record: link, message id or ticket (required)")
	expiresAt := fs.String("expires-at", "", "optional RFC3339 instant after which the approval is rejected")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args[1:]); err != nil {
		return writePublishError(stderr, "invalid_argument", "approve: %v", err)
	}

	if *previewHash == "" {
		return writePublishError(stderr, "invalid_argument", "approve: --preview-hash is required")
	}
	if *approvedBy == "" {
		return writePublishError(stderr, "invalid_argument", "approve: --approved-by is required")
	}
	if *reason == "" {
		return writePublishError(stderr, "invalid_argument", "approve: --reason is required")
	}
	if *approvalEvidence == "" {
		return writePublishError(stderr, "invalid_argument", "approve: --approval-evidence is required")
	}

	apprIn := &publish.ApproveInput{
		LearningID:       domain.LearningID(learningID),
		PreviewHash:      *previewHash,
		ApprovedBy:       *approvedBy,
		Reason:           *reason,
		ApprovalEvidence: *approvalEvidence,
		Actor:            domain.Actor{Kind: "human", Name: "cli-user"},
	}
	if *expiresAt != "" {
		t, err := time.Parse(time.RFC3339, *expiresAt)
		if err != nil {
			return writePublishError(stderr, "invalid_argument", "approve: --expires-at must be RFC3339: %v", err)
		}
		apprIn.ExpiresAt = &t
	}

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := publish.NewService(db, root,
		filepath.Join(root, ".royo-learn", "backups"),
		filepath.Join(root, ".royo-learn"),
		filepath.Join(root, ".royo-learn", "records"))

	ctx := context.Background()
	approval, err := svc.Approve(ctx, projectID, apprIn)
	if err != nil {
		return writeDomainError(stderr, err, "invalid_argument", `run "royo-learn approve --help"`, "approve: ")
	}

	if *jsonFlag {
		out := map[string]interface{}{
			"approval_id":  string(approval.ID),
			"learning_id":  string(approval.LearningID),
			"preview_hash": approval.PreviewHash,
			"approved_by":  approval.ApprovedBy,
		}
		if approval.ExpiresAt != nil {
			out["expires_at"] = approval.ExpiresAt.Format(time.RFC3339)
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Approved preview %s for learning %s (approval: %s)\n",
			approval.PreviewHash, approval.LearningID, approval.ID)
	}

	return exitSuccess
}

// ---------------------------------------------------------------------------
// publish
// ---------------------------------------------------------------------------

func runPublish(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning ID to publish (required)")
	previewHash := fs.String("preview-hash", "", "preview hash to confirm (required)")
	approvalID := fs.String("approval-id", "", "approval ID (required when the preview reports requires_approval=true)")
	apply := fs.Bool("apply", false, "actually write the files; without it publish is a dry run (D7)")
	dryRun := fs.Bool("dry-run", true, "when false, equivalent to --apply; the default is a dry run")
	force := fs.Bool("force", false, "bypass dirty worktree check")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writePublishError(stderr, "invalid_argument", "publish: %v", err)
	}

	if *learningID == "" {
		return writePublishError(stderr, "invalid_argument", "publish: --learning-id is required")
	}
	if *previewHash == "" {
		return writePublishError(stderr, "invalid_argument", "publish: --preview-hash is required")
	}

	// --apply and --dry-run=false are equivalent; either one enables the write.
	writeRequested := *apply || !*dryRun

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := publish.NewService(db, root,
		filepath.Join(root, ".royo-learn", "backups"),
		filepath.Join(root, ".royo-learn"),
		filepath.Join(root, ".royo-learn", "records"))

	pubIn := &publish.PublishInput{
		LearningID:  domain.LearningID(*learningID),
		PreviewHash: *previewHash,
		Apply:       writeRequested,
		Force:       *force,
		Actor: domain.Actor{
			Kind: "human", Name: "cli-user", Model: "", SessionID: "",
		},
	}
	if *approvalID != "" {
		id := domain.ApprovalID(*approvalID)
		pubIn.ApprovalID = &id
	}

	ctx := context.Background()
	result, err := svc.Publish(ctx, projectID, pubIn)
	if err != nil {
		return writeDomainError(stderr, err, "invalid_argument", `run "royo-learn publish --help"`, "publish: ")
	}

	// Dry run: report the plan without any write (D7).
	if result.DryRun {
		if *jsonFlag {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"dry_run":      true,
				"learning_id":  *learningID,
				"preview_hash": *previewHash,
				"targets":      result.Targets,
				"next_action":  "re-run with --apply to write the files",
			}, "", "  ")
			_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
		} else {
			_, _ = fmt.Fprintf(stdout, "Dry run: %d target(s) would change. Re-run with --apply to write.\n",
				len(result.Targets))
		}
		return exitSuccess
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"publication_id": result.Publication.ID,
			"learning_id":    result.Publication.LearningID,
			"status":         result.Publication.Status,
			"journal_id":     result.JournalID,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Published learning %s as %s (status: %s)\n",
			result.Publication.LearningID, result.Publication.ID, result.Publication.Status)
	}

	return exitSuccess
}

// ---------------------------------------------------------------------------
// rollback
// ---------------------------------------------------------------------------

func runRollback(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	journalID := fs.String("journal-id", "", "publication ID to rollback (required)")
	listRecoverable := fs.Bool("list", false, "list interrupted publications that require recovery")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writePublishError(stderr, "invalid_argument", "rollback: %v", err)
	}

	if *listRecoverable && *journalID != "" {
		return writePublishError(stderr, "invalid_argument", "rollback: --list cannot be combined with --journal-id")
	}
	if !*listRecoverable && *journalID == "" {
		return writePublishError(stderr, "invalid_argument", "rollback: --journal-id is required")
	}

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := publish.NewService(db, root,
		filepath.Join(root, ".royo-learn", "backups"),
		filepath.Join(root, ".royo-learn"),
		filepath.Join(root, ".royo-learn", "records"))

	ctx := context.Background()
	if *listRecoverable {
		candidates, err := svc.RecoverablePublications(ctx)
		if err != nil {
			return writeDomainError(stderr, err, "rollback_failed", `run "royo-learn doctor --json"`, "rollback: ")
		}
		if *jsonFlag {
			data, _ := json.MarshalIndent(candidates, "", "  ")
			_, _ = fmt.Fprintf(stdout, "%s\n", data)
		} else if len(candidates) == 0 {
			_, _ = fmt.Fprintln(stdout, "No interrupted publications require recovery.")
		} else {
			for _, candidate := range candidates {
				_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", candidate.PublicationID, candidate.Status, candidate.JournalStatus)
			}
		}
		return exitSuccess
	}
	err := svc.Rollback(ctx, projectID, &publish.RollbackPublicationInput{
		PublicationID: domain.PublicationID(*journalID),
		Actor: domain.Actor{
			Kind: "human", Name: "cli-user", Model: "", SessionID: "",
		},
	})
	if err != nil {
		return writeDomainError(stderr, err, "rollback_failed", `run "royo-learn rollback --help"`, "rollback: ")
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"publication_id": *journalID,
			"status":         "rolled_back",
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Rolled back publication %s\n", *journalID)
	}

	return exitSuccess
}

// resolvePublishContext resolves project root, opens DB, runs migrations,
// and ensures the project exists. Returns (root, db, projectID, exitCode).
func resolvePublishContext(explicitRoot string, stderr io.Writer) (string, *storage.DB, domain.ProjectID, int) {
	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	req := &project.ResolveRequest{
		CWD:          cwd,
		ExplicitRoot: explicitRoot,
	}

	proj, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		return "", nil, "", mapProjectError(stderr, err)
	}

	root := proj.Root

	markerPath := filepath.Join(root, ".royo-learn", "config.yaml")
	if _, statErr := os.Stat(markerPath); statErr != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    project.ErrProjectNotFound,
			Message: fmt.Sprintf("no project marker found at %s", root),
		})
		return "", nil, "", exitProjectNotFound
	}

	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		writePublishError(stderr, "invalid_argument", "cannot open database: %v", err)
		return "", nil, "", exitFailure
	}

	if err := storage.Migrate(db); err != nil {
		db.Close()
		writePublishError(stderr, "invalid_argument", "migration failed: %v", err)
		return "", nil, "", exitFailure
	}

	ctx := context.Background()
	tx, txErr := db.DB.BeginTx(ctx, nil)
	if txErr != nil {
		db.Close()
		writePublishError(stderr, "invalid_argument", "begin tx: %v", txErr)
		return "", nil, "", exitFailure
	}

	var projectID domain.ProjectID
	if existing, _ := storage.GetProjectByKey(ctx, tx, proj.Key); existing == nil {
		projEntity := &domain.Project{
			ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
			ProjectKey:    proj.Key,
			DisplayName:   proj.Key,
			CanonicalPath: root,
			GitRemote:     proj.GitRemote,
			Fingerprint:   proj.Key,
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}
		if err := storage.SaveProject(ctx, tx, projEntity); err != nil {
			tx.Rollback()
			db.Close()
			writePublishError(stderr, "invalid_argument", "save project: %v", err)
			return "", nil, "", exitFailure
		}
		projectID = projEntity.ID
	} else {
		projectID = existing.ID
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		writePublishError(stderr, "invalid_argument", "commit project: %v", err)
		return "", nil, "", exitFailure
	}

	return root, db, projectID, exitSuccess
}

func writePublishError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn preview --help"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// engram-health
// ---------------------------------------------------------------------------

func runEngramHealth(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("engram-health", flag.ContinueOnError)
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writeEngramError(stderr, "invalid_argument", "engram-health: %v", err)
	}

	// Resolve project to get Engram config.
	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	req := &project.ResolveRequest{CWD: cwd, ExplicitRoot: *projectRoot}
	proj, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		return mapProjectError(stderr, err)
	}

	cfg, err := config.Load(proj.Root)
	if err != nil {
		return writeEngramError(stderr, "invalid_argument", "engram-health: cannot load config: %v", err)
	}

	engramURL := cfg.Engram.BaseURL
	if engramURL == "" {
		engramURL = "http://localhost:8765"
	}
	if !cfg.Engram.Enabled {
		if *jsonFlag {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"status":  "disabled",
				"message": "engram integration is disabled in config",
			}, "", "  ")
			_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
		} else {
			_, _ = fmt.Fprintf(stdout, "Engram: disabled (set engram.enabled: true in config)\n")
		}
		return exitSuccess
	}

	client := engram.NewHTTPClient(engramURL)
	result, err := client.Health(context.Background())
	if err != nil {
		return writeEngramError(stderr, "invalid_argument", "engram-health: %v", err)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"status":  result.Status.String(),
			"message": result.Message,
			"url":     engramURL,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Engram: %s (%s)\n", result.Status.String(), result.Message)
	}
	return exitSuccess
}

// ---------------------------------------------------------------------------
// engram-search
// ---------------------------------------------------------------------------

func runEngramSearch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("engram-search", flag.ContinueOnError)
	query := fs.String("query", "", "search query (required)")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writeEngramError(stderr, "invalid_argument", "engram-search: %v", err)
	}
	if *query == "" {
		return writeEngramError(stderr, "invalid_argument", "engram-search: --query is required")
	}

	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	req := &project.ResolveRequest{CWD: cwd, ExplicitRoot: *projectRoot}
	proj, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		return mapProjectError(stderr, err)
	}

	cfg, err := config.Load(proj.Root)
	if err != nil {
		return writeEngramError(stderr, "invalid_argument", "engram-search: cannot load config: %v", err)
	}

	engramURL := cfg.Engram.BaseURL
	if engramURL == "" {
		engramURL = "http://localhost:8765"
	}
	if !cfg.Engram.Enabled {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    "engram_disabled",
			Message: "engram integration is disabled in config",
		})
		return exitFailure
	}

	client := engram.NewHTTPClient(engramURL)
	degraded := engram.NewDegradedClient(client)
	results, err := degraded.Search(context.Background(), *query)
	if err != nil {
		return writeEngramError(stderr, "invalid_argument", "engram-search: %v", err)
	}

	if *jsonFlag {
		if results == nil {
			results = []engram.SearchResult{}
		}
		data, _ := json.MarshalIndent(results, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		for _, r := range results {
			_, _ = fmt.Fprintf(stdout, "[%.2f] %s: %s\n", r.Score, r.Title, r.Content)
		}
		if len(results) == 0 {
			_, _ = fmt.Fprintf(stdout, "No results.\n")
		}
	}
	return exitSuccess
}

func writeEngramError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn engram-health"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// recurrences
// ---------------------------------------------------------------------------

func runRecurrences(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("recurrences", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning ID to list recurrences for (required)")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")
	limit := fs.Int("limit", 50, "max results")

	if err := fs.Parse(args); err != nil {
		return writeRecurrenceError(stderr, "invalid_argument", "recurrences: %v", err)
	}
	if *learningID == "" {
		return writeRecurrenceError(stderr, "invalid_argument", "recurrences: --learning-id is required")
	}

	_, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	records, err := recurrence.ListRecurrencesForLearning(ctx, db, domain.LearningID(*learningID), *limit)
	if err != nil {
		return writeRecurrenceError(stderr, "invalid_argument", "recurrences: %v", err)
	}

	if *jsonFlag {
		items := make([]map[string]any, 0, len(records))
		for _, r := range records {
			items = append(items, map[string]any{
				"id":                     string(r.ID),
				"recurrence_fingerprint": r.RecurrenceFingerprint,
				"learning_id":            string(r.LearningID),
				"summary":                r.Summary,
				"occurred_at":            r.OccurredAt.Format(time.RFC3339),
			})
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Recurrences for learning %s:\n", *learningID)
		for _, r := range records {
			_, _ = fmt.Fprintf(stdout, "  %s: %s (%s)\n",
				r.ID, r.Summary, r.OccurredAt.Format(time.RFC3339))
		}
		if len(records) == 0 {
			_, _ = fmt.Fprintf(stdout, "  (none)\n")
		}
	}

	return exitSuccess
}

// ---------------------------------------------------------------------------
// metrics
// ---------------------------------------------------------------------------

func runMetrics(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("metrics", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning ID to compute metrics for (required)")
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writeRecurrenceError(stderr, "invalid_argument", "metrics: %v", err)
	}
	if *learningID == "" {
		return writeRecurrenceError(stderr, "invalid_argument", "metrics: --learning-id is required")
	}

	root, db, _, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	ctx := context.Background()
	lid := domain.LearningID(*learningID)

	// Get the learning to compute its fingerprint.
	tx, txErr := db.DB.BeginTx(ctx, nil)
	if txErr != nil {
		return writeRecurrenceError(stderr, "invalid_argument", "metrics: begin tx: %v", txErr)
	}
	defer tx.Rollback()

	learning, err := storage.GetLearning(ctx, tx, lid)
	if err != nil || learning == nil {
		tx.Rollback()
		return writeRecurrenceError(stderr, "invalid_argument", "metrics: learning %q not found", *learningID)
	}
	tx.Rollback()

	fp := recurrence.RecurrenceFingerprint(learning)

	// Resolve project.
	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	req := &project.ResolveRequest{CWD: cwd, ExplicitRoot: root}
	proj, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		return mapProjectError(stderr, err)
	}

	projID, projErr := resolveProjectID(ctx, db, proj)
	if projErr != nil {
		return writeRecurrenceError(stderr, "invalid_argument", "metrics: %v", projErr)
	}

	metrics, err := recurrence.ComputeMetrics(ctx, db, projID, fp)
	if err != nil {
		return writeRecurrenceError(stderr, "invalid_argument", "metrics: %v", err)
	}

	status, _ := recurrence.CheckNeedsReview(ctx, db, projID, learning)
	metrics.NeedsReview = status.NeedsReview

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]any{
			"fingerprint":  metrics.Fingerprint,
			"count":        metrics.Count,
			"first_seen":   metrics.FirstSeen.Format(time.RFC3339),
			"last_seen":    metrics.LastSeen.Format(time.RFC3339),
			"avg_interval": metrics.AvgInterval.String(),
			"trend":        string(metrics.Trend),
			"needs_review": metrics.NeedsReview,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Metrics for learning %s:\n", *learningID)
		_, _ = fmt.Fprintf(stdout, "  Count: %d\n", metrics.Count)
		_, _ = fmt.Fprintf(stdout, "  First seen: %s\n", metrics.FirstSeen.Format(time.RFC3339))
		_, _ = fmt.Fprintf(stdout, "  Last seen: %s\n", metrics.LastSeen.Format(time.RFC3339))
		_, _ = fmt.Fprintf(stdout, "  Avg interval: %s\n", metrics.AvgInterval.String())
		_, _ = fmt.Fprintf(stdout, "  Trend: %s\n", metrics.Trend)
		_, _ = fmt.Fprintf(stdout, "  Needs review: %v\n", metrics.NeedsReview)
	}

	return exitSuccess
}

func resolveProjectID(ctx context.Context, db *storage.DB, proj *project.Project) (domain.ProjectID, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	existing, err := storage.GetProjectByKey(ctx, tx, proj.Key)
	if err != nil {
		return "", err
	}
	if existing != nil {
		tx.Rollback()
		return existing.ID, nil
	}
	tx.Rollback()
	return "", fmt.Errorf("project not registered")
}

func writeRecurrenceError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn recurrences --learning-id <id>"`,
	})
	return domain.ErrorCode(code).ExitCode()
}

// ---------------------------------------------------------------------------
// self-update
// ---------------------------------------------------------------------------

func runSelfUpdate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	checkOnly := fs.Bool("check", false, "report available update without downloading")
	versionFlag := fs.String("version", "", "install a specific version")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writeSelfUpdateError(stderr, "invalid_argument", "self-update: %v", err)
	}

	if *checkOnly && *versionFlag != "" {
		return writeSelfUpdateError(stderr, "invalid_argument", "self-update: --check cannot be combined with --version")
	}

	execPath, err := os.Executable()
	if err != nil {
		return writeSelfUpdateError(stderr, "invalid_argument", "self-update: cannot determine executable path: %v", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return writeSelfUpdateError(stderr, "invalid_argument", "self-update: cannot resolve executable path: %v", err)
	}

	u, err := selfupdate.New(selfupdate.Config{
		CurrentVersion: buildinfo.Version,
		ExecutablePath: execPath,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
	})
	if err != nil {
		return writeSelfUpdateError(stderr, "invalid_argument", "self-update: %v", err)
	}

	ctx := context.Background()

	// --check mode: only report availability.
	if *checkOnly {
		check, err := u.Check(ctx)
		if err != nil {
			return writeSelfUpdateError(stderr, "self_update_failed", "self-update: %v", err)
		}
		if *jsonFlag {
			data, _ := json.MarshalIndent(check, "", "  ")
			_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
		} else {
			_, _ = fmt.Fprintf(stdout, "Current version: %s\n", check.CurrentVersion)
			_, _ = fmt.Fprintf(stdout, "Latest version:  %s\n", check.LatestVersion)
			if check.UpdateAvailable {
				_, _ = fmt.Fprintf(stdout, "Update available.\n")
			} else {
				_, _ = fmt.Fprintf(stdout, "Already up to date.\n")
			}
		}
		return exitSuccess
	}

	// Full update.
	result, err := u.Update(ctx, *versionFlag)
	if err != nil {
		if errors.Is(err, selfupdate.ErrDevBuild) {
			return writeSelfUpdateError(stderr, "development_build", "%s", err.Error())
		}
		return writeSelfUpdateError(stderr, "self_update_failed", "self-update: %v", err)
	}

	if !result.Updated {
		if *jsonFlag {
			data, _ := json.MarshalIndent(result, "", "  ")
			_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
		} else {
			_, _ = fmt.Fprintf(stdout, "Already up to date (%s).\n", result.NewVersion)
		}
		return exitSuccess
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(result, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		_, _ = fmt.Fprintf(stdout, "Updated to %s\n", result.NewVersion)
	}
	return exitSuccess
}

func writeSelfUpdateError(stderr io.Writer, code, format string, args ...interface{}) int {
	msg := fmt.Sprintf(format, args...)
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        code,
		Message:     msg,
		Recoverable: true,
		Details:     map[string]any{},
		NextAction:  `run "royo-learn self-update --check" or use the installer`,
	})
	return exitFailure
}
