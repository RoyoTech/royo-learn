// Package claudecode implements the experience-discovery adapter for Claude Code.
//
// Scope:
//   - Read-only access to Claude Code's append-only JSONL transcript store. The
//     adapter never mutates the upstream file: no append, no rewrite, no
//     rotation. mtime + size + SHA-256 tests pin this invariant.
//   - Translation of Claude Code's session/turn schema into the neutral
//     experience.ExperienceEnvelope consumed by internal/experience.Service.
//   - Path security enforced through internal/project.Canonicalize and
//     internal/project.IsInsideRoot. Symlink escapes and paths outside the
//     stored project root are rejected with typed errors.
//
// Boundaries (per docs/22-ADAPTER-CONTRACT.md and docs/24-EXPERIENCE-THREAT-MODEL.md):
//   - The adapter discovers, health-checks, scans and resolves traces. It does
//     not approve, publish, or mutate a Learning. Promotion is the only bridge
//     between observed experience and approved knowledge, and that bridge runs
//     through capture.Service in a separate package.
//   - Fingerprints and digests are calculated by the core experience.Service
//     after redaction. The adapter only redacts well-known transport fields
//     (paths, IDs) and forwards the original transcript text untouched.
//   - thinking/reasoning blocks (the upstream Claude Code "thinking" content
//     type) are dropped at envelope-build time, never interpreted. This is
//     the concrete application of AGENTS.md regla 9 (no LLM reasoning in v1)
//     and ADR-0001.
//   - The adapter runs entirely offline. No shell, no network, no daemon.
//
// Package version: claude-code/jsonl-v1 (see docs/22-ADAPTER-CONTRACT.md §7).
package claudecode
