package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// README <-> binary contract (plan §Tramo 5).
//
// The Quick Start is the first thing a user runs. v0.1.9 shipped a README that
// advertised commands the binary did not have — the exact failure this binds
// shut: every `royo-learn <command>` the README shows must be a real,
// implemented, non-deprecated, non-pending command.
//
// Scope, stated honestly: this proves each advertised command EXISTS and is
// dispatchable. It does not execute the Quick Start literally, because the
// block is illustrative — it carries placeholders (`/path/to/your/project`,
// `<learning-id>`) and `self-update`, which reaches the network. The commands
// themselves are executed end to end by the e2e suite and by the clean-install
// job in CI, which runs the installed binary from an empty project.
// ---------------------------------------------------------------------------

// readmeInvocation matches a `royo-learn <command>` call inside the README,
// including continuation lines of a multi-line invocation.
var readmeInvocation = regexp.MustCompile(`(?m)royo-learn\s+([a-z0-9-]+)`)

// quickStartBlock isolates the "## Quick Start" section up to the next H2, or
// to the end of the file when it is the last section.
var quickStartBlock = regexp.MustCompile(`(?s)## Quick Start\n(.*?)(?:\n## |\z)`)

func TestContract_ReadmeQuickStartCommandsExist(t *testing.T) {
	t.Parallel()

	readmePath := filepath.Join(repoRoot(t), "README.md")
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}
	// The repo checks out CRLF on Windows; the contract is the same either way.
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))

	block := quickStartBlock.FindSubmatch(raw)
	if block == nil {
		t.Fatalf("no '## Quick Start' section found in %s; the extractor is broken", readmePath)
	}

	found := map[string]bool{}
	for _, m := range readmeInvocation.FindAllSubmatch(block[1], -1) {
		found[string(m[1])] = true
	}
	if len(found) == 0 {
		t.Fatalf("no royo-learn invocations found in the Quick Start; the extractor is broken")
	}

	registry := registryByName()
	for _, name := range sortedKeys(found) {
		// A bare flag such as `royo-learn --help` is not a command.
		if name == "help" {
			continue
		}
		cmd, ok := registry[name]
		if !ok {
			t.Errorf("the README Quick Start advertises %q, which is not a command at all", name)
			continue
		}
		if cmd.run == nil {
			t.Errorf("the README Quick Start advertises %q, which cannot be executed", name)
		}
		if cmd.pending != "" {
			t.Errorf("the README Quick Start advertises %q, which is declared but not built (pending: %s)",
				name, cmd.pending)
		}
		if cmd.deprecated != "" {
			t.Errorf("the README Quick Start advertises the deprecated command %q; use %q",
				name, cmd.deprecated)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
