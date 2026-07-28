// Package codex implements the read-only experience adapter for Codex rollout JSONL transcripts.
//
// It discovers rollout files, validates their schema, normalizes complete turns
// into experience envelopes, and resolves bounded redacted trace excerpts. It
// never mutates Codex data, executes transcript content, or persists reasoning.
//
// Package version: codex/rollout-v1.
package codex
