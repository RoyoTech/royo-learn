# Contributing to royo-learn

Thanks for your interest in contributing.

## How to contribute

1. **Fork** the repository
2. **Create a branch** for your change: `git checkout -b feat/my-feature`
3. **Make your changes**. Follow the conventions below.
4. **Run quality checks**: `make quality` (fmt → test → vet → build)
5. **Commit** using [conventional commits](https://www.conventionalcommits.org/):
   ```
   feat(capture): add markdown export
   fix(storage): handle WAL close on Windows
   docs(readme): update install instructions
   ```
6. **Push** to your fork and **open a Pull Request** against `main`.
7. A maintainer will review your PR. If changes are requested, push to the same
   branch — the PR updates automatically.
8. Once approved, the maintainer merges it.

## Before submitting

- All tests must pass: `go test ./...`
- No vet warnings: `go vet ./...`
- Code is formatted: `go fmt ./...`
- New features include tests
- Breaking changes are documented in the PR description
- No AI attribution (`Co-Authored-By`) in commits
- No secrets, tokens, or personal paths in code or fixtures

## Conventions

- **Language**: Go 1.24+. Code, comments, and docs in English.
- **Dependencies**: prefer standard library. Register new dependencies with a
  reason in `docs/IMPLEMENTATION-NOTES.md`.
- **Tests**: table-driven tests, meaningful names, one assertion per concept.
- **Errors**: typed errors with actionable messages.
- **File writes**: atomic (tmp + rename).
- **Commits**: conventional commits, one concern per commit.

## Project structure

See [README.md](README.md#project-structure).

## Questions?

Open an [issue](https://github.com/RoyoTech/royo-learn/issues).
