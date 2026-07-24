# Ruta de implementación — Capa de descubrimiento

- **Estado:** gobernante (define el orden de ejecución)
- **Regla base:** un hito por PR; no se avanza con gates pendientes; TDD estricto.
- **Fuente de hitos:** `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` §22, §30.
- **Contratos congelados:** `docs/20`–`docs/25`, `docs/ADR-0001`.

## 1. Principio de secuenciación

Cada PR entrega valor verificable y deja el árbol verde. El orden **no** es
lineal por número de hito: sigue la dependencia real y el valor de producto
(plan §30, Ola 1). Un hito solo empieza cuando el anterior cumple **todos** sus
criterios de aceptación.

## 2. Grafo de dependencias

```text
Hito 0 (docs) ─┐
               ▼
Hito 1 (dominio + migración 004 + servicio ingestión)
   │
   ├──► Hito 2 (OpenCode --once, migración de fixtures)
   │       │
   │       └──► Hito 3 (OpenCode --watch/setup)   [Ola 2, opcional]
   │
   ├──► Hito 5 (detectores deterministas)
   │       │
   │       └──► Hito 6 (patrones + migración 005)
   │               │
   │               └──► Hito 7 (promoción vía capture.Service)
   │                       │
   │                       └──► Hito 4 (trace progresivo)*
   │
   └──► Hito 8 (motor de jobs + migración 006)   [Ola 2]

Hito 9 (retrieval lexical)   [Ola 2, independiente]
Hito 10 (Claude Code / Codex)  [Ola 2, tras congelar OpenCode]
Hito 11 (semántica)  · Hito 12 (drift/release)   [Ola 3, con gate previo]
```

\* Trace (Hito 4) necesita eventos con procedencia (Hito 1) y se vuelve
demostrable en cuanto existe una promoción (Hito 7); puede adelantarse en
paralelo a 5–6 si conviene la demo, pero su e2e cierra tras 7.

## 3. Ruta recomendada por PRs (Ola 1 — el salto de producto)

| PR | Hito | Entrega | Migración | Gate de salida |
|----|------|---------|-----------|----------------|
| 1 | 0 | docs 20–26 + ADR-0001 + updates 01/02/17 | — | revisión documental; build/vet verdes |
| 2 | 1 | dominio experiencia, validación, repos, servicio ingestión, fingerprint, auditoría, CLI fixture | **004** | envelope válido crea sesión/turno; reintento no duplica; secreto no llega a sink; `-race` + cross-build |
| 3 | 2 | adaptador OpenCode read-only `--once`, fixtures SQLite anonimizadas, discovery, estabilidad, cursor, doctor | — | lee fixture; ignora incompletos; reinicio no duplica; cero side effects; path fuera de raíz bloqueado |
| 4 | 5 | detectores deterministas + versiones + job `experience_detect_events` | — | precisión>recall; cero eventos en charla rutinaria; determinista |
| 5 | 6 | patrones, clustering lexical, cualificación, dismissal, listado/get | **005** | 3 sesiones cualifican; 3 reintentos no; contradicción bloquea; miembros trazables |
| 6 | 7 | `learning_promote_pattern` vía `capture.Service`, evidencia/relaciones, e2e | — | promoción no publica; dedup funciona; idempotente; patrón→promoted |
| 7 | 4 | tabla Learning↔Event, resolver OpenCode, `learning_trace`, CLI `trace` | — | Learning muestra origen; excerpt solo con flag; fuente mutada detectada; <1 MB |

Al cerrar el PR 7 se cumple el "resultado esperado de la Ola 1" del plan §37.

## 4. Ola 2 — robustez y alcance

| PR | Hito | Entrega | Migración |
|----|------|---------|-----------|
| 8 | 8 | motor de jobs (lease SQLite, digest, run-due, retry, crash recovery) | **006** |
| 9 | 9 | retrieval lexical + score components + saneamiento FTS | — |
| 10 | 3 | OpenCode automático (`--watch`, setup preview/apply/remove) | — |
| 11 | 10 | Claude Code (JSONL) — PR propio | — |
| 12 | 10 | Codex (rollout) — PR propio, no fusionar con Claude Code | — |

## 5. Ola 3 — optimización (con gate previo obligatorio)

| PR | Hito | Gate previo |
|----|------|-------------|
| 13 | 12 drift/release hardening | — |
| 14 | 11 semántica opcional | informe que demuestre consultas donde lexical falla + mejora medible + rebuild fiable |
| — | Pi | documentar fuente, fixtures reales, ADR de estabilidad de formato |

## 6. Definición de "hecho" por PR

Todo PR entrega (plan §28.3): objetivo; fuera de alcance; archivos cambiados;
migraciones; contratos nuevos; riesgos; pruebas ejecutadas con resultado;
rollback (considerando migraciones); evidencia de cross-build; actualización
documental; diff contra el plan.

Gates de CI que ningún PR puede saltar: `gofmt`, `go vet`, `go test ./...`,
`go test -race -p 1 ./...`, cross-build windows/linux/darwin, migration tests,
e2e fixtures, security tests, coverage gates. Sin `continue-on-error`.

## 7. Reglas de parada (abrir ADR y detener)

Transcript completo obligatorio; endpoint remoto obligatorio; credenciales no
previstas; cambio de formato upstream; semántica que exige CGO/runtime pesado;
config de proyecto que necesita ampliar trust roots; un job que podría publicar;
modificar estado de `Learning` sin sus servicios; contradicción con una garantía
de publicación existente.

## 8. Estado actual

- **Hito 0: COMPLETO** (este árbol, solo `.md`).
- **Próximo: Hito 1** — dominio y almacenamiento de experiencia, migración 004,
  bajo TDD. Es el primer PR con código y valida los contratos congelados.
