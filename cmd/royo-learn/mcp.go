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

// runMCPServe starts the royo-learn MCP server over stdio.
func runMCPServe(args []string, _ io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcp-serve", flag.ContinueOnError)
	profile := fs.String("profile", "standard", "tool profile: minimal, standard, full")
	projectRoot := fs.String("project-root", "", "explicit project root (optional; overrides CWD)")
	if err := fs.Parse(args); err != nil {
		_ = logging.WriteError(stderr, logging.ErrorEnvelope{
			Code:        "invalid_argument",
			Message:     fmt.Sprintf("mcp-serve: %v", err),
			Recoverable: true,
			Details:     map[string]any{},
			NextAction:  `run "royo-learn mcp-serve [--profile minimal|standard|full]"`,
		})
		return exitInvalidArguments
	}

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
