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

| PR | Hito | Entrega | Migración | Estado |
|----|------|---------|-----------|--------|
| 8 | 8 | motor de jobs (lease SQLite, digest, run-due, retry, crash recovery) | **007** | ✅ COMPLETO (PR #26) |
| 9 | 9 | retrieval lexical + score components + saneamiento FTS | — | ✅ COMPLETO (`c24fed5`) — cobertura 89.2%, benchmark 7ms |
| 10 | 3 | OpenCode automático (`--watch`, setup preview/apply/remove) | — | ❌ PENDIENTE (opcional) |
| 11 | 10 | Claude Code (JSONL) — PR propio | — | ✅ COMPLETO (PR #11) |
| 12 | 10 | Codex (rollout) — PR propio, no fusionar con Claude Code | — | ✅ COMPLETO (PR #12) — cobertura 94.4% |

## 5. Ola 3 — optimización (con gate previo obligatorio)

| PR | Hito | Gate previo | Estado |
|----|------|-------------|--------|
| 13 | 12 drift/release hardening | — | 🚧 EN VUELO (PRs #13a/#13b/#13c, `feat/hito12-release-extras`) — Slice 1/2 merged a `main`; Slice 3 (SBOM + RELEASE.md + CHANGELOG backfill) en este PR |
| 14 | 11 semántica opcional | informe que demuestre consultas donde lexical falla + mejora medible + rebuild fiable | ✅ COMPLETO (`feat/hito11-semantic`) |
| — | Pi | documentar fuente, fixtures reales, ADR de estabilidad de formato | ❌ PENDIENTE |

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

### Ola 1 — COMPLETA ✅

- **Hito 0: COMPLETO** — docs 20–26 + ADR-0001.
- **Hito 1: COMPLETO** — dominio experiencia, migración 004, servicio
  ingestión, CLI `experience inject`. Cobertura `internal/experience` 90%.
- **Hito 2: COMPLETO** (PR #3) — adaptador OpenCode read-only (`--once`).
  Cobertura `internal/experience/opencode` 80.5%.
- **Hito 5: COMPLETO** (PR #4, `59d5e74`) — detectores deterministas,
  persistencia, CLI/MCP surface.
- **Hito 6: COMPLETO** (PR #5, `55ef635`) — pattern mining, qualification,
  dismissal, CLI/MCP surface. Migración 005.
- **Hito 7: COMPLETO** — promoción gobernada vía `capture.Service`,
  idempotencia, redaction pipeline.
- **Hito 4: COMPLETO** (PR #25) — trace progresivo, migración 006,
  `learning_trace` MCP + CLI. Cobertura 83.6%.

### Ola 2 — mayormente completa

- **Hito 8: COMPLETO** (PR #26, `81605f7`) — lease-based job engine,
  migración 007.
- **Hito 9: COMPLETO** (`c24fed5`) — retrieval lexical, score components,
  sanitización FTS5. Cobertura 89.2%, benchmark 7ms.
- **Hito 10: COMPLETO** — Claude Code (PR #11) + Codex (PR #12),
  simétricos. Cobertura Codex 94.4%.
- **Hito 3: PENDIENTE** — OpenCode `--watch` (opcional).

### Ola 3 — en progreso

- **Hito 11: COMPLETO** (`feat/hito11-semantic`, mergeado a `main`) —
  motor de jobs semántico, `Job()` accessors, CLI collapse.
- **Hito 12: EN VUELO** — drift/release hardening. Slices 1/2 merged a `main`
  (publication drift checker con cuatro outcomes, unified CLI/MCP
  envelope, adapter parity). Slice 3 (PR #13c) ships preconditions for
  `v1.0.0`: SBOM en `.goreleaser.yml`, `RELEASE.md` runbook, CHANGELOG
  backfill. See `feat/hito12-release-extras`.
- **Pi adapter: PENDIENTE** — documentar fuente, fixtures reales, ADR.

**Próximo: Hito 12 Slice 3** — PR #13c en este change. Tras merge, el
operador decide cuándo taggear `v1.0.0` siguiendo `RELEASE.md`.

## 9. Lecciones aprendidas en Hito 2 (para PR #3 y siguientes)


- **`SkippedIncomplete` debe ser visible al operador.** El adapter
  originalmente descartaba turnos con `complete=0` en silencio. Mientras
  los tests del adapter los cubrían como "no se ingestaron", el CLI no
  podía reportar la pérdida al usuario. La métrica se agregó al
  `ScanResult` (`SkippedIncomplete int`) y se propagó al reporte JSON.
  Lección: cualquier drop silencioso del adaptador necesita un contador
  expuesto al caller.
- **Los fixtures de tests deben vivir dentro del `projectRoot`.** El
  validador de envelopes rechaza locators fuera del project root
  canónico (`experience_locator_outside_root`). El primer corte de
  tests del CLI creaba el fixture en un `t.TempDir()` separado y el
  scan fallaba al ingestar. Solución: el fixture va en
  `root/.opencode-fixture/opencode.db`. Aplica a cualquier test que
  use `--fixture` o discovery automático.
- **El CLI necesita su propia capa de tests, no solo la del adapter.**
  El adapter tenía cobertura completa de Discover/Health/Scan/
  ResolveTrace, pero el subcomando `experience opencode scan` no
  tenía tests hasta slice 2.6. La cobertura del adapter mide
  unidades, no la orquestación. Patrón: una vez que un método del
  adapter se enchufa a un comando CLI, ese comando necesita su
  tabla de tests propia.
- **`cursorCheckpoint` debe aceptar los 4 tipos numéricos.** El
  decoder depende de cómo se construyó el mapa: nativo Go, JSON
  round-trip, sub-agentes externos. El test cubre `int64`/`int`/
  `float64`/`int32` explícitamente; un futuro refactor que asuma
  un solo tipo lo rompería.
