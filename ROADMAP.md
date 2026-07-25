# ROADMAP — Royo-Learn por olas

> Mini-roadmap vivo. Refrescar al cerrar cada hito. La fuente de verdad
> completa es `CHANGELOG.md` (Version ↔ Ola map + Trigger → tag) y
> `docs/26-IMPLEMENTATION-ROADMAP.md` (dependencias entre hitos).

## Dónde estamos

- **Ola actual:** Ola 1 (v0.2.0)
- **Último hito cerrado:** Hito 2 — OpenCode `--once` mergeado a `main`
  como PR #21 (squash commit `ad269a7`).
- **Próximo tag candidate:** `v0.2.0` cuando mergee el PR de Hito 4
  (último de Ola 1) a `main` remoto.

---

## Ola 1 → v0.2.0 — Experience loop end-to-end

*"El momento en que un agente puede hacer el loop completo de
experiencia: capturar → validar → detectar → clusterizar → promover →
trazar."* (PLAN-MAESTRO §37)

| # | Hito | PR | Estado | Rama / commit |
|---|---|---|---|---|
| 0 | Contratos docs/20-26 + ADR-0001 | #19 | ✅ merged | `docs/grieta-20-26-clean` |
| 1 | Experience discovery | #17 | ✅ merged en `b105e34` | `feat/experience-hito1-1d` |
| 2 | OpenCode `--once` (adaptador read-only) | #21 | ✅ merged en `ad269a7` | `feat/hito2-opencode-once` (squashed, rama borrada) |
| 5 | Detectores deterministas | TBD | ⏳ | — |
| 6 | Patrones + clustering + migración 005 | TBD | ⏳ | — |
| 7 | Promoción vía `capture.Service` | TBD | ⏳ | — |
| 4 | Trace progresivo (requiere 1 + 7) | TBD | ⏳ | — |

**Trigger → tag `v0.2.0`:** merge del PR de Hito 4 a `origin/main`.

**Sub-slices de Hito 2 (referencia, ya shipped):** 2.0 scaffold ·
2.1 discover · 2.2 health · 2.3 scan · 2.4 idempotencia · 2.5 resolveTrace
· 2.6 CLI `experience opencode scan --once` · 2.7 acceptance + docs.

**Lecciones operativas del PR #21** (registradas en `docs/26 §9`):

- `SkippedIncomplete` debe ser visible al operador (no drop silencioso)
- Los fixtures de tests deben vivir dentro del `projectRoot`
- El CLI necesita su propia capa de tests, no solo la del adapter
- `cursorCheckpoint` debe aceptar los 4 tipos numéricos

---

## Ola 2 → v0.3.0 — Robustez + multi-agente

*"La API operativa cambia: motor de jobs con lease, FTS para búsqueda
lexical, adaptadores multi-agente."* Por eso minor, no patch.

| # | Hito | PR | Estado |
|---|---|---|---|
| 8 | Motor de jobs (lease-based) + migración 006 | TBD | ⏳ |
| 9 | Retrieval lexical (FTS5) | TBD | ⏳ |
| 3 | OpenCode `--watch` (continuo, depende de 2) | TBD | ⏳ |
| 10 | Adaptador Claude Code / Codex | TBD | ⏳ |

**Trigger → tag `v0.3.0`:** merge del PR de Hito 10 a `origin/main`.

---

## Ola 3 → v1.0.0 — Production-ready

*"El momento en que el proyecto se compromete con un contrato estable
para equipos externos. Breaking changes después de acá requieren
major."*

| # | Hito | PR | Estado |
|---|---|---|---|
| 12 | Drift + release hardening | TBD | ⏳ |
| 11 | Semántica opcional (embeddings, opt-in) | TBD | ⏳ |
| — | Integración Pi | TBD | ⏳ |

**Trigger → tag `v1.0.0`:** merge del PR de Pi a `origin/main`.

---

## Cómo mantener este archivo

Al cerrar un hito:

1. Cambiar ⏳ a ✅ y anotar el PR + merge commit en la columna.
2. Si era el último de la ola, mover "Ola actual" a la siguiente.
3. Si el slice del hito 2 que estabas en curso terminó, marcar ✅ y
   pasar al próximo hito de la lista.
4. No reescribir las definiciones de las olas — esas viven en el
   CHANGELOG. Acá solo sigue el estado.
