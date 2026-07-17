// Package mcpserver implements the royo-learn MCP server over stdio.
package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"agent-royo-learn/internal/buildinfo"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds configuration for the MCP server.
type Config struct {
	// Profile selects which tool set to expose: read, agent, admin (D2).
	// The v0.1.9 names minimal, standard and full are accepted as deprecated
	// aliases and mapped onto read, agent and admin respectively.
	// Default: "agent".
	Profile string

	// DBPath is the path to the SQLite database file (for diagnostic logging).
	DBPath string

	// RecordsDir is the directory where Markdown records are written.
	RecordsDir string

	// MaxRequestBytes is the maximum size of a single JSON-RPC request.
	// Default: 1MB.
	MaxRequestBytes int64

	// AllowedCommands is the allowlist for command evidence collection. A nil
	// or empty list permits only git, which is the CommandRunner default.
	AllowedCommands []string
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
	// Resolve the profile to its canonical form, accepting deprecated names.
	canonical, _, ok := resolveProfile(cfg.Profile)
	if !ok {
		canonical = defaultProfile
	}
	cfg.Profile = canonical

	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = 1 << 20 // 1MB
	}

	// Build instructions.
	instructions := buildInstructions(cfg.Profile)

	// Create service wrappers.
	capSvc, err := newCaptureSvc(db, cfg.RecordsDir, projectRoot, cfg.AllowedCommands)
	if err != nil {
		return nil, fmt.Errorf("mcpserver: init capture service: %w", err)
	}
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

	// Register tools for the profile. Each entry binds its canonical name and
	// its deprecated aliases to the same handler.
	for _, tool := range profileTools(cfg.Profile) {
		tool.register(mcpServer, srv, tool)
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

// buildInstructions creates the MCP server instructions text.
//
// D14: the tool list is DERIVED from the registry of the active profile. It is
// never written by hand. A tool that is not registered in this profile cannot
// appear here, and a tool that is registered cannot be omitted. Deprecated
// aliases keep working but are not advertised: advertising them would perpetuate
// their use.
func buildInstructions(profile string) string {
	meta := buildinfo.Current()

	canonical, _, ok := resolveProfile(profile)
	if !ok {
		canonical = defaultProfile
	}

	var toolList strings.Builder
	for _, tool := range profileTools(canonical) {
		fmt.Fprintf(&toolList, "\n- %s: %s", tool.name, tool.description)
	}

	return fmt.Sprintf(
		`royo-learn MCP Server v%s
Profile: %s
Schema version: %d

Prerequisite: each project must be initialized once before these tools work.
If a call returns project_not_found, the project has no store yet. Run
'royo-learn init --project-root <root>' first to create it. After initialization,
optionally run 'royo-learn setup install' to register the MCP server and install
the skills. The store lives at <root>/.royo-learn/ and is discovered by walking
up from the working directory, so ONE init per project root covers every
subfolder — not one per folder.

The tools below are exactly the tools this profile serves.
%s

All tool outputs are structured JSON. Errors are returned in the response content with IsError=true.`,
		meta.Version, canonical, meta.SchemaVersion, toolList.String(),
	)
}
