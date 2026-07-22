# HANDOFF — Royo-Learn experience discovery (cierre de sesión 2026-07-22)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar nada.
> Cierra la sesión que dejó Hito 1 slices 1.A-1.C mergeados en `main`.

## 0. Frase para pegar en la próxima sesión

Copiá y pegá esto tal cual al iniciar la próxima sesión:

```text
Continuá Royo-Learn con la hoja de ruta de la capa de descubrimiento
(repo RoyoTech/royo-learn). Antes de actuar:

1. Leé HANDOFF-EXPERIENCE-DISCOVERY.md completo (incluida esta sección 0).
2. Leé docs/26-IMPLEMENTATION-ROADMAP.md desde el `main` actual y
   PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md desde el commit histórico
   `d812709` (no viven en la rama de feature por convención "un hito
   por PR").
3. Verificá el estado real con `git log --oneline -3 main`,
   `git log --oneline -3 origin/main`, y `git status --short --branch`.

Estado: Hito 1 slices 1.A, 1.B y 1.C mergeadas en `main` como commit
único `4fe9774 feat(experience): merge Hito 1 experience discovery
(slice 1.A-1.C)` (squash). PR #17 e issue #16 cerrados y completos.
Rama local: `main`. HEAD local: `4fe9774`. Working tree: solo los dos
archivos untracked preexistentes `HANDOFF-EXPERIENCE-DISCOVERY.md` y
`PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`.

Tareas pendientes para esta sesión, en este orden:

3. Slice 1.D: comando CLI interno para inyectar un `ExperienceEnvelope`
   de fixture, suite de aceptación de Hito 1 (envelope válido crea
   sesión/turno; reintento exacto no duplica; revisión actualiza
   seguro; secreto no llega a ningún sink; cursor solo tras commit;
   `-race ./...` y cross-build windows/linux/macOS verdes),
   `ExperienceConfig` deshabilitado por defecto en
   `internal/config`, y coherencia documental (HANDOFF,
   IMPLEMENTATION-LOG/NOTES, docs 03/04/05/14/17).

4. Aislar o documentar los flakes preexistentes del entorno Windows:
   `cmd/royo-learn TestRunPreviewEndToEnd` (cleanup de `t.TempDir`)
   y `internal/buildinfo` (`fork/exec ... Access is denied`). Que no
   contaminen la corrida `-race` ni la matriz de cross-build de
   CI para Hito 1.

5. Cierre de Hito 1: gate `-race ./...`, cobertura de
   `internal/experience` (≥90% según plan §24), cross-build formal
   windows/linux/macOS, y verificación final de los contratos
   congelados en docs/20-26.

Reglas innegociables (recordatorio):
- TDD estricto: RED primero, GREEN después, refactor sólo con tests
  verdes.
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

- Implementé slice 1.C (servicio de ingestión) en la rama existente
  `feat/experience-hito1-domain` sin abrir otro PR.
- Resolví 13 hallazgos severos del primer 4R (redacción, identidad,
  atomicidad cursor, JSON determinista, CAS sesión/turno, observación
  de fallos).
- Reconcilié la numeración de migraciones: `source_order` vive en
  `004_experience_ingestion.sql`; nunca se creó `005_*` (reservada por
  roadmap para patrones de Hito 6).
- Un segundo 4R aprobó todo menos `RESILIENCE-001` (orden de
  metadata de fallo en cursor). Lo arreglé con un guard tipado y
  la validación independiente aprobó con cinco escenarios
  deterministas.
- Atomicé trabajo en 8 commits en `feat/experience-hito1-domain` y
  los empujé. PR #17 quedó con `head.sha = 9ec8805`.
- PR #17 mergeado vía squash contra `main` como
  `4fe9774 feat(experience): merge Hito 1 experience discovery
  (slice 1.A-1.C)`. Issue #16 cerrado por la cláusula `Closes #16`.
- Adelanté la rama local `main` con fast-forward a `4fe9774`.

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

- Rama local actual: `main`.
- HEAD local: `4fe9774`.
- Working tree: solo los dos untracked preexistentes
  (`HANDOFF-EXPERIENCE-DISCOVERY.md` y `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`),
  intencionalmente fuera del merge.
