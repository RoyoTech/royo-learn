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

- [ ] capture idempotente.
- [ ] búsqueda previa.
- [ ] curación con estados válidos.
- [ ] needs_evidence.
- [ ] merge.
- [ ] reject.
- [ ] approve.
- [ ] preview.
- [ ] approval ligada a hash.
- [ ] publish.
- [ ] verify.
- [ ] rollback.
- [ ] occurrence.
- [ ] métricas.

## F. Seguridad

- [ ] path traversal bloqueado.
- [ ] symlink escape bloqueado.
- [ ] comandos sin shell.
- [ ] secrets redacted.
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
