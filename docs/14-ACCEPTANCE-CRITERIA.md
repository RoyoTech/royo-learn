# Criterios de aceptación finales

## A. Construcción

- [ ] `go build ./cmd/royo-learn` pasa.
- [ ] cross-build Windows/Linux/macOS.
- [ ] binario único sin runtime externo.
- [ ] `go mod verify` pasa.
- [ ] `go vet ./...` pasa.
- [ ] `go test ./...` pasa.
- [ ] `go test -race ./...` pasa en Linux.

## B. CLI

- [ ] todos los comandos de `docs/04` existen.
- [ ] `--json` es válido y estable.
- [ ] exit codes coinciden.
- [ ] help incluye ejemplos.
- [ ] no hay prompts en CI.

## C. MCP

- [ ] servidor inicia por stdio.
- [ ] Codex lista el servidor.
- [ ] tools coinciden con `docs/05`.
- [ ] `stdout` no contiene logs.
- [ ] tool schemas rechazan campos inválidos.
- [ ] publicación está marcada como write/destructive.
- [ ] errores son estructurados.
- [ ] payload limits funcionan.

## D. Persistencia

- [ ] migrations idempotentes.
- [ ] FTS5 funcional.
- [ ] audit append-only.
- [ ] record Markdown generado.
- [ ] rebuild index funciona.
- [ ] corrupción detectada.
- [ ] WAL y busy timeout configurados.

## E. Ciclo de aprendizaje

- [ ] capture idempotente: la misma `idempotency_key` no crea un segundo aprendizaje ni duplica evidencia.
- [ ] capture acepta evidencia embebida (`evidence[]`) por CLI y por MCP.
- [ ] `learning_add_evidence` (MCP) y `royo-learn evidence add` (CLI) adjuntan evidencia después de la captura.
- [ ] búsqueda previa.
- [ ] curación con estados válidos.
- [ ] needs_evidence.
- [ ] un aprendizaje en `needs_evidence` puede volver a `approved` tras adjuntar evidencia, **sin tocar SQLite a mano**.
- [ ] merge.
- [ ] reject.
- [ ] approve.
- [ ] **`captured → needs_evidence → evidence_attached → approved` recorrido íntegramente por interfaces públicas.** Ninguna prueba puede llamar a `storage.SaveEvidence` directamente.
- [ ] preview.
- [ ] approval ligada a hash.
- [ ] publish.
- [ ] verify.
- [ ] rollback.
- [ ] tras `publish`, SQLite y el registro Markdown reflejan `published`.
- [ ] tras un rollback exitoso, el aprendizaje vuelve a `approved` y el registro
      Markdown refleja ese estado; un rollback fallido no lo cambia (D18).
- [ ] antes de la primera escritura existe una publicación `in_progress` con
      metadatos suficientes para recuperar todos los destinos (D20).
- [ ] un destino modificado fuera del proceso nunca se sobrescribe durante
      publicación o rollback; se devuelve un patch de reversión accionable (D20).
- [ ] CLI y MCP conservan `code`, `recoverable`, `details`, `next_action` y la
      ruta del artefacto de recuperación de un error de rollback.
- [ ] occurrence.
- [ ] métricas.
- [ ] cada ejecución de job registra `job_pending`, `job_running` y exactamente
      un evento terminal (`job_succeeded` o `job_failed`) con el mismo `run_id`.
- [ ] los payloads `job_*` respetan el allow-list del engine y no contienen
      texto de transcript, argumentos de tools ni `OutputHint`.
- [ ] la migración 008 agrega taxonomía a `job_registry` y `job_run_log` de
      forma idempotente en SQLite real.
- [ ] `experience scan --source=<opencode|claudecode|codex>` conserva byte por
      byte el envelope JSON del formulario legacy para la misma entrada.

## F. Seguridad

- [ ] path traversal bloqueado.
- [ ] symlink escape bloqueado.
- [ ] comandos sin shell.
- [ ] secrets redacted.
- [ ] la redacción ocurre **antes** de cualquier persistencia, no a la salida: un secreto entregado en un registro de evidencia no aparece en SQLite, ni en el blob store, ni en el Markdown, ni en el audit log, ni en la respuesta JSON de CLI o MCP.
- [ ] `internal/evidence` está invocado desde una ruta de producción: `evidence.Redact` se ejecuta en la captura real, no solo en sus propias pruebas.
- [ ] changed target bloquea apply.
- [ ] shared/AGENTS requiere humano.
- [ ] archivos sucios bloqueados por defecto.
- [ ] no acceso directo a DB Engram.
- [ ] no telemetría.

## G. Integración

