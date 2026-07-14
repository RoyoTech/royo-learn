package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Profiles (D2)
//
// Canonical profiles are read / agent / admin, as specified by
// docs/04-CLI-SPEC.md. The v0.1.9 names minimal / standard / full keep working
// as deprecated aliases that map onto them.
// ---------------------------------------------------------------------------

const (
	profileRead  = "read"
	profileAgent = "agent"
	profileAdmin = "admin"

	// defaultProfile is the profile served when none is requested. It is the
	// canonical equivalent of the v0.1.9 default ("standard"), so no existing
	// installation changes behaviour.
	defaultProfile = profileAgent
)

// deprecatedProfiles maps every v0.1.9 profile name to its canonical profile.
var deprecatedProfiles = map[string]string{
	"minimal":  profileRead,
	"standard": profileAgent,
	"full":     profileAdmin,
}

// deprecationRemovedIn is the version that retires the deprecated names (D8).
const deprecationRemovedIn = "v0.2.0"

// resolveProfile maps a profile name, canonical or deprecated, onto its
// canonical form. It reports whether the input was deprecated and whether it was
// recognised at all.
func resolveProfile(name string) (canonical string, deprecated bool, ok bool) {
	switch name {
	case profileRead, profileAgent, profileAdmin:
		return name, false, true
	}
	if canonical, found := deprecatedProfiles[name]; found {
		return canonical, true, true
	}
	return "", false, false
}

// validCanonicalProfile reports whether name is one of the canonical profiles.
func validCanonicalProfile(name string) bool {
	switch name {
	case profileRead, profileAgent, profileAdmin:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Tool access annotations (D2)
// ---------------------------------------------------------------------------

// toolAccess classifies a tool by its effect on persistent state.
type toolAccess string

const (
	// accessRead does not modify any persistent state.
	accessRead toolAccess = "read"
	// accessWrite adds to persistent state but destroys nothing.
	accessWrite toolAccess = "write"
	// accessDestructive may overwrite or remove existing state. Confined to admin.
	accessDestructive toolAccess = "destructive"
)

// annotations renders the access class as MCP tool annotations.
func (a toolAccess) annotations() *mcp.ToolAnnotations {
	destructive := a == accessDestructive
	closedWorld := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    a == accessRead,
		DestructiveHint: &destructive,
		OpenWorldHint:   &closedWorld,
	}
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// profileTool describes one canonical MCP tool: the profiles that serve it, its
// access class, and the deprecated aliases that resolve to the same handler.
type profileTool struct {
	// name is the canonical tool name (D1). Always learning_*.
	name string
	// aliases are the deprecated v0.1.9 names bound to the SAME handler (D1).
	// They are callable but never advertised in the server instructions (D14).
	aliases []string
	// description is the text advertised to clients.
	description string
	// access drives the MCP annotations (D2).
	access toolAccess
	// profiles lists the canonical profiles that serve this tool.
	profiles map[string]bool
	// register binds the canonical name and every alias to one handler.
	register func(ms *mcp.Server, srv *Server, t profileTool)
}

func (t profileTool) enabled(profile string) bool {
	return t.profiles[profile]
}

// profileTools returns the tools served by a profile, in registry order.
// The profile name may be canonical or deprecated.
func profileTools(profile string) []profileTool {
	canonical, _, ok := resolveProfile(profile)
	if !ok {
		canonical = defaultProfile
	}
	var out []profileTool
	for _, t := range allTools {
		if t.enabled(canonical) {
			out = append(out, t)
		}
	}
	return out
}

// bind registers the canonical name and every deprecated alias of t against a
// single handler. An alias is a name binding, never a copy of the logic: the
// alias path only decorates the result with the deprecation notice required by
// D8 before returning the canonical handler's own output.
func bind[In any](ms *mcp.Server, t profileTool, handler mcp.ToolHandlerFor[In, any]) {
	mcp.AddTool(ms, &mcp.Tool{
		Name:        t.name,
		Description: t.description,
		Annotations: t.access.annotations(),
	}, handler)

	for _, alias := range t.aliases {
		mcp.AddTool(ms, &mcp.Tool{
			Name: alias,
			Description: fmt.Sprintf("Deprecated alias of %s, removed in %s. %s",
				t.name, deprecationRemovedIn, t.description),
			Annotations: t.access.annotations(),
		}, withDeprecationNotice(alias, t.name, handler))
	}
}

// withDeprecationNotice wraps a canonical handler so that calls arriving through
// a deprecated alias carry a deprecation notice on the response (D8). The
// wrapped handler is the canonical one; no business logic is duplicated.
func withDeprecationNotice[In any](alias, canonical string, handler mcp.ToolHandlerFor[In, any]) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		result, out, err := handler(ctx, req, in)
		if result != nil {
			if result.Meta == nil {
				result.Meta = mcp.Meta{}
			}
			result.Meta["deprecation"] = map[string]any{
				"alias":      alias,
				"canonical":  canonical,
				"removed_in": deprecationRemovedIn,
			}
		}
		return result, out, err
	}
}

