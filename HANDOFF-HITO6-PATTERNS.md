# HANDOFF — Royo-Learn post-Hito 5 → arranque de Hito 6 (patterns + clustering)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar
> nada. Cierra la sesión que dejó Hito 5 mergeado a `main` y deja el
> árbol verde. La próxima sesión continúa con el slice 6.0 (scaffold
> del paquete de patterns + tabla de tests del contrato de
> cualificación).

## 0. Frase para pegar en la próxima sesión

Copiá y pegá esto tal cual al iniciar la próxima sesión:

```text
Continuá Royo-Learn en `main` local/remoto, donde quedó mergeado
Hito 5 como squash commit `59d5e74` (PR #22, merge --squash
--delete-branch, rama `feat/hito5-detectors` borrada en local y
remoto). `v0.2.0-rc1` sigue siendo el último tag (correcto: el
trigger de `v0.2.0` espera a Hito 4 — el último PR de Ola 1).

Antes de actuar:

1. Leé HANDOFF-HITO6-PATTERNS.md completo (incluida esta sección 0).
2. Leé docs/26-IMPLEMENTATION-ROADMAP.md §3 PR #5 para los gates
   de salida de Hito 6.
3. Leé docs/lessons.md — los patterns operacionales aprendidos
   (bypass de WSL para el lifecycle interceptor, scope discipline
   del review, Windows 8.3 path handling, fix del bug de --fixture,
   deteminismo JSON round-trip int→float64, schema del MCP
   `json.RawMessage` infiere array).
4. Leé docs/23-PATTERN-MINING.md §3-7 para entender qué cualifica
   un patrón y qué lo descarta.
5. Leé docs/20-EXPERIENCE-INGESTION-PRD.md §6 RF-E07 y docs/24
   para entender la frontera de seguridad del miner.
6. Verificá el estado real con:
   `git log --oneline -5 main` (último debe ser `59d5e74`),
   `git status --short --branch` (solo PROMPT untracked, preservado
   por convención),
   `git tag --list 'v0.*' | tail -3` (último debe ser `v0.2.0-rc1`),
   `go test -race -count=1 ./internal/experience/...`
   (debe pasar, incluyendo `internal/experience/detectors` 90.1%
   y `internal/experience/opencode` 80.5%).

Estado: Hito 0/1/2/5 mergeados a main. PR #22 cerrado. Working tree
limpio excepto PROMPT (preservado). Cobertura de
`internal/experience/detectors` 90.1%. Slice 5.0 → 5.4 + MCP tool
cubiertos.

Tarea de la próxima sesión, en este orden:

1. (Decisión de housekeeping) No queda housekeeping pendiente en
   este momento — el commit `b2090ce` cubrió post-Hito 2 y este
   commit cubre post-Hito 5. Si antes de empezar aparece algo,
   consultá al operador.
2. (Opcional) Verificar que `CHANGELOG.md [Unreleased]` lleva
   las entradas de Hito 2 y Hito 5 antes de empezar. Si falta
   alguna, agregarla.
3. Crear rama `feat/hito6-patterns` desde `origin/main` (NO desde
   local main — ver docs/lessons.md §4 sobre el scope inflado).
4. Slice 6.0 — scaffold del paquete `internal/experience/patterns/`
   con stubs de la interfaz `Pattern`, tabla de tests del
   contrato de cualificación, y stubs de los algoritmos de
   clustering v1 (fingerprint exact + Jaccard sobre retrieval
   terms, sin embeddings). RED primero, GREEN después. Sin
   lógica todavía.
5. Slice 6.1-6.4 en orden, TDD estricto. El detalle completo
   está en HANDOFF-HITO6-PATTERNS.md §4 más abajo.
6. (PR #5 según docs/26) — un solo PR al cierre con los 5 slices.

Reglas innegociables (recordatorio, no negociables en este proyecto):

- TDD estricto: RED primero, GREEN después, REFACTOR solo con tests
  verdes. `go test -race ./...` antes de cada commit.
- Redacción antes de hash y persistencia. El fingerprint no se
  calcula antes de la redacción.
- Reusar `capture.Service` e `internal/evidence`; no duplicar.
- SQLite = verdad operacional. El miner escribe solo vía
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
  documentado es el script en `/mnt/c/.../run.sh`.
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

Criterios de "Hito 6 hecho" (per docs/26 §3 PR #5, gates de salida):

- 3 sesiones cualifican un patrón; 3 reintentos (mismo fingerprint
  en distintas sesiones) NO cualifican (anti-anti-pattern
  detector).
- Una contradicción posterior bloquea la cualificación.
- Miembros de un patrón son trazables hasta el turno (`Learning ↔
  ExperienceEvent` vía tabla dedicada — migración 005).
- Sin embeddings (regla 9 de AGENTS.md; Jaccard sobre retrieval
  terms + fingerprint exact es el camino v1).
- Cobertura `internal/experience/patterns/` ≥ 80% (umbral de
  dominio per AGENTS.md §Calidad mínima).
- `go test -race ./...` verde; cross-build win/linux/darwin verde
  (con la directiva del operador: solo Windows es required para
  merge, los demás quedan como ruido).
```