- [ ] funciona sin Engram.
- [ ] busca Engram por HTTP cuando disponible.
- [ ] degradación observable.
- [ ] funciona sin Gentle-AI.
- [ ] refresca skill registry cuando disponible.
- [ ] no modifica archivos administrados.
- [ ] Codex MCP se registra sin duplicados.
- [ ] backup de config Codex.

## H. Instalación

- [ ] install Windows.
- [ ] install Linux/macOS.
- [ ] PATH.
- [ ] version.
- [ ] doctor.
- [ ] uninstall conserva datos.
- [ ] purge requiere flag.
- [ ] scripts idempotentes.

## I. Documentación

- [ ] README real.
- [ ] configuración documentada.
- [ ] MCP tool reference generada.
- [ ] ejemplos reproducibles.
- [ ] threat model.
- [ ] release instructions.
- [ ] final implementation report.

## J. Demo obligatoria

Codex debe adjuntar salida de:

```bash
royo-learn e2e --temp --json
```

que demuestre:

```json
{
  "capture": "passed",
  "curate": "passed",
  "approval_block": "passed",
  "publish": "passed",
  "verification": "passed",
  "occurrence": "passed",
  "rollback": "passed",
  "integrity": "passed"
}
```

## Condición de rechazo

No se acepta como terminado si:

- faltan tools;
- una publicación puede ocurrir sin preview;
- se autoaprueba AGENTS/shared;
- Engram es dependencia obligatoria;
- el binario necesita Node/Python;
- no existe instalador Windows;
- hay TODOs en rutas críticas;
- los tests e2e están simulados sin filesystem/SQLite real.


## K. Experiencia Hito 1

- [ ] un envelope válido crea sesión y turno;
- [ ] el reintento exacto no duplica;
- [ ] una revisión nueva actualiza el turno de forma segura;
- [ ] secretos no llegan a SQLite ni auditoría;
- [ ] el cursor solo avanza tras un commit exitoso;
- [ ] `experience inject` conserva JSON en stdout y errores en stderr.


## L. Hito 12 — drift detection + release

### Drift checker (four outcomes)

- [ ] `Checker.Check` returns exactly four outcomes: `ok`, `drifted`,
  `target_missing`, `target_unreadable`.
- [ ] `ok` outcome: target matches the recorded fingerprint.
- [ ] `drifted` outcome: target exists and is readable but fingerprint
  differs from the recorded one.
- [ ] `target_missing` outcome: target no longer exists at the recorded path.
- [ ] `target_unreadable` outcome: target exists but cannot be read
  (permissions, I/O error); the gate fails closed.
- [ ] `publication_drift_check` job is wired into the publish path with the
  four outcomes surfaced in the job event payload.
- [ ] drift repository records the observation (`RecordDrift`) and lists
  recent observations (`ListDrift`).

### Unified CLI/MCP envelope

- [ ] CLI `publish drift-check` and MCP `publish_drift_check` return the
  same JSON envelope shape: `outcome`, `learning_id`, `target_path`,
  `expected_fingerprint`, `actual_fingerprint`, `checked_at`.
- [ ] CLI flag `--json` is honored; human-readable output is the default.
- [ ] MCP error responses use the project's typed error envelope
  (`code`, `recoverable`, `details`, `next_action`).

### Cross-adapter drift policy parity

- [ ] Claude Code, Codex, and OpenCode adapters report the same four
  outcomes for the same input.
- [ ] `Job()` accessors on each adapter surface the publication drift
  check with identical payload contract.
- [ ] `SkippedIncomplete` parity: incomplete turns are counted in all
  three adapters.

### SBOM emission

- [ ] `.goreleaser.yml` carries the `sboms:` block with
  `formats: ['spdx-json']`.
- [ ] `goreleaser release --snapshot --clean` produces `*.spdx.json` files
  in `dist/` next to `*.tar.gz` / `*.zip`.
- [ ] `tests/release/goreleaser_snapshot_test.go` compiles and runs
  (skips when `goreleaser` is not on PATH).

### CHANGELOG backfill

- [ ] Hitos 8, 9, 10, 11 appear under `[0.8.0]`, `[0.9.0]`, `[0.10.0]`,
  `[0.11.0]` respectively.
- [ ] Each entry carries a `[^pr-N]: #N` footnote linking to the GitHub PR.
- [ ] `[Unreleased]` contains no Hito 8/9/10/11 entries.
- [ ] `v1.0.0` ⏳ marker is demoted to a "no tag yet" section that lists
  the Hito 12 preconditions and links to `RELEASE.md`.
- [ ] No ISO-8601 date appears under the `v1.0.0` heading.
