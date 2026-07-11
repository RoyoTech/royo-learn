// Package mcpserver implements the royo-learn MCP server over stdio.
package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"agent-royo-learn/internal/buildinfo"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds configuration for the MCP server.
type Config struct {
	// Profile selects which tool set to expose: minimal, standard, full.
	// Default: "standard".
	Profile string

	// DBPath is the path to the SQLite database file (for diagnostic logging).
	DBPath string

	// RecordsDir is the directory where Markdown records are written.
	RecordsDir string

	// MaxRequestBytes is the maximum size of a single JSON-RPC request.
	// Default: 1MB.
	MaxRequestBytes int64
}

// Server wraps the MCP server with royo-learn business logic.
type Server struct {
	mcpServer    *mcp.Server
	db           *storage.DB
	projectID    domain.ProjectID
	projectRoot  string
	cfg          Config
	instructions string
	capSvc       *captureSvc
	curateSvc    *curateSvc
	publishSvc   *publishSvc
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg Config, db *storage.DB, projectID domain.ProjectID, projectRoot string) (*Server, error) {
	// Validate and default profile.
	if !validProfile(cfg.Profile) {
		cfg.Profile = "standard"
	}
	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = 1 << 20 // 1MB
	}

	// Build instructions.
	instructions := buildInstructions(cfg.Profile)

	// Create service wrappers.
	capSvc := newCaptureSvc(db, cfg.RecordsDir)
	curateSvc := newCurateSvc(db, cfg.RecordsDir)
	publishSvc := newPublishSvc(db, projectRoot, projectRoot)

	srv := &Server{
		db:           db,
		projectID:    projectID,
		projectRoot:  projectRoot,
		cfg:          cfg,
		instructions: instructions,
		capSvc:       capSvc,
		curateSvc:    curateSvc,
		publishSvc:   publishSvc,
	}

	// Discard logger: stderr is for diagnostics, stdout is MCP only.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	meta := buildinfo.Current()
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "royo-learn",
		Title:   "royo-learn MCP Server",
		Version: meta.Version,
	}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       logger,
	})

	srv.mcpServer = mcpServer

	// Register tools for the profile.
	for _, tool := range profileTools(cfg.Profile) {
		tool.register(mcpServer, srv)
	}

	// Register middleware.
	registerMiddleware(mcpServer, cfg.MaxRequestBytes)

	return srv, nil
}

// Run starts the MCP server over the given transport.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	return s.mcpServer.Run(ctx, transport)
}

// Instructions returns the server instructions sent to clients on initialization.
func (s *Server) Instructions() string {
	return s.instructions
}

// Close releases resources held by the server.
func (s *Server) Close() error {
	return s.db.Close()
}

// validProfile checks if the given profile name is recognized.
func validProfile(profile string) bool {
	switch profile {
	case "minimal", "standard", "full":
		return true
	}
	return false
}

// buildInstructions creates the MCP server instructions text.
func buildInstructions(profile string) string {
	meta := buildinfo.Current()
	return fmt.Sprintf(
		`royo-learn MCP Server v%s
Profile: %s
Schema version: %d

Use the listed tools to capture, search, curate, preview and publish learnings.

- capture_learning: capture a new learning or return an existing one
- search_learnings: full-text search across learnings
- list_learnings: list learnings with optional filters
- get_learning: retrieve a single learning by ID
- curate_learning: approve, reject, or relate a learning
- preview_publication: preview what would be published
- publish_learning: publish an approved learning
- doctor: health check and diagnostics

All tool outputs are structured JSON. Errors are returned in the response content with IsError=true.`,
		meta.Version, profile, meta.SchemaVersion,
	)
}
