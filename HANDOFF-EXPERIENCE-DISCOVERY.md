# HANDOFF — Royo-Learn experience discovery (Hito 1 mergeado a `main` localmente)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar nada.
> Cierra la sesión que dejó Hito 1 mergeado localmente a `main` (slice 1.D + flake isolation + cierre + corrección post-review) en `b105e34`. Push y PR no ejecutados; espero decisión humana.

## 0. Frase para pegar en la próxima sesión

Copiá y pegá esto tal cual al iniciar la próxima sesión:

```text
Continuá Royo-Learn en `main` local, donde quedó mergeado
`feat/experience-hito1-1d` como commit `b105e34`
(merge --no-ff de los 18 commits sobre `4fe9774`). Antes de actuar:

1. Leé HANDOFF-EXPERIENCE-DISCOVERY.md completo (incluida esta sección 0).
2. Leé docs/26-IMPLEMENTATION-ROADMAP.md y PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md
   desde el commit histórico `d812709` (no viven en main por la grieta
   documental pre-existente).
3. Leé docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md (status: Proposed) para
   entender el seguimiento del timeout MCP.
4. Verificá el estado real con
   `git log --oneline -5 main`,
   `git status --short --branch`,
   `git rev-list --count 4fe9774..main`,
   `ls .git/gentle-ai/review-transactions/v2/`.

Estado: Hito 1 mergeado localmente a `main` (`b105e34`, 19 commits adelante
de `origin/main`). 18 commits atómicos en la rama + merge --no-ff, con:
slice 1.D (CLI `experience inject`, `ExperienceConfig` deshabilitado por
defecto, acceptance suite, docs) + flake isolation (buildinfo
`//go:build !windows`, TestRunPreviewEndToEnd cleanup, patrón
`testutil.TempDir` aplicado a múltiples paquetes) + cobertura
`internal/experience` 85.3 % → 90.0 % + cierre de Hito 1 (gate -race
`./...` con timeout ortogonal MCP documentado) + corrección post-review
(lineage `experience-hito1-1d-correction-v1` aprobada: `cmd/royo-learn/experience_test.go`
usa `testutil.TempDir(t)`, comentario de cleanup corregido, y
`docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` nuevo). Push y PR no
ejecutados.

Working tree: solo el untracked preexistente
`PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`.

Tareas pendientes para la próxima sesión, en este orden:

1. (Opcional) commit chico en main para refrescar HANDOFF-EXPERIENCE-DISCOVERY.md
   y reflejar el conteo real (18 commits en rama + 1 merge = 19 ahead de
   origin/main). Si lo hacés, usar `docs: refresh Hito 1 handoff` y NO
   pushear.
2. Push explícito de `main` y apertura de PR solo cuando el usuario lo
   pida. `main` está 19 commits adelante de `origin/main`.
3. Ejecutar el §4 de `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` (repro
   base vs candidata, perfil startup/teardown, evidencia focalizada)
   antes de Hito 2; al cerrar, actualizar `docs/IMPLEMENTATION-NOTES.md:466`.
4. PR chico desde `d812709` a `main` para traer `docs/20-26` +
   `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` y cerrar la grieta
   documental pre-existente.
5. Hito 2 (OpenCode `--once`) solo después de los puntos anteriores.

Reglas innegociables (recordatorio):
- TDD estricto: RED primero, GREEN después, refactor sólo con tests verdes.
- Redacción antes de hash y persistencia.
- Reusar `capture.Service` e `internal/evidence`; no duplicar lógica.
- SQLite = verdad operacional.
- Sin Python/Bash/shell; sin red obligatoria; sin daemon.
- Un hito por PR; commits por unidad de trabajo; conventional commits
  sin `Co-Authored-By` ni atribución de IA.
- No pushear ni abrir PR sin que el usuario lo pida explícitamente.
- ADR para el flake `internal/mcpserver` `ListTools: context deadline
  exceeded` ya está creado (ADR-0002, Proposed); no tocar
  `internal/mcpserver` hasta cerrarlo.
