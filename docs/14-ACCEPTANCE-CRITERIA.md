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
- [ ] **tras un `publish` exitoso, el registro Markdown refleja el estado
      `published` de SQLite: el `doctor` queda limpio sin ejecutar
      `rebuild-index`** (D18).
- [ ] **tras un `rollback` exitoso, el aprendizaje NO sigue en `published`:
      vuelve a `approved` y el `doctor` queda limpio** (D18).
- [ ] **un `rollback` fallido no toca el estado del aprendizaje** (D18).
- [ ] occurrence.
- [ ] métricas.

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
