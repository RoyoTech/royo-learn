package main

import (
	"fmt"
	"io"

	"agent-royo-learn/internal/logging"
)

// ---------------------------------------------------------------------------
// Single declarative command registry (Tramo 4 §4.1).
//
// Both printHelp and the dispatcher in run() derive from this one table. There
// is no second list of commands anywhere: the contract test in
// commands_test.go binds this registry to docs/04-CLI-SPEC.md and to the help
// output so none of the three can drift.
//
// A command is one of three kinds:
//   - implemented: run != nil, deprecated == "", pending == "" — shown in help.
//   - deprecated alias (D8/D9/D10): deprecated names the canonical command; it
//     still runs but warns on stderr and is hidden from help.
//   - pending (Hito 2 §4.6): documented in docs/04 but not yet built; pending
//     names the recorrido that will land it; run == nil; hidden from help.
// ---------------------------------------------------------------------------

type command struct {
	name       string
	summary    string
	run        func(args []string, stdout, stderr io.Writer) int
	deprecated string // canonical command this is a deprecated alias of
	pending    string // recorrido that will implement this documented command
}

// commandRegistry is the authoritative list of every CLI command. Order here is
// the order shown in help. It is populated in init() rather than in the var
// initializer because the run functions it holds transitively reference the
// registry (run -> lookupCommand), which the compiler would flag as an
// initialization cycle.
var commandRegistry []command

func init() {
	commandRegistry = []command{
		{name: "version", summary: "Print version information", run: runVersion},
		{name: "init", summary: "Initialize a new royo-learn project", run: runInit},
		{name: "doctor", summary: "Run system diagnostics", run: runDoctor},
		{name: "capture", summary: "Capture a new learning", run: runCapture},
		{name: "evidence", summary: "Attach or list evidence for a learning", run: runEvidence},
		{name: "get", summary: "Retrieve a single learning by ID", run: runGet},
		{name: "list", summary: "List learnings with optional filters", run: runList},
		{name: "search", summary: "Search captured learnings", run: runSearch},
		{name: "curate", summary: "Curate an existing learning", run: runCurate},
		{name: "preview", summary: "Preview publication of a learning", run: runPreview},
		{name: "approve", summary: "Approve a publication preview (human authorization)", run: runApprove},
		{name: "publish", summary: "Publish a curated learning", run: runPublish},
		{name: "rollback", summary: "Rollback a published learning", run: runRollback},
		{name: "occurrence", summary: "Record a recurrence of a learning's pattern", run: runOccurrence},
		{name: "recurrences", summary: "List recurrence records for a learning", run: runRecurrences},
		{name: "metrics", summary: "Show recurrence metrics for a learning", run: runMetrics},
		{name: "status", summary: "Report the lifecycle status of a learning", run: runStatus},
		{name: "review", summary: "List candidates, needs-evidence, approved-not-published and recurrences", run: runReview},
		{name: "export", summary: "Export a versioned, portable snapshot of the store", run: runExport},
		{name: "import", summary: "Validate and import a bundle (dry-run by default)", run: runImport},
		{name: "rebuild-index", summary: "Rebuild the search index and re-materialize records from SQLite", run: runRebuildIndex},
		{name: "mcp", summary: "Start the MCP server over stdio", run: runMCPServe},
		{name: "e2e", summary: "Run the end-to-end demonstration", run: runE2E},
		{name: "setup", summary: "Configure the tool for first use", run: runSetup},
		{name: "self-update", summary: "Update to the latest or a specific version", run: runSelfUpdate},

		// Deprecated aliases (D9/D10). They keep working and warn on stderr.
		{name: "mcp-serve", summary: "Deprecated alias of mcp", run: runMCPServe, deprecated: "mcp"},
		{name: "engram-health", summary: "Deprecated: folded under doctor", run: runEngramHealth, deprecated: "doctor"},
		{name: "engram-search", summary: "Deprecated: folded under search --include-engram", run: runEngramSearch, deprecated: "search"},
	}
}

// lookupCommand finds a command by name.
func lookupCommand(name string) (command, bool) {
	for _, c := range commandRegistry {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// visibleCommands returns the commands advertised in help: implemented,
// non-deprecated, non-pending.
func visibleCommands() []command {
	var out []command
	for _, c := range commandRegistry {
		if c.run != nil && c.deprecated == "" && c.pending == "" {
			out = append(out, c)
		}
	}
	return out
}

// writePendingCommandError reports that a documented command is not built yet.
func writePendingCommandError(stderr io.Writer, c command) int {
	_ = logging.WriteError(stderr, logging.ErrorEnvelope{
		Code:        "not_implemented",
		Message:     fmt.Sprintf("command %q is documented but not implemented until %s", c.name, c.pending),
		Recoverable: false,
		Details:     map[string]any{"command": c.name, "arrives_in": c.pending},
		NextAction:  `run "royo-learn --help" to see the available commands`,
	})
	return exitFailure
}

// printCommandHelp prints one command's summary. It is the central handler for
// "<command> --help", so every registered command accepts --help uniformly.
func printCommandHelp(stdout io.Writer, c command) int {
	switch {
	case c.pending != "":
		_, _ = fmt.Fprintf(stdout, "royo-learn %s — documented in docs/04-CLI-SPEC.md; not implemented until %s.\n",
			c.name, c.pending)
	case c.deprecated != "":
		_, _ = fmt.Fprintf(stdout, "royo-learn %s — %s. Use \"royo-learn %s\" instead.\n",
			c.name, c.summary, c.deprecated)
	default:
		_, _ = fmt.Fprintf(stdout, "royo-learn %s — %s.\nRun \"royo-learn %s\" with its flags; pass no flags to see the required ones.\n",
			c.name, c.summary, c.name)
	}
	return exitSuccess
}
