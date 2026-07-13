package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agent-royo-learn/internal/setup"
)

func TestMCPServe_UninitializedProjectRequiresInit(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"mcp-serve", "--project-root", root}, &stdout, &stderr)
	if exitCode != exitProjectNotFound {
		t.Fatalf("run(mcp-serve) exit code = %d, want %d; stderr: %s", exitCode, exitProjectNotFound, stderr.String())
	}

	var diagnostic struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &diagnostic); err != nil {
		t.Fatalf("decode mcp-serve stderr: %v; got: %s", err, stderr.String())
	}
	if diagnostic.Code != "project_not_found" {
		t.Errorf("mcp-serve error code = %q, want project_not_found", diagnostic.Code)
	}

	for _, want := range []string{
		fmt.Sprintf("no project marker found at %s", root),
		fmt.Sprintf("royo-learn init --project-root %s", root),
	} {
		if !strings.Contains(diagnostic.Message, want) {
			t.Errorf("mcp-serve message missing %q; got: %s", want, diagnostic.Message)
		}
	}
}

func TestOnboardingSkillInstallsFromRepository(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	source, err := resolveSkillsSource(root)
	if err != nil {
		t.Fatalf("resolve repository skills: %v", err)
	}
	target := t.TempDir()
	if _, err := setup.InstallSkills(source, target); err != nil {
		t.Fatalf("install repository skills: %v", err)
	}
	installed, err := os.ReadFile(filepath.Join(target, "royo-learn-onboarding", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed onboarding skill: %v", err)
	}
	content := strings.ReplaceAll(string(installed), "\r\n", "\n")
	frontmatter := "---\nname: royo-learn-onboarding\ndescription: Initialize and verify royo-learn for a project, optionally install its integrations, then continue with capture-learning.\nlicense: MIT\nmetadata:\n  author: RoyoTech\n  version: \"2.0.0\"\n---\n"
	if !strings.HasPrefix(content, frontmatter) {
		t.Error("installed onboarding skill has invalid frontmatter")
	}
	for _, want := range []string{
		"<root>/.royo-learn/config.yaml", "royo-learn init --project-root <root>",
		"royo-learn doctor --project-root <root> --json", "Optionally run", "royo-learn setup install",
		"Claude Code, Codex, and OpenCode", "../capture-learning/SKILL.md",
		"exactly once for that project root", "Project discovery walks upward",
		"Initialize one store per project root", "separate, independent stores",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("installed onboarding skill missing %q", want)
		}
	}
	initAt, setupAt := strings.Index(content, "royo-learn init"), strings.Index(content, "royo-learn setup install")
	if initAt < 0 || setupAt <= initAt {
		t.Errorf("setup install must follow mandatory init")
	}
}