```

---

## 1. Qué cambió en esta sesión

- Verifiqué el handoff original, el roadmap y el plan maestro en
  `d812709`. Detecté dos discrepancias con la versión anterior del
  handoff: (a) el conteo real de commits sobre `4fe9774` es 18, no 15;
  (b) `HANDOFF-EXPERIENCE-DISCOVERY.md` ya está trackeado en la rama.
- Lancé la 4R sobre el árbol entregado en worktree aislado
  (`experience-hito1-1d-delivered-v1`, high tier). Risk sin findings.
  Refuter dejó el timeout MCP en `inconclusive` por falta de
  causalidad independiente. La review se cerró como `escalated` y
  paré el merge forzado.
- Implementé la corrección autorizada:
  - `cmd/royo-learn/experience_test.go` ahora usa `testutil.TempDir(t)`.
  - `cmd/royo-learn/main_test.go` corrige el comentario engañoso del
    sleep de Windows (Windows-only; Unix no duerme).
  - `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` nuevo (Proposed), acotando
    la investigación sin tocar `internal/mcpserver`.
  - `docs/IMPLEMENTATION-NOTES.md:466` ahora apunta al ADR.
- Revisión de corrección `experience-hito1-1d-correction-v1` (lente de
  reliability, tier medio, 4 paths) sin findings; pre-commit `allow`.
- Commit `f989579 fix(test): correct Hito 1 cleanup and bound MCP
  timeout follow-up` en la rama.
- `git merge --no-ff feat/experience-hito1-1d` → `b105e34` en `main`.
- No push, no PR.

## 2. Invariantes innegociables (recordatorio)

- Go + SQLite como núcleo; SQLite es la verdad operacional.
- Redacción **antes** de hash y persistencia.
- Experiencia observada ≠ conocimiento aprobado.
- Promoción únicamente vía `capture.Service`.
- Sin Python/Bash/`os.system`/shell; sin red obligatoria; sin daemon.
- Preservar CLI/MCP actuales, JSON estable, Windows/Linux/macOS.
- Preview hash, aprobación, publicación atómica, verificación,
  rollback intactos.

## 3. Estado actual del repositorio

- `main` HEAD local: `b105e34` (merge --no-ff de
  `feat/experience-hito1-1d` HEAD `f989579`).
- `main` vs `origin/main`: ahead 19 commits. No pusheado.
- `feat/experience-hito1-1d` HEAD: `f989579`. Queda en local.
- Working tree: solo `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked
  (intencionalmente fuera del merge; preservado).
- `HANDOFF-EXPERIENCE-DISCOVERY.md` ahora está tracked y commiteado
  en `main` (entró con el merge de Hito 1).
- PR #17: cerrado y mergeado (Hito 1 1.A-1.C).
- Issue #16: cerrado, estado `completed`.
- `feat/experience-hito1-domain` queda en `origin` con los 8 commits
  originales por trazabilidad (no se borró).
- Review transactions nuevas en
  `.git/gentle-ai/review-transactions/v2/`:
  - `experience-hito1-1d-delivered-v1` (high tier, 4R, escalated).
  - `experience-hito1-1d-correction-v1` (medium tier, reliability,
    approved).
  - `experience-hito1-1d-merge-v1` (low tier, escalated por scope
    incorrecto contra un untracked, contenido en incidente
    controlado).

## 4. Cobertura Hito 1

### 4.1. En `main` antes del merge (`4fe9774`, Hito 1 slices 1.A-1.C)

- Validación de esquema/proyecto/locator con errores tipados seguros
  (`ErrExperience*` con exit-code estable).
- Límites de bytes para turn, cursor, locator, IDs, actor, project
  root, source instance.
- Redacción endurecida: cookies, `<private>`, `ENCRYPTED PRIVATE KEY`,
  asignaciones con comillas/colon.
- JSON determinista con `json.Number`, colisión de keys detectada,
  fingerprint estable sobre digests redacted.
- Migración 004 con `source_order`, FKs activos y checksum.
- Repository atómico: datos, auditoría y cursor éxito comparten
  una sola transacción.
- Cursor con CAS por `source_order` y revision.
- Idempotencia por identidad externa + fingerprint + `source_revision`.
- `busy_timeout` por conexión pooled.
- `experience_commit_unknown` tipado sin contradecir la metadata de fallo.
- Auditoría append-only.

### 4.2. Delta de `feat/experience-hito1-1d` (slices 1.D + flake isolation + cierre + corrección)

- `ExperienceConfig` deshabilitado por defecto en
  `internal/config/config.go`.
- CLI `experience inject` en `cmd/royo-learn/experience.go` (lee envelope
  JSON de `--envelope <path>`, llama a `experience.Service.IngestEnvelope`,
  stdout JSON, stderr error envelope).
- Acceptance suite (`internal/experience/acceptance_test.go`):
  5 tests cubriendo los criterios Hito 1.
- Cobertura `internal/experience` 85.3 % → 90.0 % con tests focalizados
  (`internal/experience/coverage_test.go`).
- Flake `internal/buildinfo` aislado con `//go:build !windows`.
- Flake `TestRunPreviewEndToEnd` mitigado con `t.Cleanup` + espera
  150 ms en Windows.
- Patrón `testutil.TempDir(t)` aplicado a 8 archivos adicionales
  (internal/setup, internal/selfupdate, internal/storage, y
  cmd/royo-learn/{mcp,evidence_cli}_test.go).
- Coherencia documental: HANDOFF, IMPLEMENTATION-LOG,
  IMPLEMENTATION-NOTES, docs 03/04/05/14/17.
- `cmd/royo-learn/experience_test.go` usa `testutil.TempDir(t)`
  (corrección post-review; antes usaba `t.TempDir()` y podía
  reintroducir el flake en Windows).
