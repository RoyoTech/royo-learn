package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExperienceUnified_MissingSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runExperience([]string{"scan"}, &stdout, &stderr); code != exitInvalidArguments {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--source") || !strings.Contains(stderr.String(), "opencode|claudecode|codex") {
		t.Fatalf("stderr = %q, want source usage", stderr.String())
	}
}

func TestExperienceUnified_InvalidSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runExperience([]string{"scan", "--source=invalid"}, &stdout, &stderr); code != exitInvalidArguments {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "allowed values: opencode, claudecode, codex") {
		t.Fatalf("stderr = %q, want allowed values", stderr.String())
	}
}

func TestExperienceUnified_OpenCodeAccepted(t *testing.T) {
	assertUnifiedSourceAccepted(t, "opencode")
}

func TestExperienceUnified_ClaudecodeAccepted(t *testing.T) {
	assertUnifiedSourceAccepted(t, "claudecode")
}

func TestExperienceUnified_CodexAccepted(t *testing.T) {
	assertUnifiedSourceAccepted(t, "codex")
}

func assertUnifiedSourceAccepted(t *testing.T, source string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"scan", "--source=" + source}, &stdout, &stderr)
	if code != exitInvalidArguments || !strings.Contains(stderr.String(), "--project-root") {
		t.Fatalf("source %q was not routed: exit=%d stderr=%s", source, code, stderr.String())
	}
}

func TestExperienceUnified_OutputKeysParity(t *testing.T) {
	root := setupProjectRoot(t)
	fixture := makeOpencodeFixtureEmpty(t, root)
	args := []string{"--project-root", root, "--fixture", fixture}

	var unified, legacy, unifiedErr, legacyErr bytes.Buffer
	if code := runExperienceSource("opencode", args, &unified, &unifiedErr); code != 0 {
		t.Fatalf("unified exit=%d stderr=%s", code, unifiedErr.String())
	}
	if code := runExperienceOpencodeScan(args, &legacy, &legacyErr); code != 0 {
		t.Fatalf("legacy exit=%d stderr=%s", code, legacyErr.String())
	}
	if !bytes.Equal(unified.Bytes(), legacy.Bytes()) {
		t.Fatalf("stdout differs\nunified=%s\nlegacy=%s", unified.String(), legacy.String())
	}
}

func TestExperienceUnified_DeprecationNote_CollapseOff(t *testing.T) {
	t.Setenv("ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE", "false")
	var stdout, stderr bytes.Buffer
	code := runExperience([]string{"opencode", "scan"}, &stdout, &stderr)
	if code != exitInvalidArguments {
		t.Fatalf("exit=%d, want project-root validation code 2", code)
	}
	if !strings.Contains(stderr.String(), "DEPRECATED: use 'experience scan --source=opencode'") {
		t.Fatalf("stderr=%q, want deprecation note", stderr.String())
	}
}

func TestExperienceUnified_NoDeprecationNote_Unified(t *testing.T) {
	t.Setenv("ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE", "false")
	var stdout, stderr bytes.Buffer
	_ = runExperience([]string{"scan", "--source=codex"}, &stdout, &stderr)
	if strings.Contains(stderr.String(), "DEPRECATED:") {
		t.Fatalf("unified stderr contains deprecation note: %q", stderr.String())
	}
}
