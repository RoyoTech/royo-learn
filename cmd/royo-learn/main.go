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
	"time"

	"agent-royo-learn/internal/buildinfo"
	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/config"
	"agent-royo-learn/internal/curate"
	"agent-royo-learn/internal/doctor"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/engram"
	"agent-royo-learn/internal/logging"
	"agent-royo-learn/internal/project"
	"agent-royo-learn/internal/publish"
	"agent-royo-learn/internal/recurrence"
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
	if len(args) == 0 {
		return writeUnknownCommandError(stderr)
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
	case "preview":
		return runPreview(args[1:], stdout, stderr)
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
	default:
		return writeUnknownCommandError(stderr)
	}
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
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:        "invalid_argument",
			Message:     invalidArgumentsMessage,
			Recoverable: true,
			Details:     map[string]any{},
			NextAction:  invalidArgumentsNextAction,
		})
		return exitInvalidArguments
	}

	return writeVersionJSON(stdout, stderr)
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
	return exitFailure
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
	return exitFailure
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
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

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
	svc := capture.NewService(db, recordsDir)

	input := &capture.CaptureInput{
		Title:       *title,
		Context:     *contextStr,
		Observation: *observation,
		Lesson:      *lesson,
		Type:        domain.LearningType(*learningType),
		Scope:       domain.Scope(*pScope),
		Actor: domain.Actor{
			Kind:      "human",
			Name:      "cli-user",
			Model:     "",
			SessionID: "",
		},
	}

	result, err := svc.Capture(ctx, projectID, input)
	if err != nil {
		return writeCaptureError(stderr, "invalid_argument", "capture: %v", err)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"learning_id": result.LearningID,
			"status":      result.Status,
			"new":         result.New,
		}, "", "  ")
		_, _ = fmt.Fprintf(stdout, "%s\n", string(data))
	} else {
		statusLabel := "new"
		if !result.New {
			statusLabel = "deduplicated (existing)"
		}
		_, _ = fmt.Fprintf(stdout, "captured %s learning %s (status: %s)\n", statusLabel, result.LearningID, result.Status)
	}

	return exitSuccess
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
	return exitFailure
}

// ---------------------------------------------------------------------------
// curate
// ---------------------------------------------------------------------------

func runCurate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("curate", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning ID to curate (required)")
	action := fs.String("action", "", "curation action: approve, reject, needs_evidence, relate (required)")
	targetID := fs.String("target-id", "", "target learning ID for relate action")
	relation := fs.String("relation", "related", "relation type for relate action")
	rationale := fs.String("rationale", "", "rationale for the curation decision")
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
	}

	result, curErr := svc.Curate(ctx, projectID, curateInput)
	if curErr != nil {
		return writeCurateError(stderr, "invalid_argument", "curate: %v", curErr)
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

// parseCurateAction maps a CLI action string to a CurationDecision.
func parseCurateAction(action string) (domain.CurationDecision, error) {
	switch action {
	case "approve":
		return domain.CurationApproveProjectKnowledge, nil
	case "reject":
		return domain.CurationReject, nil
	case "needs_evidence":
		return domain.CurationNeedsEvidence, nil
	default:
		return "", fmt.Errorf("unknown action %q: must be one of approve, reject, needs_evidence, relate", action)
	}
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
	return exitFailure
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
		filepath.Join(root, ".royo-learn"))

	ctx := context.Background()
	result, err := svc.Preview(ctx, projectID, &publish.PreviewInput{
		LearningID: domain.LearningID(*learningID),
		Actor: domain.Actor{
			Kind: "human", Name: "cli-user", Model: "", SessionID: "",
		},
	})
	if err != nil {
		return writePublishError(stderr, "invalid_argument", "preview: %v", err)
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
// publish
// ---------------------------------------------------------------------------

func runPublish(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	learningID := fs.String("learning-id", "", "learning ID to publish (required)")
	previewHash := fs.String("preview-hash", "", "preview hash to confirm (required)")
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

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := publish.NewService(db, root,
		filepath.Join(root, ".royo-learn", "backups"),
		filepath.Join(root, ".royo-learn"))

	ctx := context.Background()
	result, err := svc.Publish(ctx, projectID, &publish.PublishInput{
		LearningID:  domain.LearningID(*learningID),
		PreviewHash: *previewHash,
		Force:       *force,
		Actor: domain.Actor{
			Kind: "human", Name: "cli-user", Model: "", SessionID: "",
		},
	})
	if err != nil {
		return writePublishError(stderr, "invalid_argument", "publish: %v", err)
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
	projectRoot := fs.String("project-root", "", "project root directory")
	jsonFlag := fs.Bool("json", false, "emit stable JSON to stdout")

	if err := fs.Parse(args); err != nil {
		return writePublishError(stderr, "invalid_argument", "rollback: %v", err)
	}

	if *journalID == "" {
		return writePublishError(stderr, "invalid_argument", "rollback: --journal-id is required")
	}

	root, db, projectID, exitCode := resolvePublishContext(*projectRoot, stderr)
	if exitCode != exitSuccess {
		return exitCode
	}
	defer db.Close()

	svc := publish.NewService(db, root,
		filepath.Join(root, ".royo-learn", "backups"),
		filepath.Join(root, ".royo-learn"))

	ctx := context.Background()
	err := svc.Rollback(ctx, projectID, &publish.RollbackPublicationInput{
		PublicationID: domain.PublicationID(*journalID),
		Actor: domain.Actor{
			Kind: "human", Name: "cli-user", Model: "", SessionID: "",
		},
	})
	if err != nil {
		return writePublishError(stderr, "invalid_argument", "rollback: %v", err)
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
	return exitFailure
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
	return exitFailure
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
	return exitFailure
}
