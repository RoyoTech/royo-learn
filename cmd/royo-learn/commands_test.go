package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// ---------------------------------------------------------------------------
// Permanent CLI command contract test (Tramo 4 §4.1 of
// docs/PLAN-recuperacion-contrato.md).
//
// Both --help and the dispatcher derive from a single declarative registry
// (commandRegistry). This test binds three surfaces together so none of them
// can drift again:
//
//	docs/04-CLI-SPEC.md   (documented commands)
//	commandRegistry       (implemented / deprecated / pending commands)
//	printHelp output      (advertised commands)
//
// It asserts the five conditions §4.1 requires:
//  1. every command in --help exists and executes (has a run func);
//  2. every implemented command appears in help;
//  3. every command accepts --help;
//  4. every command documented in docs/04 is implemented (or explicitly pending);
//  5. no phantom commands (advertised but absent) and no ghost commands
//     (implemented but hidden from help).
// ---------------------------------------------------------------------------

// docsCommandHeading matches a command section heading in docs/04-CLI-SPEC.md,
// e.g. "### `royo-learn evidence add <learning-id>`". The command name is the
// first word after "royo-learn ".
var docsCommandHeading = regexp.MustCompile("(?m)^###\\s+`royo-learn\\s+([a-z0-9-]+)")

// helpCommandLine matches an advertised command in the printHelp output, i.e.
// a two-space-indented "  name   Summary" line under the Commands: block.
var helpCommandLine = regexp.MustCompile(`(?m)^  ([a-z0-9-]+)\s{2,}\S`)

func documentedCommands(t *testing.T) map[string]bool {
	t.Helper()
	specPath := filepath.Join(repoRoot(t), "docs", "04-CLI-SPEC.md")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}
	out := make(map[string]bool)
	for _, m := range docsCommandHeading.FindAllStringSubmatch(string(raw), -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatalf("no command headings found in %s; the extractor is broken", specPath)
	}
	return out
}

// repoRoot resolves the repository root from this package's directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func advertisedCommands(t *testing.T) map[string]bool {
	t.Helper()
	var stdout bytes.Buffer
	if code := printHelp(&stdout); code != exitSuccess {
		t.Fatalf("printHelp exit = %d, want %d", code, exitSuccess)
	}
	out := make(map[string]bool)
	for _, m := range helpCommandLine.FindAllStringSubmatch(stdout.String(), -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatalf("no command lines found in help output; the extractor is broken:\n%s", stdout.String())
	}
	return out
}

func registryByName() map[string]command {
	out := make(map[string]command, len(commandRegistry))
	for _, c := range commandRegistry {
		out[c.name] = c
	}
	return out
}

// Condition 1: every command advertised in --help exists in the registry with a
// run func (it executes), is not deprecated, and is not pending.
func TestContract_HelpCommandsAllExecute(t *testing.T) {
	t.Parallel()
	registry := registryByName()
	for name := range advertisedCommands(t) {
		c, ok := registry[name]
		if !ok {
			t.Errorf("help advertises command %q, which is absent from the registry (phantom command)", name)
			continue
		}
		if c.run == nil {
			t.Errorf("help advertises command %q, but it has no run function", name)
		}
		if c.pending != "" {
			t.Errorf("help advertises command %q, but it is marked pending (%s)", name, c.pending)
		}
		if c.deprecated != "" {
			t.Errorf("help advertises command %q, but it is a deprecated alias of %q", name, c.deprecated)
		}
	}
}

// Condition 2 & 5 (ghost): every implemented, non-deprecated, non-pending command
// appears in help.
func TestContract_ImplementedCommandsAppearInHelp(t *testing.T) {
	t.Parallel()
	advertised := advertisedCommands(t)
	for _, c := range commandRegistry {
		if c.run == nil || c.deprecated != "" || c.pending != "" {
			continue
		}
		if !advertised[c.name] {
			t.Errorf("command %q is implemented but hidden from help (ghost command)", c.name)
		}
	}
}

// Condition 3: every command in the registry accepts --help and exits 0.
func TestContract_EveryCommandAcceptsHelp(t *testing.T) {
	t.Parallel()
	for _, c := range commandRegistry {
		for _, flag := range []string{"--help", "-h"} {
			var stdout, stderr bytes.Buffer
			code := run([]string{c.name, flag}, &stdout, &stderr)
			if code != exitSuccess {
				t.Errorf("run(%q %q) exit = %d, want %d; stderr=%s",
					c.name, flag, code, exitSuccess, stderr.String())
			}
		}
	}
}

// Condition 4: every command documented in docs/04 is in the registry, either
// implemented or explicitly pending. No documented command is missing.
func TestContract_DocumentedCommandsAreImplementedOrPending(t *testing.T) {
	t.Parallel()
	registry := registryByName()
	for name := range documentedCommands(t) {
		c, ok := registry[name]
		if !ok {
			t.Errorf("command %q is documented in docs/04-CLI-SPEC.md but absent from the registry", name)
			continue
		}
		if c.run == nil && c.pending == "" {
			t.Errorf("command %q is documented but neither implemented nor marked pending", name)
		}
	}
}

// Condition 5 (phantom) plus the D9 direction: every implemented, non-deprecated,
// non-pending command is documented in docs/04. Deprecated aliases (D9/D10) and
// pending commands are exempt: aliases carry a retirement date instead, and
// pending commands are documented-but-unbuilt.
func TestContract_NoPhantomOrUndocumentedCommand(t *testing.T) {
	t.Parallel()
	documented := documentedCommands(t)
	for _, c := range commandRegistry {
		if c.deprecated != "" || c.pending != "" {
			continue
		}
		if c.run == nil {
			t.Errorf("registry command %q has no run function and is not pending", c.name)
			continue
		}
		if !documented[c.name] {
			t.Errorf("command %q is implemented but not documented in docs/04-CLI-SPEC.md (D9: every executable command has a docs/04 entry or a retirement date)", c.name)
		}
	}
}

// The pending list is honest debt, not an escape hatch: a pending command must be
// documented in docs/04 and must not have a run function. It can only shrink.
func TestContract_PendingCommandsAreDocumentedAndUnbuilt(t *testing.T) {
	t.Parallel()
	documented := documentedCommands(t)
	pending := 0
	for _, c := range commandRegistry {
		if c.pending == "" {
			continue
		}
		pending++
		if c.run != nil {
			t.Errorf("command %q is marked pending (%s) but has a run function; remove the pending marker", c.name, c.pending)
		}
		if !documented[c.name] {
			t.Errorf("command %q is marked pending but is not documented in docs/04-CLI-SPEC.md", c.name)
		}
	}
	// Guard against silent name collisions between pending and real commands.
	seen := make(map[string]bool)
	for _, c := range commandRegistry {
		if seen[c.name] {
			t.Errorf("duplicate command name %q in the registry", c.name)
		}
		seen[c.name] = true
	}
}