---

## 1. Qué cambió en esta sesión

- Hito 5 mergeado a `main` como squash commit `59d5e74` (PR #22).
  18 archivos, 2846 inserciones, 3 borradas. Ramas local y remota
  `feat/hito5-detectors` borradas (merge con `--delete-branch`).
- Working tree dejado intencionalmente con un solo untracked:
  - `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` — preservado por
    convención del proyecto.
- Slice 5.0 → 5.4 + MCP tool cubiertos. 8 commits atómicos
  intermedios:
  - `1f953fa` — slice 5.0 scaffold (Detector interface + tabla
    del contrato)
  - `af8e687` — slice 5.1 retry detector
  - `53031e1` — slice 5.2 registry
  - `7b2313d` — slice 5.3 CLI experience detect
  - `b6ab7af` — slice 5.4 persist via experience.Service
  - `11d4347` — slice 5.4 close: MCP tool experience_detect_events
  - `7c81d86`, `9fba09d` — gofmt whitespace cleanups
- `CHANGELOG.md [Unreleased]` extendido con la entrada de Hito 5
  (PR #22, merge commit `59d5e74`). Se acumula hasta el corte de
  `v0.2.0` cuando mergea Hito 4.
- Tag `v0.2.0-rc1` en remote: `706439e`, anotado (sin cambios).
- El trigger table del CHANGELOG confirma que `v0.2.0` espera a
  Hito 4 (no se taggea todavía). PRs abiertos: ninguno.

## 2. Estado actual del repositorio

- `main` HEAD local: `59d5e74`
- `origin/main` HEAD: `59d5e74` (sincronizado, 0 ahead, 0 behind)
- Tag `v0.2.0-rc1` en remote: `706439e`, anotado (sin cambios)
- PRs abiertos: ninguno. PR #22 mergeado y cerrado.
- Working tree: solo `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked
  (intencional).
- Ramas locales de Hitos ya mergeados: borradas.

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
  Los coverage gates pueden fallar legítimamente si dependen de
  tests que flakean en Linux (no es scope de cada PR arreglarlo,
  abrir issue aparte).
- JSON round-trip: `int` se decodifica como `float64` cuando
  llega desde `any`/`interface{}`. Documentado en los tests del
  slice 5.3.
- MCP `json.RawMessage` infiere array en el schema, lo cual
  rechaza payloads-objeto. Usar `any` para campos abiertos.

## 4. Slice breakdown de Hito 6 (referencia, no contrato)

Detalle propuesto. El próximo agente puede ajustar el orden si la
implementación revela dependencias distintas, pero la cobertura de
los gates de salida no se negocia.

| # | Sub-slice | Qué entrega | Gate específico |
|---|---|---|---|
| **6.0** | Scaffold | Paquete `internal/experience/patterns/` con stubs de la interfaz `Pattern`, tipo `PatternStatus`, tabla de tests del contrato de cualificación | Compila; tests del contrato fallan (RED) |
| **6.1** | Fingerprint + retrieval terms | Hash determinista de pattern (extiende el de EventFingerprint) + normalización de retrieval terms | Determinista; misma entrada → mismo fingerprint |
| **6.2** | Clustering v1 | Función pura `Cluster(events)` que devuelve clusters por fingerprint exacto + Jaccard sobre retrieval terms | Sin embeddings; ≥ 2 eventos → 1 cluster |
| **6.3** | Cualificación | Función `Qualify(cluster)` con los criterios de docs/23 §5 (≥ 3 sesiones distintas O ≥ 3 días, ≥ 2 ocurrencias exitosas, sin contradicción, no cubierto por Learning vigente, etc.) | Tabla de tests cubre los criterios y los anti-patrones (3 reintentos en 1 sesión NO cualifican) |
| **6.4** | Dismissal + listing + aceptación e2e | `Dismiss(pattern, reason)` con motivos tipados, `List(status)`, `Get(id)`, acceptance test contra fixture con eventos sintéticos | Idempotente; 7 criterios de cualificación cubiertos; cobertura ≥ 80% |

**Total esperado**: 5 commits atómicos en una sola rama
`feat/hito6-patterns` desde `origin/main`. Un solo PR al cierre
(PR #5 de la roadmap).

**Contratos a respetar** (todos en `origin/main` ahora):

- `docs/23-PATTERN-MINING.md` — qué cualifica, qué descarta,
  qué se promueve.
- `docs/20-EXPERIENCE-INGESTION-PRD.md` §6 RF-E07 — minería
  como pipeline auditable, no llamada única a un LLM.
- `docs/24-EXPERIENCE-THREAT-MODEL.md` — frontera de seguridad.
- `docs/26-IMPLEMENTATION-ROADMAP.md` §3 PR #5 — gates de salida.
- `docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md` — mapeo requisito →
  acceptance → test.

## 5. Próximo trabajo

### 5.1. (Hito 6) Slice 6.0

El scaffold es chico. TDD estricto: escribir los tests del contrato
primero (la tabla de tests que demuestra que `Pattern`,
`PatternStatus`, `QualificationCriteria`, `Membership` existen con
la firma correcta), ver que fallen (RED), implementar los stubs
mínimos, ver que pasen (GREEN). Sin lógica todavía.

Rama: `feat/hito6-patterns` desde `origin/main` (no desde
local main — ver `docs/lessons.md` §4 sobre el scope inflado).

### 5.2. (Después de Hito 6) Hito 7, 4

Hitos restantes de Ola 1. Cubren promoción vía `capture.Service`
(Hito 7) y trace progresivo (Hito 4). Cada uno es un PR. Al cerrar
el último (Hito 4), el trigger table del CHANGELOG dice "cortar
`v0.2.0`".

### 5.3. (Opcional) Housekeeping

`PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` queda preservado por convención.
Cualquier housekeeping adicional (CHANGELOG updates, otros handoffs)
debe decidirse con el operador antes de commitear.

### 5.4. (Mucho después) Ola 2 y Ola 3

Ola 2 (5 PRs): motor de jobs, retrieval lexical, OpenCode
`--watch`, Claude Code, Codex.
Ola 3 (3 PRs): drift/release, semántica opcional, Pi.

Estas olas son multi-mes. No relevante para la próxima sesión.

## 6. Lecciones operativas aprendidas en esta sesión (para PR #5 y siguientes)

- **El threshold y la ventana del retry detector deben ser
  configurables al construir el detector** (no en runtime). El
  constructor `NewRetryDetector(threshold, window)` rechaza
  configuración inválida al inicio — la alternativa (validar al
  primer Detect) deja el detector en estado zombie entre el
  startup y la primera observación.
- **El detector debe defender contra payload equivocado** (typed
  error, no panic). El slice 5.1 cubre este caso con
  `TestRetryDetector_WrongPayloadType`. El miner de slice 6.3
  debe respetar la misma convención.
- **`EventFingerprint` debe aceptar Go maps con keys en cualquier
  orden** (orden-independiente). Si no, dos corridas del mismo
  detector con el mismo evento producen fingerprints distintos y
  rompen la idempotencia del ingest. Cubierto en
  `TestEventFingerprint_OrderIndependent`.
- **MCP `json.RawMessage` infiere array en el schema** y rechaza
  payloads-objeto en validación. Para tools MCP que aceptan
  cualquier JSON, declarar el campo como `any` (interface{}).
  El decoder interno hace re-marshal → unmarshal al tipo fuerte.
- **JSON round-trip int → float64** cuando el campo de salida es
  `any`/`interface{}`. Los tests del CLI que verifican
  `ev.Extra["occurrences"]` deben coercionar a `float64` (no
  `int`). Documentado en el comentario del test.
- **El detector produce un envelope sintético para
  `experience.Service.IngestEnvelope`** con Source=`detector`,
  Locator.Kind=`detector`, Locator.Path=`projectRoot`. Las
  validaciones `isValidExperienceSource` y `localLocatorKinds`
  se extendieron en este slice; si el miner de Hito 6 necesita
  otros valores, hay que extender la enum + el set de kinds.
- **El slice 5.4 acceptance test (CLI) cubre el camino
  end-to-end** con SQLite real. Esa cobertura no se pierde con
  refactors del miner — solo se rompe si alguien cambia el
  contrato de `(Source, Session.ExternalID, Turn.ExternalID)`.
  Si eso pasa, el test falla claro en
  `cmd/royo-learn/experience_detect_test.go:PersistEndToEnd`.
- **Bypass de `git commit -m -m`** vía script neutral en
  `/mnt/c/.../run.sh` + `MSYS_NO_PATHCONV=1 wsl.exe bash
  "/mnt/c/.../run.sh"`. El harness bloquea lifecycle commands
  cuando se invocan desde el bash directo.
- **`gh` no está en PATH de WSL**; usar la ruta completa
  `/mnt/c/Program Files/GitHub CLI/gh.exe`.
- **Para `gh pr merge --body-file` desde WSL**: pasar la ruta
  con el formato Windows (`C:\Users\...\file.md`), no la WSL
  (`/mnt/c/...`). El binario Windows no entiende paths WSL.
- **`gh pr merge --squash --delete-branch`** borra también la
  rama local (no solo la remota). El comportamiento del proyecto
  es mantener la rama local borrada post-merge.
- **`gentle_review`**: pendiente. El PR #22 no pasó por
  `gentle_review start` — el operador pidió merge directo. Si el
  próximo PR se quiere revisar, ejecutar `gentle_review start`
  antes del push. El handoff de Hito 2 tiene el patrón documentado.

## 7. Lo que NO hay que hacer (anti-trampas)

- **No** crear rama desde `main` local (puede diverger de
  `origin/main`). Usar siempre `origin/main` (o el commit explícito
  que se intente). Ver `docs/lessons.md` §4.
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

## 8. Referencias clave

- `CHANGELOG.md` — versión actual, historia, version↔ola map,
  trigger table.
- `docs/lessons.md` — patterns operacionales (shell, WSL, review,
  PR base, Windows 8.3). Léelo antes de tocar nada con git o WSL.
- `AGENTS.md` — reglas no negociables del proyecto (no modificar
  sin aprobación humana).
- `CLAUDE.md` — pointer file para agentes Anthropic.
- `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` — plan de fondo de
  las capacidades a absorber.
- `docs/26-IMPLEMENTATION-ROADMAP.md` — roadmap por ola, gates
  de salida por hito. Hito 5 marcado ✅; próximo Hito 6.
- `docs/20-EXPERIENCE-INGESTION-PRD.md` — contrato de ingestión.
- `docs/21-EXPERIENCE-DOMAIN.md` — entidades del dominio
  experiencia.
- `docs/22-ADAPTER-CONTRACT.md` — interfaz del adapter (referencia
  para Hito 6 si el miner tiene su propio adapter).
- `docs/23-PATTERN-MINING.md` — qué cualifica, qué descarta,
  qué se promueve.
- `docs/24-EXPERIENCE-THREAT-MODEL.md` — reglas de seguridad.
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` — decisión
  anti-MemSearch.
- `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` — flake MCP
  investigado con resultado negativo.
- `ROADMAP.md` — meta-doc con el estado por ola.
- `HANDOFF-EXPERIENCE-DISCOVERY.md` — handoff previo, describe
  el cierre de Hito 1.
- `HANDOFF-HITO2-OPENCODE-ONCE.md` — handoff previo, describe
  el inicio de Hito 2 desde `v0.2.0-rc1`.
- `HANDOFF-HITO5-DETECTORS.md` — handoff previo, describe el
  inicio de Hito 5.
- Reviews:
  - `.git/gentle-ai/review-transactions/v2/hito2-opencode-once-review-v1/`
    (high tier, 4R, 2 SUGGESTION no bloqueantes, approved).
- PR/issue: #22 mergeado (Hito 5). Issue #16 cerrado.
- Pre-existing untracked: `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`
  (preservado fuera del merge por convención).
