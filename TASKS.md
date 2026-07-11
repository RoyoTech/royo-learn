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
- [ ] repositories.
- [ ] transactions.
- [ ] audit.
- [ ] FTS5.
- [ ] integrity.
- [ ] rebuild.

Aceptación: migration + CRUD + search.

## T03 Dominio

- [ ] enums.
- [ ] transitions.
- [ ] validation.
- [ ] typed errors.
- [ ] canonical JSON.
- [ ] hashing.

Aceptación: tests de todas las transiciones.

## T04 Evidence/security

- [ ] redaction.
- [ ] blob store.
- [ ] command runner.
- [ ] Git evidence.
- [ ] path security.
- [ ] limits.
- [ ] tests malicious inputs.

Aceptación: secretos nunca persisten.

## T05 Capture/search

- [ ] capture service.
- [ ] idempotency.
- [ ] exact dedup.
- [ ] lexical similar.
- [ ] Markdown records.
- [ ] CLI.
- [ ] tests.

Aceptación: dos llamadas iguales producen una entidad.

## T06 Curate

- [ ] curation service.
- [ ] relation service.
- [ ] evidence thresholds.
- [ ] CLI.
- [ ] records update.
- [ ] tests.

Aceptación: no aprobar hipótesis sin justificación requerida.

## T07 Preview/approval

- [ ] target resolver.
- [ ] diff generator.
- [ ] canonical preview.
- [ ] policies.
- [ ] approvals.
- [ ] invalidation.
- [ ] tests.

Aceptación: mutar target invalida publicación.

## T08 Publish/rollback

- [ ] atomic writer.
- [ ] backups.
- [ ] managed blocks.
- [ ] Skill validator.
- [ ] verify runner.
- [ ] journal.
- [ ] rollback.
- [ ] dirty worktree handling.
- [ ] tests.

Aceptación: fallo de verification restaura todos los archivos.

## T09 MCP

- [ ] server.
- [ ] instructions.
- [ ] tools.
- [ ] schemas.
- [ ] profiles.
- [ ] middleware.
- [ ] size/time limits.
- [ ] conformance/smoke.
- [ ] Codex test.

Aceptación: Codex lista y llama tools.

## T10 Engram

- [ ] health.
- [ ] search.
- [ ] context.
- [ ] optional save.
- [ ] degradation.
- [ ] project ambiguity.
- [ ] tests fake/real.

Aceptación: Engram apagado no impide capture.

## T11 Gentle-AI/Codex setup

- [ ] Skill install helper.
- [ ] registry refresh.
- [ ] Codex MCP register.
- [ ] config backup.
- [ ] no duplicate.
- [ ] doctor checks.

Aceptación: stack existente sigue operativo.

## T12 Recurrence

- [ ] fingerprint.
- [ ] occurrence.
- [ ] metrics.
- [ ] needs_review policy.
- [ ] CLI/MCP.
- [ ] tests.

Aceptación: segunda recurrencia visible en status.

## T13 Install/release

- [ ] install.ps1.
- [ ] install.sh.
- [ ] uninstall.
- [ ] GoReleaser.
- [ ] cross-build.
- [ ] checksums.
- [ ] SBOM.
- [ ] docs.

## T14 E2E/final

- [ ] `e2e --temp`.
- [ ] Linux.
- [ ] Windows.
- [ ] MCP Inspector/client.
- [ ] Codex manual.
- [ ] security suite.
- [ ] final report.
- [ ] cero TODO crítico.
