package domain

import (
	"flag"
	"fmt"
	"sort"
	"strings"
	"testing"

	"agent-royo-learn/internal/gendocs"
)

// ---------------------------------------------------------------------------
// Generated error reference (plan §Tramo 6).
//
// docs/generated/ERROR_REFERENCE.md is DERIVED from AllErrorCodes() and
// ErrorCode.ExitCode() — the single source both the CLI and the MCP handlers
// map errors through. A hand-maintained table would be a second mapping, and a
// second mapping is one that disagrees.
//
// Regenerate with:
//
//	go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs
// ---------------------------------------------------------------------------

var updateDocs = flag.Bool("update-docs", false, "rewrite the generated reference documents")

func TestGeneratedErrorReferenceIsCurrent(t *testing.T) {
	want := buildErrorReference()

	dir, err := gendocs.Dir(2)
	if err != nil {
		t.Fatalf("resolve docs/generated: %v", err)
	}
	if *updateDocs {
		if err := gendocs.Write(dir, "ERROR_REFERENCE.md", want); err != nil {
			t.Fatalf("write: %v", err)
		}
		return
	}

	got, err := gendocs.Read(dir, "ERROR_REFERENCE.md")
	if err != nil {
		t.Fatalf("read docs/generated/ERROR_REFERENCE.md: %v\nregenerate with -update-docs", err)
	}
	if got != want {
		t.Fatalf("docs/generated/ERROR_REFERENCE.md is stale.\n--- checked in\n%s\n--- generated\n%s\n"+
			"The document is DERIVED from AllErrorCodes(). Regenerate it; do not edit it by hand.", got, want)
	}
}

func buildErrorReference() string {
	var sb strings.Builder
	sb.WriteString(gendocs.Header("internal/domain/gendocs_test.go",
		"`AllErrorCodes()` y `ErrorCode.ExitCode()`"))
	sb.WriteString("# Referencia de errores\n\n")
	sb.WriteString("Todo error del producto lleva uno de estos códigos estables. El código\n")
	sb.WriteString("determina el exit code: ninguna superficie elige una constante a mano ni\n")
	sb.WriteString("interpreta un error comparando cadenas.\n\n")

	codes := AllErrorCodes()
	sb.WriteString(fmt.Sprintf("Códigos: **%d**.\n\n", len(codes)))

	byExit := map[int][]string{}
	for _, c := range codes {
		byExit[c.ExitCode()] = append(byExit[c.ExitCode()], string(c))
	}
	exits := make([]int, 0, len(byExit))
	for e := range byExit {
		exits = append(exits, e)
	}
	sort.Ints(exits)

	sb.WriteString("| Exit code | Códigos |\n|-----------|---------|\n")
	for _, e := range exits {
		names := byExit[e]
		sort.Strings(names)
		quoted := make([]string, 0, len(names))
		for _, n := range names {
			quoted = append(quoted, "`"+n+"`")
		}
		sb.WriteString(fmt.Sprintf("| %d | %s |\n", e, strings.Join(quoted, ", ")))
	}

	sb.WriteString("\n## Sobre la recuperabilidad\n\n")
	sb.WriteString("Un código no recuperable señala un estado que el usuario no puede corregir\n")
	sb.WriteString("reintentando: exige intervención explícita.\n")
	return sb.String()
}
