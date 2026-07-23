# HANDOFF — Royo-Learn experience discovery (Hito 1 cerrado en `feat/experience-hito1-1d`)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar nada.
> Cierra la sesión que dejó Hito 1 completo (slice 1.D + flake isolation + cierre) en la sub-rama `feat/experience-hito1-1d`. PR/merge no ejecutado; espero decisión humana.

## 0. Frase para pegar en la próxima sesión

Copiá y pegá esto tal cual al iniciar la próxima sesión:

```text
Continuá Royo-Learn con la sub-rama `feat/experience-hito1-1d` y sus
15 commits sobre `4fe9774`. Antes de actuar:

1. Leé HANDOFF-EXPERIENCE-DISCOVERY.md completo (incluida esta sección 0).
2. Leé docs/26-IMPLEMENTATION-ROADMAP.md y PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md
   desde el commit histórico `d812709` (no viven en main por la grieta
   documental pre-existente).
3. Verificá el estado real con `git log --oneline -3 feat/experience-hito1-1d`,
   `git log --oneline -3 main`, y `git status --short --branch`.

Estado: Hito 1 cerrado en `feat/experience-hito1-1d` (HEAD `076ae7d`).
15 commits atómicos: slice 1.D (4 commits: CLI fixture, ExperienceConfig,
acceptance suite, docs) + flake isolation (buildinfo `//go:build !windows`,
TestRunPreviewEndToEnd cleanup, patrón `testutil.TempDir` aplicado a 6
paquetes) + cobertura `internal/experience` 85.3% → 90.0% + cierre de
Hito 1 (gate -race ./... verde, cross-build windows/linux/darwin verde,
acceptance suite verde). Sub-rama lista para merge; no pusheada.
Working tree: solo el untracked preexistente `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`.

Tareas pendientes para la próxima sesión, en este orden:

1. Decisión de merge: `feat/experience-hito1-1d` → `main` (15 commits
   atómicos) o squash. Opcional: `go test -race -count=2 ./...` dos veces
   para confirmar ausencia de flakes residuales.

2. ADR para el flake `internal/mcpserver` `ListTools: context deadline
   exceeded` (ortogonal, sensible a timeout MCP, no es Windows AV).

3. PR chico de `docs/20-26` + `PLAN-MAESTRO` desde `d812709` a `main`
   para cerrar la grieta de coherencia documental pre-existente.

4. Hito 2 (OpenCode `--once`) cuando se decida entrar a Ola 1 hito 2.

Reglas innegociables (recordatorio):
- TDD estricto: RED primero, GREEN después, refactor sólo con tests verdes.
- Redacción antes de hash y persistencia.
- Reusar `capture.Service` e `internal/evidence`; no duplicar lógica.
- SQLite = verdad operacional.
- Sin Python/Bash/shell; sin red obligatoria; sin daemon.
- Un hito por PR; commits por unidad de trabajo; conventional commits
  sin `Co-Authored-By` ni atribución de IA.
- No pushear ni abrir PR sin que el usuario lo pida explícitamente.
```

---

## 1. Qué cambió en esta sesión

- Implementé slice 1.D en la sub-rama `feat/experience-hito1-1d` desde
  `4fe9774` (main). 4 commits atómicos: `ExperienceConfig`,
  CLI `experience inject`, acceptance suite cubriendo los 5 criterios
  Hito 1, coherencia documental.
- Subí cobertura `internal/experience` de 85.3% a 90.0% con tests
  focalizados en `internal/experience/coverage_test.go` (603 líneas, TDD).
- Aislé el flake `internal/buildinfo` con `//go:build !windows` +
  comentario contractual. CI ya corre -race en Linux only, así que la
  matriz queda limpia.
- Mitigué el flake `TestRunPreviewEndToEnd` con `t.Cleanup` que cierra
  DB idempotentemente + espera 150 ms en Windows antes del `RemoveAll`
  automático de `t.TempDir`.
- Extendí el patrón `testutil.TempDir(t)` a `internal/setup/*`,
  `internal/selfupdate/*`, `internal/storage/db_test.go`,
  `cmd/royo-learn/{mcp,evidence_cli}_test.go` (8 archivos adicionales)
  porque compartían el mismo flake de cleanup en Windows.
- Gate final verde: `go test -race -count=1 ./...` 20 ok / 0 fail,
  cross-build windows/linux/darwin verde, cobertura 90.0%.
- Actualicé `HANDOFF-EXPERIENCE-DISCOVERY.md` con el estado de cierre
  (este commit).
- No se pusheó ni abrió PR. Sub-rama `feat/experience-hito1-1d` lista
  para decisión humana.

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

