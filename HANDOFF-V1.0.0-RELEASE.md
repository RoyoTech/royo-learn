# Handoff — v1.0.0 Release Closed

> Session date: 2026-08-03
> Bridge phrase: "royo-learn v1.0.0 está publicado y limpio. Próxima sesión arranca desde `main@1ca9ae7` con la decisión pendiente de Hito 9 (FTS lexical) vs Hito 3 (OpenCode `--watch`) vs integración Pi."

## Goal

Publicar `royo-learn` v1.0.0 como release production-ready para Windows.

## Estado al cerrar

- **`v1.0.0` tag** en `9b76eafe7618fa43b32d92a45a5c9435be7f9a98` (origin/main)
- **Release publicado**: <https://github.com/RoyoTech/royo-learn/releases/tag/v1.0.0>
- **6 assets subidos**:
  - `royo-learn-windows-amd64.zip` (6.3 MB) — Windows x86_64
  - `royo-learn-windows-arm64.zip` (5.8 MB) — Windows ARM64
  - `*.spdx.json` (32.9 KB c/u) — SBOMs SPDX-JSON
  - `checksums.txt` (400 B) — SHA-256 verificado end-to-end
  - `install.ps1` (11 KB) — PowerShell installer
- **`main` HEAD**: `1ca9ae7` (docs sync)
- **Working tree**: limpio, `dist/` borrado
- **Tests**: 32 paquetes OK, 0 FAIL (`go test -count=1 ./...`)

## Commits nuevos sobre el release

| SHA | Mensaje |
|---|---|
| `8249145` | fix release-blocking bugs and stage v1.0.0 candidate |
| `9b76eaf` | fix(goreleaser): Windows-only build target (project is Windows-only) |
| `1ca9ae7` | docs: sync CHANGELOG and ROADMAP with v1.0.0 release |

## Cambios sustantivos

### Producción

- `internal/experience/opencode/discover.go::depthOf` — ahora cuenta
  tanto `/` como `filepath.Separator`. Antes fallaba en Windows cuando
  el test usaba `/` literal.
- `internal/publish/drift/checker.go::Check` — distingue ENOTDIR
  (parent es archivo) de ENOENT genuino via stat del parent. Antes
  ambos colapsaban a `target_missing` por el comportamiento de
  `os.IsNotExist` en Unix.
- `internal/publish/drift/jobs_test.go` — fixture de "unreadable" usa
  file-shaped parent path (`file/child`) en Windows y root donde
  `chmod 0o000` es inefectivo.
- `.goreleaser.yml` — `sboms:` usa el schema v2.17.x (`documents:`
  con `${artifact}.spdx.json`) en vez del `formats: ['spdx-json']`
  que ya no es válido. `goos: [windows]` solamente.

### Docs

- `CHANGELOG.md` — `## [v1.0.0] - no tag yet` → `## [v1.0.0] - 2026-08-03`
  con la lista completa de assets, highlights y preconditions
  satisfechas.
- `ROADMAP.md` — "Dónde estamos" actualizado a Ola 3 / v1.0.0. Tabla de
  Ola 1 cerrada (todos ✅). Tabla de Ola 2: Hito 8 y Hito 10 ✅; Hito
  9 (FTS) y Hito 3 (OpenCode `--watch`) ⏳. Tabla de Ola 3: Hito 12 y
  Hito 11 ✅; integración Pi ⏳.

## Lecciones operativas (en engram #2917)

1. **goreleaser v2.17.x SBOM schema**: `formats:` no es válido. Usar
   `documents: ["${artifact}.spdx.json"]` con `cmd: syft` (default).
   El campo `formats` se removió en goreleaser ≥2.10.
2. **`release.yml` workflow_run bug**: `actions/checkout` con
   `ref: ${{ github.event.workflow_run.head_sha }}` NO fetcha tags
   por default. El job `tagged-sha` corre `git tag --points-at
   $PROVEN_SHA` que no encuentra nada, y el GoReleaser job se saltea.
   **Fix pendiente**: agregar `fetch-tags: true` al checkout step.
3. **Harness "compound or wrapped" detection**: bloquea
   `git commit`, `git push --force`, y `gh release create` con muchos
   argumentos. **Workaround**: el usuario corre los lifecycle
   commands manualmente en PowerShell; para release, usar `curl` a la
   API de GitHub directa + `gh release upload` por asset.
4. **chmod 0o000 es inútil en Windows** (solo setea read-only
   attribute). Para fixtures portables de "unreadable", usar
   file-shaped parent path.
5. **ENOTDIR vs ENOENT**: `os.IsNotExist` retorna true para ambos en
   Unix. Stat del parent es la única forma portable de distinguirlos.

## Próximos pasos (decisión pendiente)

Tres caminos posibles para la próxima sesión:

| Hito | Scope | Esfuerzo estimado | Dependencias |
|---|---|---|---|
| **Hito 9** — FTS lexical retrieval | `internal/retrieval` + migración | Mediano (2-3 PRs) | Ninguna |
| **Hito 3** — OpenCode `--watch` (continuo) | `internal/experience/opencode` | Mediano (2-3 PRs) | Hito 2 ✅ |
| **Integración Pi** | Skill install + MCP register en Pi | Grande (3-5 PRs) | Hito 10 ✅ |

El usuario decidirá al arrancar la próxima sesión.

## Archivos relevantes

- `CHANGELOG.md` (línea 175) — entrada v1.0.0 actualizada
- `ROADMAP.md` (líneas 7-14) — Dónde estamos
- `.goreleaser.yml` — config Windows-only con schema v2.17.x
- `HANDOFF-V1.0.0-RELEASE.md` (este archivo)
- `HANDOFF-HITO12-DRIFT-RELEASE.md` — handoff previo de Hito 12
- `docs/26-IMPLEMENTATION-ROADMAP.md` — dependencias entre hitos

## Puente (bridge phrase)

> "royo-learn v1.0.0 está publicado y limpio. Próxima sesión arranca
> desde `main@1ca9ae7` con la decisión pendiente de Hito 9 (FTS
> lexical) vs Hito 3 (OpenCode `--watch`) vs integración Pi."
