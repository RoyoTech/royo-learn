# HANDOFF — Royo-Learn post-Hito 6 → arranque de Hito 7 (promoción vía capture.Service)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar
> nada. Cierra la sesión que dejó Hito 6 mergeado a `main` (PR #23,
> squash commit `55ef635`) y deja slice 7.0 commitado en
> `feat/hito7-promotion` (`d9617af`). La próxima sesión continúa con
> slice 7.1 (pipeline de redacción previo al `capture.Service.Ingest`).

## 0. Frase para pegar en la próxima sesión

```text
Continuá Royo-Learn desde `feat/hito7-promotion` (HEAD `d9617af`,
slice 7.0 commitado) que se creó desde `origin/main` (`9f5dee1`).
Hito 6 está mergeado en `main` (PR #23, squash commit `55ef635`).
`v0.2.0-rc1` sigue siendo el último tag (correcto: el trigger de
`v0.2.0` espera a Hito 4 — el último PR de Ola 1).

Antes de actuar:

1. Leé HANDOFF-HITO7-PROMOTION.md completo (incluida esta sección 0).
2. Leé docs/26-IMPLEMENTATION-ROADMAP.md §3 PR #6 para los gates
   de salida de Hito 7.
3. Leé docs/20-EXPERIENCE-INGESTION-PRD.md §6 RF-E08 (si existe)
   y docs/23-PATTERN-MINING.md §8.
4. Leé docs/lessons.md — los patterns operacionales aprendidos
   (bypass de WSL para el lifecycle interceptor, scope discipline
   del review, Windows 8.3 path handling, JSON round-trip int →
   float64, MCP `json.RawMessage` schema, finalize-dropped rule §5).
5. Verificá el estado real con:
   `git log --oneline -5 feat/hito7-promotion` (último debe ser
   `d9617af`, slice 7.0 del paquete promotion),
   `git status --short --branch` (limpio, sólo PROMPT untracked,
   preservado por convención),
   `git tag --list 'v0.*' | tail -3` (último debe ser `v0.2.0-rc1`),
   `go test -race -count=1 ./internal/experience/promotion/...`
   (debe pasar; cobertura scaffold 51.2%, gate de 80% aplica al
   Hito 7 entero).

Estado: Hitos 0/1/2/5/6 mergeados a main. PR #23 cerrado. Slice 7.0
commitado en `feat/hito7-promotion`. Working tree limpio excepto
PROMPT (preservado). Cobertura de `internal/experience/promotion`
51.2% (scaffold).

Tarea de la próxima sesión, en este orden:

1. (Decisión de housekeeping) El handoff actual
   (`HANDOFF-HITO7-PROMOTION.md`) y el `tasks/hito7-todo.md` quedan
   en la rama `feat/hito7-promotion`. Si querés mover el handoff a
   `main` (convencional), confirmar con el operador antes de
   cualquier push o merge.
2. (Opcional) Verificar que `CHANGELOG.md [Unreleased]` aún no
   tiene la entrada de Hito 7 (la entrada llega al cierre, no en
   slices intermedios).
3. Slice 7.1 — pipeline de redacción previo al `capture.Service.Ingest`.
   Extiende `internal/evidence` con la redacción de campos derivados
   del pattern (title, observation, retrieval terms, fingerprint
   preview hash). TDD estricto: tests de evidencia redactada
   primero, RED, luego implementación GREEN.
4. Slice 7.2-7.4 en orden, TDD estricto. El detalle completo está
   en HANDOFF-HITO7-PROMOTION.md §4 más abajo.
5. (PR #6 según docs/26) — un solo PR al cierre con los 5 slices.

Reglas innegociables (recordatorio, no negociables en este proyecto):

- TDD estricto: RED primero, GREEN después, REFACTOR solo con tests
  verdes. `go test -race ./...` antes de cada commit.
- Redacción antes de hash y persistencia. El preview hash no se
  calcula antes de la redacción.
- Reusar `capture.Service` (Hito 1) e `internal/evidence`; no
  duplicar.
- SQLite = verdad operacional. La promoción escribe solo vía
  `capture.Service` y repos, NUNCA directamente.
- Sin Python/Bash/shell en runtime; sin red obligatoria; sin daemon.
- Sin LLM embebido en v1 (regla 9 de AGENTS.md).
- Un hito por PR; commits por unidad de trabajo; conventional
  commits, sin `Co-Authored-By` ni atribución de IA.
- Toda publicación compartida o cambio de `AGENTS.md` requiere
  aprobación humana verificable (regla 11 de AGENTS.md).
- No pushear ni abrir PR sin que el usuario lo pida explícitamente.
- El path-mangling de `wsl.exe` desde Git Bash requiere
  `MSYS_NO_PATHCONV=1` y rutas WSL completas (ver docs/lessons.md
  §2). El harness bash bloquea lifecycle commands; el patrón
  documentado es el script en `/mnt/c/.../`.
- **Windows 8.3 paths**: en Windows, `t.TempDir()` puede devolver
  nombres cortos como `RUNNER~1` mientras que `os.Lstat`/`Open`
  devuelven nombres largos como `runneradmin`. Para comparar paths
  en tests, usar `project.Canonicalize` en ambos lados (no
  `strings.EqualFold`, que no normaliza la forma corta).
- **JSON round-trip**: cuando un campo `int` se serializa a JSON
  y se re-decodifica en un `any`/`interface{}`, llega como
  `float64`. Para tests que verifican el shape JSON del output,
  coercionar a `float64` (no `int`).
- **MCP `json.RawMessage`**: el esquema inferido para `json.RawMessage`
  es `{"type": "array"}`, lo cual rechaza payloads-objeto en la
  validación. Para tools MCP que aceptan cualquier JSON, declarar
  el campo como `any` (interface{}).
- **`gentle_review finalize` puede ser silently dropped** (ver
  docs/lessons.md §5). Confirmar con el operador antes de
  aceptar el gap; documentar la decisión antes de merge.
- **`domain.Actor` usa `Name`, no `ID`** (campo `Name` string;
  campos: `Kind`, `Name`, `Model`, `SessionID`).

Criterios de "Hito 7 hecho" (per docs/26 §3 PR #6, gates de salida):

- Un `Pattern` cualificado por el miner (Hito 6) puede ascender
  a `Learning` vigente, vía `capture.Service`, en una sola
  transacción con audit row.
- La promoción es idempotente: un `(pattern_id)` ya promovido
  no produce un segundo `Learning`; el caller recibe el existente.
- La promoción respeta el contrato de redacción e idempotencia de
  Hito 1 (no introduce una segunda ruta de escritura).
- Toda promoción deja un audit row en `audit_events` con
  `entity_type=pattern`, `entity_id=pattern_id`, `operation=promote`,
  `details={"learning_id": ..., "source": "pattern_mining", ...}`.
- Disponibilidad CLI `experience patterns promote --id <id>` y
  MCP `learning_promote_pattern` (admin-only).
- Cobertura del paquete de promoción ≥ 80%.
- `go test -race ./...` verde; cross-build win/linux/darwin verde
  (con la directiva del operador: solo Windows es required para
  merge, los demás quedan como ruido).
```