- Ramas locales:
  - `main` HEAD: `4fe9774` (squash de PR #17 / Hito 1 slices 1.A-1.C).
  - `feat/experience-hito1-1d` HEAD: `076ae7d` (15 commits sobre `4fe9774`).
- Working tree: solo el untracked preexistente
  `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` (intencionalmente fuera del merge).
- `HANDOFF-EXPERIENCE-DISCOVERY.md` también está untracked, intencional
  fuera del merge para mantenerlo como artefacto de continuidad.
- PR #17: cerrado y mergeado (Hito 1 1.A-1.C).
- Issue #16: cerrado, estado `completed`.
- `feat/experience-hito1-domain` queda en `origin` con los 8 commits
  originales por trazabilidad (no se borró; sigue siendo la fuente
  histórica del trabajo aprobado).
- `feat/experience-hito1-1d` queda en local con 15 commits, listos
  para merge a `main` (decisión humana pendiente).

## 4. Cobertura

### 4.1. En `main` (Hito 1 slices 1.A-1.C, merge en `4fe9774`)

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
- Cursor con CAS por `source_order` y revision; rechazo de orden
  menor o mismo orden con digest distinto; recovery exacta.
- Idempotencia por identidad externa + fingerprint + `source_revision`.
- Run del migrador tolerante a aplicación concurrente idéntica y
  estricto ante checksum mismatch.
- `busy_timeout` por conexión pooled.
- Métricas locales (counters, duration) sin red ni daemon.
- `experience_commit_unknown` tipado cuando el commit es ambiguo,
  sin contradecir la metadata de fallo.
- Auditoría append-only: sesión, turno, cursor éxito/fallo, fallo
  de ingestión.

### 4.2. Delta en `feat/experience-hito1-1d` (Hito 1 slice 1.D + flake isolation + cierre, merge pendiente)

- `ExperienceConfig` deshabilitado por defecto en `internal/config/config.go`.
- CLI `experience inject` en `cmd/royo-learn/experience.go` (lee envelope
  JSON de `--envelope <path>`, llama a `experience.Service.IngestEnvelope`,
  stdout JSON, stderr error envelope).
- Acceptance suite (`internal/experience/acceptance_test.go`):
  5 tests cubriendo los criterios Hito 1.
- Cobertura `internal/experience` 85.3% → 90.0% con tests focalizados
  (`internal/experience/coverage_test.go`).
- Flake `internal/buildinfo` aislado con `//go:build !windows`.
- Flake `TestRunPreviewEndToEnd` mitigado con `t.Cleanup` + espera 150 ms.
- Patrón `testutil.TempDir(t)` aplicado a 8 archivos de `internal/setup/*`,
  `internal/selfupdate/*`, `internal/storage/db_test.go`,
  `cmd/royo-learn/{mcp,evidence_cli}_test.go`.
- Coherencia documental: HANDOFF, IMPLEMENTATION-LOG, IMPLEMENTATION-NOTES,
  docs 03/04/05/14/17.

## 5. Hoja de ruta (resumen; detalle en `docs/26-IMPLEMENTATION-ROADMAP.md`)

Ola 1 (un PR por hito):
- **0 ✅** docs 20-26 + ADR-0001.
- **1 ✅** Hito 1 completo en `feat/experience-hito1-1d` (merge pendiente).
- 2 OpenCode `--once`.
- 5 detectores deterministas.
- 6 patrones + migración 005.
- 7 promoción vía `capture.Service`.
- 4 trace.

Próximo paso: merge de Hito 1 + Hito 2.

## 6. Próximo trabajo

### 6.1. Decisión de merge

`feat/experience-hito1-1d` (15 commits) → `main`. Dos opciones:
- Mantener los 15 commits por unidad de trabajo (trazabilidad).
- Squash para limpieza.

Recomendación: mantener los 15 commits. Opcional `go test -race -count=2
./...` dos veces para descartar flakes residuales.

### 6.2. ADR flake `internal/mcpserver`

`ListTools: context deadline exceeded` es ortogonal, sensible a timeout
MCP, no es Windows AV. Documentado en `IMPLEMENTATION-NOTES.md`. Merece
ADR antes de Hito 2.

### 6.3. PR chico de docs/20-26 + PLAN-MAESTRO

`docs/20-26` + `PLAN-MAESTRO` viven solo en `d812709`. PR chico para
traerlos a `main` antes o después del merge de slice 1.D. Cierra la
grieta de coherencia documental pre-existente.

### 6.4. Hito 2 (OpenCode `--once`)

Una vez cerrado lo anterior, arrancar Hito 2 según `docs/26` §3.

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

- `docs/20`–`docs/26` — contratos congelados + roadmap (viven en `d812709`).
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` — decisión anti-MemSearch.
- `HANDOFF-EXPERIENCE-DISCOVERY.md` — handoff original de Hito 0/1.
- Roadmap y plan maestro:
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
- Sub-rama Hito 1 closure: `feat/experience-hito1-1d` HEAD `076ae7d`
  (15 commits sobre `4fe9774`).
- PR/issue de referencia: #17 / #16 (Hito 1 1.A-1.C).
- Pre-existing untracked: `HANDOFF-EXPERIENCE-DISCOVERY.md`,
  `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`.
