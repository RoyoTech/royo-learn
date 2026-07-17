package evidence

import (
	"context"
	"fmt"
	"strings"

	"agent-royo-learn/internal/domain"
)

// The collectors below are the ONLY ones the contract offers: evidence supplied
// directly, `git status`, `git diff`, and the result of an explicitly allowlisted
// command. Twenty collector types is a taxonomy, not a feature.
//
// Every collector routes through CommandRunner, which already runs without a
// shell, caps output, enforces a timeout and redacts its own output. Nothing here
// reimplements any of that.

// CollectGitStatus captures `git status --porcelain` in repoPath.
func (s *Service) CollectGitStatus(ctx context.Context, repoPath string) (Item, error) {
	result, err := s.run(ctx, repoPath, []string{"git", "-C", repoPath, "status", "--porcelain"})
	if err != nil {
		return Item{}, fmt.Errorf("collect git status: %w", err)
	}
	return Item{
		Kind:     domain.KindCommand,
		Summary:  "git status --porcelain",
		Source:   "git status --porcelain",
		Content:  result.Stdout,
		Command:  []string{"git", "status", "--porcelain"},
		ExitCode: &result.ExitCode,
	}, nil
}

// CollectGitDiff captures the working-tree `git diff` in repoPath.
func (s *Service) CollectGitDiff(ctx context.Context, repoPath string) (Item, error) {
	result, err := s.run(ctx, repoPath, []string{"git", "-C", repoPath, "diff", "--unified=3"})
	if err != nil {
		return Item{}, fmt.Errorf("collect git diff: %w", err)
	}
	return Item{
		Kind:     domain.KindGitDiff,
		Summary:  "git diff (working tree)",
		Source:   "git diff --unified=3",
		Content:  result.Stdout,
		Command:  []string{"git", "diff", "--unified=3"},
		ExitCode: &result.ExitCode,
	}, nil
}

// CollectCommand runs an explicitly allowlisted command and captures its output.
// A command outside the allowlist is refused, not silently skipped.
func (s *Service) CollectCommand(ctx context.Context, repoPath string, argv []string) (Item, error) {
	if len(argv) == 0 {
		return Item{}, domain.NewValidationError(domain.ErrInvalidArgument,
			"evidence command: the command is empty")
	}
	result, err := s.run(ctx, repoPath, argv)
	if err != nil {
		return Item{}, fmt.Errorf("collect command: %w", err)
	}
	joined := strings.Join(argv, " ")
	content := result.Stdout
	if result.Stderr != "" {
		content = content + "\n" + result.Stderr
	}
	return Item{
		Kind:     domain.KindCommand,
		Summary:  fmt.Sprintf("%s (exit %d)", joined, result.ExitCode),
		Source:   joined,
		Content:  content,
		Command:  argv,
		ExitCode: &result.ExitCode,
	}, nil
}

// run executes argv through the shared CommandRunner, confined to repoPath and
// bound by the service allowlist.
func (s *Service) run(ctx context.Context, repoPath string, argv []string) (*CommandResult, error) {
	if s == nil {
		return nil, fmt.Errorf("evidence: nil service")
	}
	runner := &CommandRunner{
		AllowedCommands: s.allowed,
		Root:            repoPath,
		Dir:             repoPath,
	}
	return runner.Run(ctx, argv)
}