- Comentario de cleanup en `cmd/royo-learn/main_test.go` corregido
  para describir el sleep Windows-only fielmente.
- `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` (Proposed): acota la
  investigación del timeout MCP sin tocar `internal/mcpserver`.

## 5. Hoja de ruta (resumen; detalle en `docs/26-IMPLEMENTATION-ROADMAP.md`)

Ola 1 (un PR por hito):

- **0 ✅** docs 20-26 + ADR-0001 (viven solo en `d812709`; grieta
  documental sin resolver).
- **1 ✅** Hito 1 mergeado localmente a `main` en `b105e34`; pendiente
  push + PR.
- 2 OpenCode `--once` (bloqueado por ADR-0002).
- 5 detectores deterministas.
- 6 patrones + migración 005.
- 7 promoción vía `capture.Service`.
- 4 trace.

## 6. Próximo trabajo

### 6.1. (Opcional) Refresh del HANDOFF

El handoff describe "15 commits" pero la realidad es 18 en la rama + 1
merge. Un commit chico en main actualizando la sección 0 y la sección 3
lo deja consistente para la próxima sesión. No urgente.

### 6.2. Push y PR explícitos

`main` está 19 commits adelante de `origin/main`. No pushear hasta que
el usuario lo autorice explícitamente. Si abre PR, base `main`, head
`main` local; título y cuerpo a definir al momento.

### 6.3. ADR-0002: ejecutar §4 antes de Hito 2

El ADR-0002 (`docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md`) exige, antes de
Hito 2:

1. Perfil startup/teardown de `internal/mcpserver` (wall-clock,
   allocations, goroutines) en Windows y Linux.
2. Repro base vs candidata con
   `go test -race -count=10 ./internal/mcpserver/`.
3. Bisección entre `4fe9774` y `f989579` para localizar cambios.
4. Reporte con nombre/archivo:line, comando exacto, perfil, versión
   Go, evidencia en cliente real `mcp-serve`, propuesta de remediación
   evaluada contra §2.2 del ADR.

Al cerrar, actualizar `docs/IMPLEMENTATION-NOTES.md:466` para apuntar
a la resolución.

### 6.4. PR chico de `docs/20-26` + `PLAN-MAESTRO`

`docs/20-26` + `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` viven solo en
`d812709`. PR chico desde `d812709` a `main` para traerlos y cerrar
la grieta de coherencia documental pre-existente.

### 6.5. Hito 2 (OpenCode `--once`)

Una vez cerrados los puntos 6.3 y 6.4, arrancar Hito 2 según `docs/26`
§3. No tocar MCP en este hito.

## 7. Reglas operativas (recordatorio)

- Un hito por PR; commits por unidad de trabajo; conventional,
  sin atribución de IA.
- No pushear ni abrir PR sin que el usuario lo pida explícitamente.
- Antes de cada slice: objetivo, invariantes, fuera de alcance,
  archivos, migraciones, riesgos, acceptance.
- Reutilizar servicios; no duplicar reglas. Verificar nombres
  reales antes de crear archivos (no asumir paths del plan).
- Regla de parada (abrir ADR y detener): necesidad de transcript
  completo; endpoint remoto obligatorio; credenciales no previstas;
  cambio de formato upstream; semántica que exige CGO; config de
  proyecto que amplía trust roots; un job que podría publicar;
  modificar estado de `Learning` fuera de sus servicios.

## 8. Referencias clave

- `docs/20`–`docs/26` — contratos congelados + roadmap (viven en
  `d812709`).
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` — decisión anti-MemSearch.
- `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` — Proposed; acota la
  investigación del timeout MCP antes de Hito 2.
- `docs/26-IMPLEMENTATION-ROADMAP.md` (en `d812709`, no en main).
- `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` en commit histórico
  `d812709`.
- Código a reutilizar:
  - `internal/capture/capture.go` (`Service.Capture`).
  - `internal/evidence/` (redacción, runner sin shell).
  - `internal/storage/` (migrate.go, repo_experience.go).
  - `internal/domain/` (types, errors, validation).
  - `internal/testutil/tempdir.go` (`testutil.TempDir(t)` con
    `RemoveAllWithRetry` interno — patrón para evitar flakes de
    cleanup en Windows).
- Commits squash mergeados: `4fe9774` en `main` (Hito 1 1.A-1.C).
- Merge local de Hito 1: `b105e34` en `main` (no pusheado).
- Reviews:
  - `.git/gentle-ai/review-transactions/v2/experience-hito1-1d-delivered-v1/`
    (high tier 4R, escalated).
  - `.git/gentle-ai/review-transactions/v2/experience-hito1-1d-correction-v1/`
    (medium tier, reliability, approved).
- PR/issue de referencia: #17 / #16 (Hito 1 1.A-1.C).
- Pre-existing untracked: `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`
  (preservado fuera del merge).
