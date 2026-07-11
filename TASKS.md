# TASKS — Plan ejecutable para Codex

Codex debe marcar cada ítem al completarlo y añadir evidencia de prueba.

## T00 Bootstrap

- [x] Crear `go.mod`.
- [x] Pin SDK oficial MCP.
- [x] Pin SQLite sin CGO.
- [x] Crear `cmd/royo-learn/main.go`.
- [x] Version injection.
- [x] Logger stderr.
- [x] CI base.
- [x] Makefile.
- [x] `docs/IMPLEMENTATION-NOTES.md`.

Aceptación: `royo-learn version --json`.

T00 implementation evidence is recorded in the Engram apply-progress artifact.
The first commit remains parent-controlled: T00 ends as a staged, uncommitted
combined baseline and bootstrap tree.

## T01 Config y proyecto

- [ ] Config usuario/proyecto.
- [ ] Precedencia.
- [ ] Git root.
- [ ] project key.
- [ ] ambigüedad.
- [ ] monorepo.
- [ ] tests Windows paths.

Aceptación: `doctor` resuelve repo correctamente.

## T02 Storage

- [x] migrations.
- [x] SQLite pragmas.
- [x] repositories.
- [x] transactions.
- [x] audit.
- [x] FTS5.
- [x] integrity.
- [x] rebuild.

Aceptación: migration + CRUD + search.

## T03 Dominio

- [x] enums.
- [x] transitions.
- [x] validation.
- [x] typed errors.
- [x] canonical JSON.
- [x] hashing.

Aceptación: tests de todas las transiciones.

## T04 Evidence/security

- [x] redaction.
- [x] blob store.
- [x] command runner.
- [x] Git evidence.
- [x] path security.
- [x] limits.
- [x] tests malicious inputs.

Aceptación: secretos nunca persisten.

## T05 Capture/search

- [x] capture service.
- [x] idempotency.
- [x] exact dedup.
- [x] lexical similar.
- [x] Markdown records.
- [x] CLI.
- [x] tests.

Aceptación: dos llamadas iguales producen una entidad.

## T06 Curate

- [x] curation service.
- [x] relation service.
- [x] evidence thresholds.
- [x] CLI.
- [x] records update.
- [x] tests.

Aceptación: no aprobar hipótesis sin justificación requerida.

## T07 Preview/approval

- [x] target resolver.
- [x] diff generator.
- [x] canonical preview.
- [x] policies.
- [x] approvals.
- [x] invalidation.
- [x] tests.

Aceptación: mutar target invalida publicación.

## T08 Publish/rollback

- [x] atomic writer.
- [x] backups.
- [x] managed blocks.
- [x] Skill validator.
- [x] verify runner.
- [x] journal.
- [x] rollback.
- [x] dirty worktree handling.
- [x] tests.

Aceptación: fallo de verification restaura todos los archivos.

## T09 MCP

- [x] server.
- [x] instructions.
- [x] tools.
- [x] schemas.
- [x] profiles.
- [x] middleware.
- [x] size/time limits.
- [x] conformance/smoke.
- [ ] Codex test.

Aceptación: Codex lista y llama tools.

## T10 Engram

- [x] health.
- [x] search.
- [x] context.
- [x] optional save.
- [x] degradation.
- [x] project ambiguity.
- [x] tests fake/real.

Aceptación: Engram apagado no impide capture.

## T11 Gentle-AI/Codex setup

- [x] Skill install helper.
- [x] registry refresh.
- [x] Codex MCP register.
- [x] config backup.
- [x] no duplicate.
- [x] doctor checks.

Aceptación: stack existente sigue operativo.

## T12 Recurrence

- [x] fingerprint.
- [x] occurrence.
- [x] metrics.
- [x] needs_review policy.
- [x] CLI/MCP.
- [x] tests.

Aceptación: segunda recurrencia visible en status.

## T13 Install/release

- [x] install.ps1.
- [x] install.sh.
- [x] uninstall.
- [x] GoReleaser.
- [x] cross-build.
- [x] checksums.
- [x] SBOM.
- [x] docs.

## T14 E2E/final

- [x] `e2e --temp`.
- [x] Linux.
- [x] Windows.
- [x] MCP Inspector/client.
- [x] Codex manual.
- [x] security suite.
- [x] final report.
- [x] cero TODO crítico.