---

## 1. Qué cambió en esta sesión

- Hito 6 mergeado a `main` como squash commit `55ef635` (PR #23).
  34 archivos, 8144 inserciones, 12 borradas. Ramas local y remota
  `feat/hito6-patterns` borradas.
- Housekeeping commit `9f5dee1` con CHANGELOG [Unreleased] +
  ROADMAP + `HANDOFF-HITO6-CLOSEOUT.md`.
- Slice 7.0 del Hito 7 creado en `feat/hito7-promotion` (`d9617af`):
  paquete `internal/experience/promotion/` con tipos, errores,
  interface, stub del Service, tabla de tests del contrato.
- Nuevo `ErrorCode = "promotion_not_implemented"` en domain/errors.go.
- `docs/17-ERROR-CODES.md` actualizado.
- `tasks/hito7-todo.md` con el plan de execution + lista de
  riesgos abiertos.
- `HANDOFF-HITO7-PROMOTION.md` (este archivo) con la frase puente
  para la próxima sesión.

## 2. Estado actual del repositorio

- `main` HEAD local: `9f5dee1`
- `origin/main` HEAD: `9f5dee1` (sincronizado, 0 ahead, 0 behind)
- `feat/hito7-promotion` HEAD: `d9617af` (1 commit ahead de main)
- PRs abiertos: ninguno. PR #23 mergeado y cerrado.
- Tag `v0.2.0-rc1` en remote: `706439e`, anotado (sin cambios).
- Working tree: solo `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked
  (intencional).
- Ramas locales de Hitos ya mergeados: borradas.
- Rama `feat/hito7-promotion` creada pero NO pusheada.

## 3. Invariantes innegociables (recordatorio)

- Go + SQLite como núcleo; SQLite es la verdad operacional.
- Redacción **antes** de hash y persistencia.
- Experiencia observada ≠ conocimiento aprobado.
- Promoción únicamente vía `capture.Service`.
- Sin Python/Bash/`os.system`/shell; sin red obligatoria;
  sin daemon.
- Preservar CLI/MCP actuales, JSON estable, Windows/Linux/macOS.
- Preview hash, aprobación, publicación atómica, verificación,
  rollback intactos.
- Un hito por PR; commits por unidad de trabajo; conventional
  commits, sin atribución de IA.
- Toda publicación compartida o cambio de `AGENTS.md` requiere
  aprobación humana verificable.
- El path-mangling de `wsl.exe` desde Git Bash requiere
  `MSYS_NO_PATHCONV=1` (ver `docs/lessons.md` §2).
- Windows 8.3 short names: usar `project.Canonicalize` para
  comparar paths (no `strings.EqualFold`).
- **Directiva del operador (2026-07-25)**: "piensa en hacer
  royo-learn solo para windows inicialmente, no gastes tiempo
  ni tokens en linux y mac. Lo escribo si es que viene al caso".
  El CI cross-platform queda como ruido; mergear requiere solo
  que Windows + clean install + Windows installer safety pasen.
- JSON round-trip: `int` se decodifica como `float64` cuando
  llega desde `any`/`interface{}`. Documentado en los tests del
  slice 5.3.
- MCP `json.RawMessage` infiere array en el schema, lo cual
  rechaza payloads-objeto. Usar `any` para campos abiertos.
- `gentle_review finalize` puede ser silently dropped (ver
  `docs/lessons.md` §5). El operador puede aceptar el gap
  documentado; siempre confirmar antes de proceder.
- `domain.Actor` tiene `Name`, no `ID`. Campos: `Kind`, `Name`,
  `Model`, `SessionID`.

## 4. Slice breakdown de Hito 7 (referencia, no contrato)

Detalle propuesto. El próximo agente puede ajustar el orden si la
implementación revela dependencias distintas, pero la cobertura de
los gates de salida no se negocia.

| # | Sub-slice | Qué entrega | Gate específico |
|---|---|---|---|
| **7.0** | Scaffold | Paquete `internal/experience/promotion/` con tipos, errores, interface, stub del Service, tabla de tests del contrato | ✅ Compila; tests del contrato pasan (GREEN) |
| **7.1** | Redacción + preview hash | Pipeline de redacción previo al `capture.Service.Ingest` (extiende `internal/evidence`) | Redacción determinista; preview hash estable |
| **7.2** | Promoción transaccional | Función `Promote(ctx, pattern)` que abre tx, captura vía `capture.Service`, inserta audit row, actualiza pattern.status y proposed_learning_id | Tx atómica; rollback limpio en error |
| **7.3** | Idempotencia | Lookup pre-insert: si el pattern ya tiene `proposed_learning_id`, retorna el existente con `WasNew=false` | Doble `Promote` no produce doble `Learning` |
| **7.4** | CLI + MCP + acceptance e2e | `experience patterns promote --id <id>`, `learning_promote_pattern` (admin-only), acceptance test con patrón sintético | Idempotente; cobertura ≥ 80% (Hito 7 entero) |

**Total esperado**: 5 commits atómicos en una sola rama
`feat/hito7-promotion` desde `origin/main`. Un solo PR al cierre
(PR #6 de la roadmap).

**Contratos a respetar** (todos en `origin/main` ahora):

- `docs/20-EXPERIENCE-INGESTION-PRD.md` §6 RF-E08 — promoción
  auditada (si está en el spec; si no, usar el último doc que
  defina la interfaz).
- `docs/22-ADAPTER-CONTRACT.md` — no se aplica directamente a la
  promoción, pero comparte la convención de redacción.
- `docs/26-IMPLEMENTATION-ROADMAP.md` §3 PR #6 — gates de salida.
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` — mapeo requisito →
  acceptance → test.
- `internal/capture/service.go` — punto único de entrada para
  crear `Learning`. La promoción lo invoca, no lo duplica.
- `internal/experience/patterns/{patterns.go, service.go}` —
  superficie actual para leer el pattern y el fingerprint.

## 5. Próximo trabajo

### 5.1. (Hito 7) Slice 7.1

El pipeline de redacción. TDD estricto: escribir los tests de
evidencia redactada primero (campos derivados del pattern
redactados antes de la inserción), ver que fallen (RED),
implementar la redacción, ver que pasen (GREEN).

Referencia: `internal/evidence/service.go` y `internal/evidence/redact.go`
para entender qué campos se redactan y cómo se construye el
preview hash.

Rama: `feat/hito7-promotion` (HEAD actual `d9617af`).

### 5.2. (Después de Hito 7) Hito 4

Hito 4 cierra Ola 1. Es el último PR antes de poder taggear
`v0.2.0`. Cubre trace progresivo end-to-end (necesita eventos
con procedencia de Hito 1 y se vuelve demostrable en cuanto
existe la promoción de Hito 7). Al cerrar el último, el trigger
table del CHANGELOG dice "cortar `v0.2.0`".

### 5.3. (Mucho después) Ola 2 y Ola 3

Ola 2 (5 PRs): motor de jobs, retrieval lexical, OpenCode
`--watch`, Claude Code, Codex.
Ola 3 (3 PRs): drift/release, semántica opcional, Pi.

Estas olas son multi-mes. No relevante para la próxima sesión.

## 6. Lecciones operativas aprendidas en esta sesión (para slice 7.1 y siguientes)

- **`domain.Actor` usa `Name`, no `ID`.** El lens detuvo un typo en
  slice 7.0 (campo `ID` no existe en `domain.Actor`). Los campos
  son `Kind`, `Name`, `Model`, `SessionID`. Cualquier referencia
  futura a `in.Actor.ID` debe ser `in.Actor.Name`.
- **Cada error tipado del paquete necesita un `ErrorCode` único.**
  El test `TestTypedErrors_AreDistinct` cazó que dos errores
  compartían canonical code (`invalid_argument`). Para agregar
  errores nuevos: (a) declarar el `ErrorCode` en
  `domain/errors.go`, (b) agregarlo a `AllErrorCodes()`, (c)
  decidir si asignar `ExitCode()` o usar el default (1).
- **`go test -cover` solo falla por el linker buildID** (AV
  Windows). `-coverprofile=/tmp/... + go tool cover -func`
  funciona y reproduce la cobertura sin race.
- **El scaffold tiene cobertura modesta** (51.2% en slice 7.0).
  El gate de 80% aplica al Hito 7 entero (slices 7.0–7.4). No
  inflar con assertion-free tests; los tests reales llegan con
  cada slice.
- **`docs/17-ERROR-CODES.md` debe actualizarse** cada vez que se
  agrega un `ErrorCode`. El catálogo está sincronizado con el
  `domain.AllErrorCodes()` pero el docs no se autogenera.
- **`PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` queda untracked, preservado
  por convención.** No commitearlo, no incluirlo en PRs.

## 7. Lo que NO hay que hacer (anti-trampas)

- **No** crear rama desde `main` local (puede diverger de
  `origin/main`). Usar siempre `origin/main`. Ver `docs/lessons.md` §4.
- **No** abrir PR sin que el usuario lo pida explícitamente.
- **No** modificar `AGENTS.md` sin aprobación humana.
- **No** commitear el `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked.
- **No** gastar tiempo/tokens en CI cross-platform (Linux/Mac) —
  directiva del operador. Solo Windows + clean install smoke +
  Windows installer safety son required para merge. Lo demás
  queda como ruido.
- **No** "arreglar" el flake MCP reintroduciendo retry masking
  o relajando timeouts — ADR-0002 §2.2 lo prohíbe explícitamente.
- **No** usar embeddings o base vectorial en v1 (AGENTS.md
  regla 9). El clustering v1 es fingerprint + Jaccard.
- **No** insertar un LLM provider dentro del binario en v1.
- **No** forkear ni modificar `Gentle-AI` ni `Engram`.
- **No** escribir en bases internas de terceros.
- **No** capturar conversaciones completas por defecto.
- **No** guardar razonamiento privado del modelo.
- **No** modificar globalmente Codex, OpenCode o Claude sin
  backup y consentimiento.
- **No** generar instaladores que descarguen y ejecuten código
  no fijado sin mostrar procedencia.
- **No** sustituir pruebas por mocks cuando el comportamiento
  real pueda verificarse localmente.
- **No** eliminar aprendizajes; usar estados `rejected`,
  `superseded` o `archived`.
- **No** reintroducir el bug de `finalize` retrying: si el
  receipt queda `not_applicable`, no seguir intentando. Pedir
  confirmar al operador o esperar a que el bug nativo se
  resuelva.
- **No** asumir que `domain.Actor` tiene `ID` — usar `Name`.
- **No** agregar un error tipado con un `ErrorCode` que ya
  tiene otro error tipado del paquete.

## 8. Referencias clave

- `CHANGELOG.md` — versión actual, historia, version↔ola map,
  trigger table. El bloque `Status as of 2026-07-25 (post-Hito 6
  merge)` documenta el estado actual.
- `ROADMAP.md` — meta-doc con el estado por ola. Hito 6 marcado
  ✅; próximo Hito 7.
- `docs/lessons.md` — patterns operacionales (shell, WSL, review,
  PR base, Windows 8.3, finalize-dropped). Léelo antes de tocar
  nada con git o WSL.
- `AGENTS.md` — reglas no negociables del proyecto (no modificar
  sin aprobación humana).
- `CLAUDE.md` — pointer file para agentes Anthropic.
- `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` — plan de fondo de
  las capacidades a absorber.
- `docs/26-IMPLEMENTATION-ROADMAP.md` — roadmap por ola, gates
  de salida por hito. Hito 6 marcado ✅; próximo Hito 7.
- `docs/20-EXPERIENCE-INGESTION-PRD.md` — contrato de ingestión.
- `docs/21-EXPERIENCE-DOMAIN.md` — entidades del dominio
  experiencia.
- `docs/22-ADAPTER-CONTRACT.md` — interfaz del adapter.
- `docs/23-PATTERN-MINING.md` — qué cualifica, qué descarta,
  qué se promueve.
- `docs/24-EXPERIENCE-THREAT-MODEL.md` — reglas de seguridad.
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` — decisión
  anti-MemSearch.
- `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` — flake MCP
  investigado con resultado negativo.
- `tasks/hito7-todo.md` — plan de execution + riesgos abiertos.
- `HANDOFF-EXPERIENCE-DISCOVERY.md` — handoff previo, describe
  el cierre de Hito 1.
- `HANDOFF-HITO2-OPENCODE-ONCE.md` — handoff previo, describe
  el inicio de Hito 2 desde `v0.2.0-rc1`.
- `HANDOFF-HITO5-DETECTORS.md` — handoff previo, describe el
  inicio de Hito 5.
- `HANDOFF-HITO6-PATTERNS.md` — handoff previo, describe el
  cierre de Hito 6.
- `HANDOFF-HITO6-CLOSEOUT.md` — handoff del cierre de Hito 6.
- `HANDOFF-HITO7-PROMOTION.md` — ESTE handoff, describe el
  cierre parcial de Hito 7 (slice 7.0) y el arranque de slice 7.1.
- Reviews:
  - `.git/gentle-ai/review-transactions/v2/hito2-opencode-once-review-v1/`
    (high tier, 4R, 2 SUGGESTION no bloqueantes, approved).
  - `.git/gentle-ai/review-transactions/v2/hito6-patterns-review-v2/`
    (high tier, finalize dropped 4x, gap operator-accepted).
- PR/issue: #19 (docs), #20 (meta-docs), #21 (Hito 2), #22 (Hito 5),
  #23 (Hito 6).
- Pre-existing untracked: `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`
  (preservado fuera del merge por convención).
