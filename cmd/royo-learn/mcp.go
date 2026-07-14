package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/logging"
	"agent-royo-learn/internal/mcpserver"
	"agent-royo-learn/internal/project"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// canonicalToolProfiles are the profiles the contract defines (D2).
var canonicalToolProfiles = map[string]string{
	"read":  "read",
	"agent": "agent",
	"admin": "admin",
}

// deprecatedToolProfiles are the v0.1.9 profile values, kept working and mapped
// onto the canonical profiles (D2, D8).
var deprecatedToolProfiles = map[string]string{
	"minimal":  "read",
	"standard": "agent",
	"full":     "admin",
}

// mcpFlagSet parses the flags of the MCP server command. It exists as a distinct
// type so that profile resolution — the part carrying the compatibility contract
// — is testable without starting a server.
type mcpFlagSet struct {
	fs          *flag.FlagSet
	tools       *string
	profile     *string
	projectRoot *string
}

func newMCPFlagSet() *mcpFlagSet {
	fs := flag.NewFlagSet("mcp-serve", flag.ContinueOnError)
	return &mcpFlagSet{
		fs:          fs,
		tools:       fs.String("tools", "", "tool profile: read, agent, admin (default: agent)"),
		profile:     fs.String("profile", "", "DEPRECATED alias of --tools; removed in v0.2.0"),
		projectRoot: fs.String("project-root", "", "explicit project root (optional; overrides CWD)"),
	}
}

func (m *mcpFlagSet) parse(args []string) error {
	return m.fs.Parse(args)
}

// resolveProfile returns the canonical tool profile requested by the flags,
// together with the deprecation warnings the caller must surface. Deprecation is
// never silent (D8).
func (m *mcpFlagSet) resolveProfile() (profile string, warnings []string, err error) {
	toolsValue, profileValue := *m.tools, *m.profile

	if toolsValue != "" && profileValue != "" {
		return "", nil, fmt.Errorf("--tools and --profile are mutually exclusive; use --tools")
	}

	value := toolsValue
	if profileValue != "" {
		value = profileValue
		warnings = append(warnings,
			"--profile is deprecated and will be removed in v0.2.0; use --tools read|agent|admin")
	}

	if value == "" {
		return "agent", warnings, nil
	}

	if canonical, ok := canonicalToolProfiles[value]; ok {
		return canonical, warnings, nil
	}
	if canonical, ok := deprecatedToolProfiles[value]; ok {
		warnings = append(warnings, fmt.Sprintf(
			"tool profile %q is deprecated and will be removed in v0.2.0; use %q", value, canonical))
		return canonical, warnings, nil
	}

	return "", nil, fmt.Errorf("unknown tool profile %q: expected read, agent or admin", value)
}

// runMCPServe starts the royo-learn MCP server over stdio.
func runMCPServe(args []string, _ io.Writer, stderr io.Writer) int {
	fs := newMCPFlagSet()
	if err := fs.parse(args); err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:        "invalid_argument",
			Message:     fmt.Sprintf("mcp-serve: %v", err),
			Recoverable: true,
			Details:     map[string]any{},
			NextAction:  `run "royo-learn mcp-serve [--tools read|agent|admin]"`,
		})
		return exitInvalidArguments
	}

	resolvedProfile, warnings, err := fs.resolveProfile()
	if err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:        "invalid_argument",
			Message:     fmt.Sprintf("mcp-serve: %v", err),
			Recoverable: true,
			Details:     map[string]any{},
			NextAction:  `run "royo-learn mcp-serve [--tools read|agent|admin]"`,
		})
		return exitInvalidArguments
	}
	// Deprecation warnings go to stderr so they never contaminate stdout, which
	// carries MCP JSON-RPC exclusively.
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(stderr, "royo-learn: warning: %s\n", warning)
	}

	profile := &resolvedProfile
	projectRoot := fs.projectRoot

	root := *projectRoot
	if root == "" {
		root = os.Getenv("ROYO_LEARN_PROJECT_ROOT")
	}

	// Resolve project root from CWD or explicit root.
	cwd, _ := os.Getwd()
	resolver := project.NewResolver()
	proj, err := resolver.Resolve(context.Background(), &project.ResolveRequest{
		CWD:          cwd,
		ExplicitRoot: root,
	})
	if err != nil {
		return mapProjectError(stderr, err)
	}

	root = proj.Root

	// Verify project marker.
	markerPath := filepath.Join(root, ".royo-learn", "config.yaml")
	if _, statErr := os.Stat(markerPath); statErr != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    project.ErrProjectNotFound,
			Message: fmt.Sprintf("no project marker found at %s. Run 'royo-learn init --project-root %s' first.", root, root),
		})
		return exitProjectNotFound
	}

	// Open database.
	dbPath := filepath.Join(root, ".royo-learn", "royo-learn.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("mcp-serve: cannot open database: %v", err),
		})
		return exitFailure
	}
	defer db.Close()

	// Run migrations.
	if err := storage.Migrate(db); err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("mcp-serve: migration failed: %v", err),
		})
		return exitFailure
	}

	// Ensure project exists in database.
	ctx := context.Background()
	tx, txErr := db.DB.BeginTx(ctx, nil)
	if txErr != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("mcp-serve: begin tx: %v", txErr),
		})
		return exitFailure
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
			_ = logging.WriteError(stderr, logging.ErrorEnvelope{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("mcp-serve: save project: %v", err),
			})
			return exitFailure
		}
		projectID = projEntity.ID
	} else {
		projectID = existing.ID
	}
	if err := tx.Commit(); err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("mcp-serve: commit project: %v", err),
		})
		return exitFailure
	}

	recordsDir := filepath.Join(root, ".royo-learn", "records")

	// Create server.
	cfg := mcpserver.Config{
		Profile:    *profile,
		DBPath:     dbPath,
		RecordsDir: recordsDir,
	}

	srv, err := mcpserver.NewServer(cfg, db, projectID, root)
	if err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("mcp-serve: create server: %v", err),
		})
		return exitFailure
	}

	// Signal handling for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		srv.Close()
		os.Exit(exitSuccess)
	}()

	// Write startup message to stderr (stdout is reserved for MCP JSON-RPC).
	_, _ = fmt.Fprintf(stderr, "royo-learn MCP server starting (profile: %s, project: %s)\n",
		*profile, root)

	// Run the server on stdio transport.
	// stdout is exclusively MCP JSON-RPC; all logging goes to stderr.
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("mcp-serve: server error: %v", err),
		})
		return exitFailure
	}

	return exitSuccess
}
