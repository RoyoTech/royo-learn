package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// allTools defines every MCP tool, with the profiles in which each is enabled.
var allTools = []profileTool{
	{
		name:        "capture_learning",
		description: "Capture a new learning or return an existing one by content hash.",
		profiles:    map[string]bool{"minimal": true, "standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "capture_learning",
				Description: "Capture a new learning or return an existing one by content hash. Deduplication is automatic.",
			}, handleCaptureLearning(srv))
		},
	},
	{
		name:        "search_learnings",
		description: "Full-text search across learnings using FTS5.",
		profiles:    map[string]bool{"minimal": true, "standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "search_learnings",
				Description: "Search learnings by full-text query. Returns ranked results from FTS5 index.",
			}, handleSearchLearnings(srv))
		},
	},
	{
		name:        "curate_learning",
		description: "Approve, reject, or relate a learning.",
		profiles:    map[string]bool{"standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "curate_learning",
				Description: "Curate a learning: approve, reject, or request more evidence. Approval enforces evidence thresholds.",
			}, handleCurateLearning(srv))
		},
	},
	{
		name:        "preview_publication",
		description: "Preview what would be published for a learning.",
		profiles:    map[string]bool{"standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "preview_publication",
				Description: "Generate a publication preview showing what files would be created or modified.",
			}, handlePreviewPublication(srv))
		},
	},
	{
		name:        "publish_learning",
		description: "Publish an approved learning to its target destination.",
		profiles:    map[string]bool{"full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "publish_learning",
				Description: "Publish an approved learning. Requires a preview hash to confirm the intended publication.",
			}, handlePublishLearning(srv))
		},
	},
	{
		name:        "list_learnings",
		description: "List learnings with optional filters.",
		profiles:    map[string]bool{"standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "list_learnings",
				Description: "List learnings for the current project. Filter by status, type, or scope.",
			}, handleListLearnings(srv))
		},
	},
	{
		name:        "get_learning",
		description: "Get a single learning by ID.",
		profiles:    map[string]bool{"standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "get_learning",
				Description: "Retrieve a single learning by its unique identifier.",
			}, handleGetLearning(srv))
		},
	},
	{
		name:        "doctor",
		description: "Health check and diagnostics.",
		profiles:    map[string]bool{"minimal": true, "standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "doctor",
				Description: "Run health checks on the server: database connectivity, project resolution, version info.",
			}, handleDoctor(srv))
		},
	},
	{
		name:        "list_recurrences",
		description: "List recurrence records for a learning.",
		profiles:    map[string]bool{"standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "list_recurrences",
				Description: "List recurrence records for a learning, tracking when the same pattern appears across captures.",
			}, handleListRecurrences(srv))
		},
	},
	{
		name:        "compute_metrics",
		description: "Compute recurrence metrics for a learning.",
		profiles:    map[string]bool{"standard": true, "full": true},
		register: func(ms *mcp.Server, srv *Server) {
			mcp.AddTool(ms, &mcp.Tool{
				Name:        "compute_metrics",
				Description: "Compute recurrence metrics (frequency, interval, trend) for a learning's recurrence pattern.",
			}, handleComputeMetrics(srv))
		},
	},
}
