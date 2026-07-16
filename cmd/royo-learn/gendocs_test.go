package main

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"agent-royo-learn/internal/gendocs"
)

// ---------------------------------------------------------------------------
// Generated CLI reference (plan §Tramo 6).
//
// docs/generated/CLI_REFERENCE.md is DERIVED from commandRegistry — the same
// registry the dispatcher and --help derive from. It cannot advertise a command
// the binary does not have, because it does not know how to invent one.
//
// Regenerate with:
//
//	go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs
// ---------------------------------------------------------------------------

var updateDocs = flag.Bool("update-docs", false, "rewrite the generated reference documents")

func TestGeneratedCLIReferenceIsCurrent(t *testing.T) {
	want := buildCLIReference()

	dir, err := gendocs.Dir(2)
	if err != nil {
		t.Fatalf("resolve docs/generated: %v", err)
	}
	if *updateDocs {
		if err := gendocs.Write(dir, "CLI_REFERENCE.md", want); err != nil {
			t.Fatalf("write: %v", err)
		}
		return
	}

	got, err := gendocs.Read(dir, "CLI_REFERENCE.md")
	if err != nil {
		t.Fatalf("read docs/generated/CLI_REFERENCE.md: %v\nregenerate with -update-docs", err)
	}
	if got != want {
		t.Fatalf("docs/generated/CLI_REFERENCE.md is stale.\n--- checked in\n%s\n--- generated\n%s\n"+
			"The document is DERIVED from commandRegistry. Regenerate it; do not edit it by hand.", got, want)
	}
}

func buildCLIReference() string {
	var sb strings.Builder
	sb.WriteString(gendocs.Header("cmd/royo-learn/gendocs_test.go", "el registro `commandRegistry`"))
	sb.WriteString("# Referencia CLI\n\n")

	var implemented, deprecated, pending []command
	for _, c := range commandRegistry {
		switch {
		case c.deprecated != "":
			deprecated = append(deprecated, c)
		case c.pending != "":
			pending = append(pending, c)
		default:
			implemented = append(implemented, c)
		}
	}

	sb.WriteString(fmt.Sprintf("Comandos implementados: **%d**.\n\n", len(implemented)))
	sb.WriteString("| Comando | Resumen |\n|---------|---------|\n")
	for _, c := range implemented {
		sb.WriteString(fmt.Sprintf("| `royo-learn %s` | %s |\n", c.name, escapePipes(c.summary)))
	}

	sb.WriteString("\n## Comandos deprecated\n\n")
	if len(deprecated) == 0 {
		sb.WriteString("Ninguno.\n")
	} else {
		sb.WriteString("| Deprecated | Canónico | Resumen |\n|------------|----------|---------|\n")
		for _, c := range deprecated {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", c.name, c.deprecated, escapePipes(c.summary)))
		}
	}

	sb.WriteString("\n## Comandos declarados y no construidos\n\n")
	sb.WriteString("Un comando aquí está documentado pero **no se puede ejecutar todavía**.\n")
	sb.WriteString("La lista se deriva del registro, así que no puede mentir por omisión.\n\n")
	if len(pending) == 0 {
		sb.WriteString("Ninguno: hoy ninguna superficie declara algo que no exista.\n")
	} else {
		sb.WriteString("| Comando | Pendiente de |\n|---------|--------------|\n")
		for _, c := range pending {
			sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", c.name, c.pending))
		}
	}
	return sb.String()
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