// ---------------------------------------------------------------------------
// allTools — the single source of truth for MCP tools, profiles and aliases.
//
// The server instructions (D14) and every contract test are derived from this
// table. Nothing about the tool surface is written by hand anywhere else.
//
// learning_publish is confined to admin. D2 places it in agent, but the binding
// clause of D2 ("D2 y D11 entran en la misma versión") forbids that move until
// the destination-based approval policies and learning_approve exist, which is
// Recorrido C. Until then it stays where v0.1.9 put it.
// ---------------------------------------------------------------------------

var allTools = []profileTool{
	{
		name:        "learning_capture",
		aliases:     []string{"capture_learning"},
		description: "Capture a new learning or return an existing one by content hash. Deduplication is automatic.",
		access:      accessWrite,
		profiles:    map[string]bool{profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleCaptureLearning(srv))
		},
	},
	{
		name:        "learning_search",
		aliases:     []string{"search_learnings"},
		description: "Search learnings by full-text query. Returns ranked results from the FTS5 index.",
		access:      accessRead,
		profiles:    map[string]bool{profileRead: true, profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleSearchLearnings(srv))
		},
	},
	{
		name:        "learning_get",
		aliases:     []string{"get_learning"},
		description: "Retrieve a single learning by its unique identifier.",
		access:      accessRead,
		profiles:    map[string]bool{profileRead: true, profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleGetLearning(srv))
		},
	},
	{
		name:        "learning_list",
		aliases:     []string{"list_learnings"},
		description: "List learnings for the current project. Filter by status, type, or scope.",
		access:      accessRead,
		profiles:    map[string]bool{profileRead: true, profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleListLearnings(srv))
		},
	},
	{
		name:        "learning_doctor",
		aliases:     []string{"doctor"},
		description: "Run health checks on the server: database connectivity, project resolution, version info.",
		access:      accessRead,
		profiles:    map[string]bool{profileRead: true, profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleDoctor(srv))
		},
	},
	{
		name:        "learning_curate",
		aliases:     []string{"curate_learning"},
		description: "Curate a learning: approve, reject, or request more evidence. Approval enforces evidence thresholds.",
		access:      accessWrite,
		profiles:    map[string]bool{profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleCurateLearning(srv))
		},
	},
	{
		name:        "learning_publication_preview",
		aliases:     []string{"preview_publication"},
		description: "Generate a publication preview showing what files would be created or modified. Persists the preview and returns its hash.",
		access:      accessWrite,
		profiles:    map[string]bool{profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handlePreviewPublication(srv))
		},
	},
	{
		name:        "learning_list_recurrences",
		aliases:     []string{"list_recurrences"},
		description: "List recurrence records for a learning, tracking when the same pattern appears across captures.",
		access:      accessRead,
		profiles:    map[string]bool{profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleListRecurrences(srv))
		},
	},
	{
		name:        "learning_compute_metrics",
		aliases:     []string{"compute_metrics"},
		description: "Compute recurrence metrics (frequency, interval, trend) for a learning's recurrence pattern.",
		access:      accessRead,
		profiles:    map[string]bool{profileAgent: true, profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handleComputeMetrics(srv))
		},
	},
	{
		name:        "learning_publish",
		aliases:     []string{"publish_learning"},
		description: "Publish an approved learning. Requires a preview hash to confirm the intended publication.",
		access:      accessWrite,
		profiles:    map[string]bool{profileAdmin: true},
		register: func(ms *mcp.Server, srv *Server, t profileTool) {
			bind(ms, t, handlePublishLearning(srv))
		},
	},
}
