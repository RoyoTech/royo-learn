package mcpserver

import (
	"flag"
	"testing"

	"agent-royo-learn/internal/gendocs"
)

var updateDocs = flag.Bool("update-docs", false, "rewrite the generated reference documents")

func generatedHeader(generator, source string) string {
	return gendocs.Header(generator, source)
}

// assertGeneratedDoc writes the document when -update-docs is set, and
// otherwise fails if the checked-in copy no longer matches what the registry
// produces. That failure means the code changed and the reference did not —
// the document is derived, so the code is right and the file is stale.
func assertGeneratedDoc(t *testing.T, name, want string) {
	t.Helper()

	dir, err := gendocs.Dir(2)
	if err != nil {
		t.Fatalf("resolve docs/generated: %v", err)
	}
	if *updateDocs {
		if err := gendocs.Write(dir, name, want); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return
	}

	got, err := gendocs.Read(dir, name)
	if err != nil {
		t.Fatalf("read docs/generated/%s: %v\nregenerate with: go test ./... -update-docs", name, err)
	}
	if got != want {
		t.Fatalf("docs/generated/%s is stale.\n--- checked in\n%s\n--- generated from the registry\n%s\n"+
			"The document is DERIVED. Regenerate it with -update-docs; do not edit it by hand.",
			name, got, want)
	}
}
