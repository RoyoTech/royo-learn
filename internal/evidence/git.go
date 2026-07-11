package evidence

import (
	"context"
	"fmt"
)

// GitDiffResult holds the output of a git diff collection.
type GitDiffResult struct {
	Diff     string
	From     string
	To       string
	RepoPath string
}

// CollectDiff runs git diff in repoPath from the given base to target.
// Returns the unified diff output and metadata.
func CollectDiff(ctx context.Context, repoPath, from, to string) (*GitDiffResult, error) {
	runner := &CommandRunner{}
	args := []string{"git", "-C", repoPath, "diff", "--unified=3", from, to}

	result, err := runner.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}

	return &GitDiffResult{
		Diff:     result.Stdout,
		From:     from,
		To:       to,
		RepoPath: repoPath,
	}, nil
}

// CollectCommitInfo returns git commit metadata for a given ref.
func CollectCommitInfo(ctx context.Context, repoPath, ref string) (string, error) {
	runner := &CommandRunner{}
	args := []string{"git", "-C", repoPath, "log", "-1", "--format=fuller", ref}

	result, err := runner.Run(ctx, args)
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}

	return result.Stdout, nil
}
