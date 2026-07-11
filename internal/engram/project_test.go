package engram

import (
	"errors"
	"testing"
)

func TestResolveProject_ExplicitName(t *testing.T) {
	// When an explicit project name is provided, it should be used.
	opts := ResolveOptions{
		ExplicitName: "my-project",
		GitRemote:    "origin",
		GitURL:       "git@github.com:user/other.git",
	}
	name, source, err := ResolveProject(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-project" {
		t.Errorf("name = %q, want 'my-project'", name)
	}
	if source != SourceExplicit {
		t.Errorf("source = %q, want %q", source, SourceExplicit)
	}
}

func TestResolveProject_FallbackToGit(t *testing.T) {
	// Without explicit name, prefer git remote name.
	opts := ResolveOptions{
		GitRemote: "origin",
		GitURL:    "https://github.com/org/my-repo.git",
	}
	name, source, err := ResolveProject(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-repo" {
		t.Errorf("name = %q, want 'my-repo'", name)
	}
	if source != SourceGitRemote {
		t.Errorf("source = %q, want %q", source, SourceGitRemote)
	}
}

func TestResolveProject_FallbackToGitSSH(t *testing.T) {
	opts := ResolveOptions{
		GitRemote: "upstream",
		GitURL:    "git@github.com:user/project-name.git",
	}
	name, source, err := ResolveProject(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "project-name" {
		t.Errorf("name = %q, want 'project-name'", name)
	}
	if source != SourceGitRemote {
		t.Errorf("source = %q, want %q", source, SourceGitRemote)
	}
}

func TestResolveProject_Ambiguous(t *testing.T) {
	// No explicit name and no git URL → ambiguous.
	opts := ResolveOptions{
		GitRemote: "origin",
		GitURL:    "", // no git info
	}
	_, _, err := ResolveProject(opts)
	if err == nil {
		t.Fatal("expected ambiguous error, got nil")
	}
	if !errors.Is(err, ErrAmbiguousProject) {
		t.Errorf("expected ErrAmbiguousProject, got %v", err)
	}
}

func TestResolveProject_ExplicitOverridesAmbiguous(t *testing.T) {
	// Even without git, explicit name resolves.
	opts := ResolveOptions{
		ExplicitName: "explicit-wins",
	}
	name, _, err := ResolveProject(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "explicit-wins" {
		t.Errorf("name = %q, want 'explicit-wins'", name)
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "https", url: "https://github.com/org/repo.git", want: "repo"},
		{name: "ssh", url: "git@github.com:user/my-project.git", want: "my-project"},
		{name: "no .git", url: "https://github.com/a/b", want: "b"},
		{name: "with path", url: "https://example.com/org/repo/sub.git", want: "sub"},
		{name: "empty", url: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoName(tt.url)
			if got != tt.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