- PR #17: cerrado y mergeado.
- Issue #16: cerrado, estado `completed`.
- `feat/experience-hito1-domain` queda en `origin` con los 8 commits
  originales por trazabilidad (no la borres; sigue siendo la fuente
  histórica del trabajo aprobado).

## 4. Cobertura que ya está mergeada en `main`

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

## 5. Hoja de ruta (resumen; detalle en `docs/26-IMPLEMENTATION-ROADMAP.md`)

Ola 1 (un PR por hito):
- **0 ✅** docs 20-26 + ADR-0001.
- **1 ✅** dominio + migración 004 + servicio de ingestión (mergeado
  en `4fe9774`).
- 2 OpenCode `--once`.
- 5 detectores deterministas.
- 6 patrones + migración 005.
- 7 promoción vía `capture.Service`.
- 4 trace.

El siguiente paso es **slice 1.D** dentro de Hito 1: CLI de fixture,
suite de aceptación y `-race` global.

## 6. Próximo trabajo — slice 1.D y cierre de Hito 1

Contratos congelados en `docs/20`/`21`/`24`/`25`/`26`.

### Slice 1.D — comando interno + suite de aceptación

- Añadir subcomando en `cmd/royo-learn/main.go` para inyectar un
  `ExperienceEnvelope` de fixture (sin adaptador real; sin daemon).
- `stdout` = JSON; logs a `stderr`.
- Tests de aceptación del Hito 1 (plan §22):
  - envelope válido crea sesión y turno;
  - reintento exacto no duplica;
  - revisión actualiza de forma segura;
  - secreto no llega a ningún sink;
  - cursor se actualiza solo tras commit;
  - `go test -race ./...` y cross-build windows/linux/darwin.
- Documentación: HANDOFF, IMPLEMENTATION-LOG/NOTES, docs 03/04/05/14/17.

### Config (parte de Hito 1)

- Añadir `ExperienceConfig` a `internal/config/config.go`
  deshabilitado por defecto, respetando trust boundary.
- Revisar `internal/config/merge.go` y `validate.go`.

### Aislar flakes preexistentes del entorno Windows

- `cmd/royo-learn TestRunPreviewEndToEnd`: falla el cleanup de
  `t.TempDir` (`directory is not empty`, lock de Windows).
- `internal/buildinfo`: `fork/exec ... Access is denied` (permiso
  Windows/AV).
- Aislarlos o documentarlos para que no contaminen la corrida
  `-race` ni la matriz CI de Hito 1.

### Gate de salida del Hito 1

- `-race ./...` verde sobre los paquetes del hito.
- Cobertura `internal/experience >= 90%` (plan §24).
- Cross-build windows/linux/macOS con artefactos verificables.
- Documentación coherente con los contratos congelados.

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

- `docs/20`–`docs/26` — contratos congelados + roadmap.
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` — decisión anti-MemSearch.
- `HANDOFF-EXPERIENCE-DISCOVERY.md` — handoff original de Hito 0/1.
- Roadmap y plan maestro:
  - `docs/26-IMPLEMENTATION-ROADMAP.md` (en `main`).
  - `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` en commit histórico
    `d812709`.
- Código a reutilizar:
  - `internal/capture/capture.go` (`Service.Capture`).
  - `internal/evidence/` (redacción, runner sin shell).
  - `internal/storage/` (migrate.go, repo_experience.go).
  - `internal/domain/` (types, errors, validation).
- Commits squash mergeados: `4fe9774` en `main`.
- PR/issue de referencia: #17 / #16.
## Slice 1.D — estado de implementación

Estado: implementada en `feat/experience-hito1-1d` con cuatro commits atómicos previstos: configuración, CLI, acceptance suite y documentación.

Archivos principales: `internal/config/config.go`, `internal/config/config_test.go`, `cmd/royo-learn/experience.go`, `cmd/royo-learn/experience_test.go`, `cmd/royo-learn/main.go`, `internal/experience/acceptance_test.go`, y documentación de CLI, dominio, MCP, aceptación y errores.

Verificación pendiente al cierre: `go fmt ./...`, `go vet ./...`, `go test ./...`, `go test -race -p 1 ./...`, cobertura de `internal/experience` y cross-build Windows/Linux/macOS.
