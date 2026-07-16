// Package gendocs supports the generated reference documents under
// docs/generated/ (plan §Tramo 6).
//
// Those documents are DERIVED from the authoritative registries in code — the
// CLI command registry, the MCP tool registry, the error-code table — and are
// never edited by hand. The plan is explicit: "No copiar la misma lista a mano
// en cinco documentos". A hand-maintained list is how v0.1.9 came to document
// commands and tools that did not exist.
//
// This package is imported only by the generator tests that live inside each
// registry's own package, because the registries are unexported: the generator
// must sit next to the truth it renders.
package gendocs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir returns the docs/generated directory, resolved from a package directory
// depth levels below the repository root.
func Dir(depth int) (string, error) {
	parts := make([]string, 0, depth)
	for i := 0; i < depth; i++ {
		parts = append(parts, "..")
	}
	root, err := filepath.Abs(filepath.Join(parts...))
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	return filepath.Join(root, "docs", "generated"), nil
}

// Header is the banner every generated document carries. It names the generator
// so the next person edits the source instead of the output.
func Header(generator, source string) string {
	return fmt.Sprintf(
		"<!-- GENERADO AUTOMÁTICAMENTE — NO EDITAR A MANO. -->\n"+
			"<!-- Fuente: %s. Generador: %s. -->\n"+
			"<!-- Regenerar: go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs -->\n\n",
		source, generator)
}

// Write writes a generated document.
func Write(dir, name, content string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Read reads a generated document, normalizing line endings so a CRLF checkout
// on Windows compares equal to the LF the generator produces.
func Read(dir, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n"), nil
}
