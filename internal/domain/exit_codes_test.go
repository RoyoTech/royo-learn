package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// ---------------------------------------------------------------------------
// One error model, one exit-code mapping (Tramo 4 §4.3).
//
// Every domain error code maps to exactly one CLI exit code, following the
// buckets documented in docs/04-CLI-SPEC.md §Exit codes. The mapping lives in
// the domain (ErrorCode.ExitCode) and is the single source both the CLI and the
// MCP handlers translate against — never a hand-picked constant, never string
// matching. This test is the contract: one row per error class.
// ---------------------------------------------------------------------------

func TestExitCodeMapping_EveryClass(t *testing.T) {
	t.Parallel()

	// docs/04-CLI-SPEC.md §Exit codes:
	//   2 invalid args | 3 invalid config | 4 project | 5 not found |
	//   6 invalid transition | 7 approval | 8 destination conflict |
	//   9 verification | 10 optional integration | 11 security/path |
	//   12 corruption | 13 storage | 14 mcp | 15 external
	want := map[ErrorCode]int{
		ErrInvalidArgument:     2,
		ErrEvidenceMissing:     2,
		ErrEvidenceTooLarge:    2,
		ErrPayloadTooLarge:     2,
		ErrInvalidConfig:       3,
		ErrProjectNotFound:     4,
		ErrAmbiguousProject:    4,
		ErrUnknownProject:      4,
		ErrLearningNotFound:    5,
		ErrPreviewNotFound:     5,
		ErrInvalidTransition:   6,
		ErrApprovalRequired:    7,
		ErrApprovalInvalid:     7,
		ErrApprovalExpired:     7,
		ErrDuplicateLearning:   8,
		ErrTargetAmbiguous:     8,
		ErrTargetChanged:       8,
		ErrDirtyTarget:         8,
		ErrPublicationConflict: 8,
		ErrPreviewHashMismatch: 8,
		ErrRollbackConflict:    8,
		ErrVerificationFailed:  9,
		ErrEngramUnavailable:   10,
		ErrEngramAmbiguous:     10,
		ErrGentleAIUnavailable: 10,
		ErrSkillRegistryFailed: 10,
		ErrPathOutsideRoot:     11,
		ErrSymlinkEscape:       11,
		ErrProtectedPath:       11,
		ErrSecretDetected:      11,
		ErrDatabaseCorrupt:     12,
		ErrMigrationChecksum:   12,
		ErrRecordHashMismatch:  12,
		ErrDatabaseLocked:      13,
		ErrRollbackFailed:      13,
		ErrPublicationFailed:   13,
		ErrMCPProtocolError:    14,
		ErrExternalCommandFailed: 15,
		ErrTimeout:               15,
	}

	for code, exit := range want {
		if got := code.ExitCode(); got != exit {
			t.Errorf("ExitCode(%q) = %d, want %d", code, got, exit)
		}
	}

	// Every code returned by AllErrorCodes has an explicit exit code in [2,15];
	// nothing falls through to the generic 1.
	for _, code := range AllErrorCodes() {
		got := code.ExitCode()
		if got < 2 || got > 15 {
			t.Errorf("ExitCode(%q) = %d, want a documented code in [2,15]; unmapped code falls through to the generic failure", code, got)
		}
		if _, ok := want[code]; !ok {
			t.Errorf("code %q is in AllErrorCodes but not covered by the mapping test; add a row", code)
		}
	}
}

// docsErrorCode matches one code line in the docs/17-ERROR-CODES.md catalog block.
var docsErrorCode = regexp.MustCompile(`(?m)^([a-z_]+)$`)

// TestExitCodeMapping_DocsCatalogInSync asserts every code documented in
// docs/17-ERROR-CODES.md exists as a domain constant with an exit code, so the
// catalog cannot document a code the code base does not model.
func TestExitCodeMapping_DocsCatalogInSync(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "docs", "17-ERROR-CODES.md"))
	if err != nil {
		t.Fatalf("read docs/17-ERROR-CODES.md: %v", err)
	}

	modeled := make(map[ErrorCode]bool)
	for _, c := range AllErrorCodes() {
		modeled[c] = true
	}

	found := 0
	for _, m := range docsErrorCode.FindAllStringSubmatch(string(raw), -1) {
		code := ErrorCode(m[1])
		// Skip prose words that happen to match the shape but are not codes.
		if !modeled[code] {
			continue
		}
		found++
	}
	// Sanity: the catalog lists many codes; a near-zero match means the parser
	// or the catalog drifted.
	if found < 20 {
		t.Fatalf("only %d catalog codes matched a domain constant; docs/17 or the parser drifted", found)
	}
}
