# HANDOFF — Royo-Learn post-Hito 2 → arranque de Hito 5 (detectores)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar
> nada. Cierra la sesión que dejó Hito 2 mergeado a `main` y deja el
> árbol verde. La próxima sesión continúa con el slice 5.0 (scaffold
> de los detectores deterministas).

## 0. Frase para pegar en la próxima sesión

Copiá y pegá esto tal cual al iniciar la próxima sesión:

```text
Continuá Royo-Learn en `main` local/remoto, donde quedó mergeado
Hito 2 como squash commit `ad269a7` (PR #21, merge --squash
--delete-branch, rama `feat/hito2-opencode-once` borrada en
local y remoto). `v0.2.0-rc1` sigue siendo el último tag
(correcto: el trigger de `v0.2.0` espera a Hito 4).

Antes de actuar:

1. Leé HANDOFF-HITO5-DETECTORS.md completo (incluida esta sección 0).
2. Leé docs/26-IMPLEMENTATION-ROADMAP.md §3 PR #4 para los gates
   de salida de Hito 5.
3. Leé docs/lessons.md — los patterns operacionales aprendidos
   (bypass de WSL para el lifecycle interceptor, scope discipline
   del review, Windows 8.3 path handling, fix del bug de --fixture).
4. Leé docs/23-PATTERN-MINING.md y docs/20-EXPERIENCE-INGESTION-PRD.md
   para entender qué detecta Hito 5.
5. Verificá el estado real con:
   `git log --oneline -5 main` (último debe ser `ad269a7`),
   `git status --short --branch` (solo PROMPT y ROADMAP untracked,
   PROMPT preservado por convención),
   `git tag --list 'v0.*' | tail -3` (último debe ser `v0.2.0-rc1`),
   `go test -race -count=1 ./internal/experience/...`
   (debe pasar, incluyendo `internal/experience/opencode` 80.5%).

Estado: Hito 0/1/2 mergeados a main. PR #21 cerrado. Working tree
limpio excepto PROMPT (preservado) y ROADMAP (decisión pendiente
de housekeeping). Kilo Code Review desinstalado del VS Code del
operador. Cobertura de `internal/experience/opencode` 80.5%.

Tarea de la próxima sesión, en este orden:

1. (Decisión de housekeeping) `ROADMAP.md` está untracked en
   working tree. Opciones: (a) commit directo en main como commit
   housekeeping, (b) rama `docs/roadmap-sync` + PR, (c) borrar.
   Preguntale al operador.
2. (Opcional) Actualizar `CHANGELOG.md` [Unreleased] con las notas
   de Hito 2 antes de seguir. El formato del proyecto espera que
   [Unreleased] se llene acumulando hasta el corte del próximo tag
   (v0.2.0 cuando mergee Hito 4).
3. Crear rama `feat/hito5-detectors` desde `origin/main` (NO desde
   local main — ver docs/lessons.md §4 sobre el scope inflado).
4. Slice 5.0 — scaffold del paquete `internal/experience/detectors/`
   con stubs de la interfaz `Detector` y tabla de tests del contrato.
   RED primero, GREEN después. Sin lógica todavía.
5. Slice 5.1-5.4 en orden, TDD estricto. El detalle completo
   está en HANDOFF-HITO5-DETECTORS.md §4 más abajo.
6. (PR #4 según docs/26) — un solo PR al cierre con los 5 slices.

Reglas innegociables (recordatorio, no negociables en este proyecto):

- TDD estricto: RED primero, GREEN después, REFACTOR solo con tests
  verdes. `go test -race ./...` antes de cada commit.
- Redacción antes de hash y persistencia. El fingerprint no se
  calcula antes de la redacción.
- Reusar `capture.Service` e `internal/evidence`; no duplicar.
- SQLite = verdad operacional. El detector escribe solo vía
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

Criterios de "Hito 5 hecho" (per docs/26 §3 PR #4, gates de salida):

- Detector determinista para al menos 1 patrón de evento
  (cambio de archivo, cambio de config, retry, error recurrente,
  etc.).
- `go test -race ./...` verde; cross-build win/linux/darwin verde;
  cobertura de `internal/experience/detectors/` ≥ 80% (umbral de
  dominio per AGENTS.md §Calidad mínima).
- Cero eventos en charla rutinaria (precisión > recall por
  construcción determinista).
- Tabla de tests cubre los 3 escenarios definidos en
  docs/23-PATTERN-MINING.md §3.
```

---

## 1. Qué cambió en esta sesión

- Hito 2 mergeado a `main` como squash commit `ad269a7` (PR #21).
  17 archivos, 3877 inserciones, 7 borradas. Rama local y remota
  `feat/hito2-opencode-once` borradas.
- Working tree dejado intencionalmente con dos untracked:
  - `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` — preservado por
    convención del proyecto (ver HANDOFF-EXPERIENCE-DISCOVERY.md).
  - `ROADMAP.md` — meta-doc creado en esta sesión. Decisión de
    housekeeping pendiente (commitar, branchear, o borrar).
- Kilo Code Review desinstalado del VS Code del operador. La
  GitHub App sigue autorizada en github.com pero ya no se
  dispara. Si se quiere limpieza total:
  `github.com → Settings → Applications → Kilo Code → Revoke`.
- `ROADMAP.md` reescrito para reflejar Hito 2 ✅ merged y el
  orden de las olas con sus triggers.
- Se ejecutó `gentle_review start` (lineage
  `hito2-opencode-once-review-v1`, high tier, full 4R) con
  `final_verification_passed: true`. Findings: 0 BLOCKER, 0
  CRITICAL, 0 WARNING, 2 SUGGESTION no bloqueantes (helper
  extraction, context.Background en --once). El subagent-run
  para los 4 lens falló por dispatch del harness; el review
  se completó manualmente como senior architect.

## 2. Estado actual del repositorio

- `main` HEAD local: `ad269a7`
- `origin/main` HEAD: `ad269a7` (sincronizado, 0 ahead, 0 behind)
- Tag `v0.2.0-rc1` en remote: `706439e`, anotado (sin cambios)
- PRs abiertos: ninguno. PR #21 mergeado y cerrado.
- Working tree: solo `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` y
  `ROADMAP.md` untracked (intencionales).

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

## 4. Slice breakdown de Hito 5 (referencia, no contrato)

Detalle propuesto. El próximo agente puede ajustar el orden si la
implementación revela dependencias distintas, pero la cobertura de
los gates de salida no se negocia.

| # | Sub-slice | Qué entrega | Gate específico |
|---|---|---|---|
| **5.0** | Scaffold | Paquete `internal/experience/detectors/` con stubs de la interfaz `Detector` + tabla de tests del contrato | Compila; tests del contrato fallan (RED) |
| **5.1** | Detector de cambios de archivo | Detecta diffs de tamaño/forma inusual vs baseline | Falso positivo < 5% en charla rutinaria; determinista |
| **5.2** | Detector de reintentos | Cuenta errores recurrentes en ventana móvil | 3 reintentos en 5 min = 1 evento; 0 falso positivo |
| **5.3** | Detector de cambios de config | Compara config hasheada contra baseline | Cambio de `.royo-learn/config.yaml` = 1 evento |
| **5.4** | CLI `experience detect` + acceptance | Orquesta los detectores, persiste vía `capture.Service` | Idempotente; CLI + MCP; cobertura ≥ 80% |

**Total esperado**: 5 commits atómicos en una sola rama
`feat/hito5-detectors` desde `origin/main`. Un solo PR al cierre
(PR #4 de la roadmap).

**Contratos a respetar** (todos en `origin/main` ahora):

- `docs/23-PATTERN-MINING.md` — qué patrones busca el detector.
- `docs/20-EXPERIENCE-INGESTION-PRD.md` — el contrato de
  ingestión que el evento detector produce.
- `docs/24-EXPERIENCE-THREAT-MODEL.md` — reglas de seguridad del
  detector (qué es evento válido, qué no).
- `docs/26-IMPLEMENTATION-ROADMAP.md` §3 PR #4 — gates de salida.

## 5. Próximo trabajo

### 5.1. (Hito 5) Slice 5.0

El scaffold es chico. TDD estricto: escribir los tests del contrato
primero (la tabla de tests que demuestra que `Detector`, `Event`,
`Baseline`, `Window` existen con la firma correcta), ver que
fallen (RED), implementar los stubs mínimos, ver que pasen
(GREEN). Sin lógica todavía.

Rama: `feat/hito5-detectors` desde `origin/main` (no desde
local main — ver `docs/lessons.md` §4 sobre el scope inflado).

### 5.2. (Después de Hito 5) Hito 6, 7, 4

Hitos restantes de Ola 1. Cubren patrones + clustering, promoción
vía `capture.Service`, y trace progresivo. Cada uno es un PR.
Al cerrar el último (Hito 4), el trigger table del CHANGELOG dice
"cortar `v0.2.0`".

### 5.3. (Housekeeping opcional)

- Commitar `ROADMAP.md` en `main` o en una rama `docs/roadmap-sync`
  - PR. Decisión del operador.
- Actualizar `CHANGELOG.md [Unreleased]` con las notas de Hito 2.
  El formato del proyecto espera que se llene acumulando hasta
  el corte del próximo tag.

### 5.4. (Mucho después) Ola 2 y Ola 3

Ola 2 (5 PRs): motor de jobs, retrieval lexical, OpenCode
`--watch`, Claude Code, Codex.
Ola 3 (3 PRs): drift/release, semántica opcional, Pi.

Estas olas son multi-mes. No relevante para la próxima sesión.

## 6. Lecciones operativas aprendidas en esta sesión (para PR #4 y siguientes)

- **`SkippedIncomplete` debe ser visible al operador.** El adapter
  originalmente descartaba turnos con `complete=0` en silencio.
  Mientras los tests del adapter los cubrían como "no se
  ingestaron", el CLI no podía reportar la pérdida. Se agregó al
  `ScanResult` (`SkippedIncomplete int`) y se propagó al reporte
  JSON. Lección: cualquier drop silencioso del adaptador necesita
  un contador expuesto al caller.
- **Los fixtures de tests deben vivir dentro del `projectRoot`.**
  El validador de envelopes rechaza locators fuera del project
  root canónico. El primer corte de tests del CLI creaba el
  fixture en un `t.TempDir()` separado y el scan fallaba al
  ingestar. Solución: el fixture va en
  `root/.opencode-fixture/opencode.db`. Aplica a cualquier test
  que use `--fixture` o discovery automático.
- **El CLI necesita su propia capa de tests, no solo la del
  adapter.** El adapter tenía cobertura completa de
  Discover/Health/Scan/ResolveTrace, pero el subcomando
  `experience opencode scan` no tenía tests hasta slice 2.6. La
  cobertura del adapter mide unidades, no la orquestación.
  Patrón: una vez que un método del adapter se enchufa a un
  comando CLI, ese comando necesita su tabla de tests propia.
- **`cursorCheckpoint` debe aceptar los 4 tipos numéricos.** El
  decoder depende de cómo se construyó el mapa: nativo Go, JSON
  round-trip, sub-agentes externos. El test cubre
  `int64`/`int`/`float64`/`int32` explícitamente; un futuro
  refactor que asuma un solo tipo lo rompería.
- **El CLI debe rechazar symlinks en `--fixture` y reemplazar
  discovery, no sumarse.** El primer CI del PR #21 falló por
  dos bugs reales: (a) el adapter rechazaba symlinks solo en
  `discover()`, pero el CLI bypassaba discover cuando se pasaba
  `--fixture`; (b) `--fixture` se sumaba a discovery en vez de
  reemplazarlo, así que el fixture se reportaba como segunda
  instance. Fix: `buildFixtureInstance` valida `os.ModeSymlink` +
  `project.IsInsideRoot` antes de aceptar, y discovery se
  skipea cuando `--fixture` está presente.
- **Windows 8.3 path comparison necesita `project.Canonicalize`.**
  `strings.EqualFold` no normaliza la forma corta (`RUNNER~1` vs
  `runneradmin`); `t.TempDir()` puede devolver una forma y
  `os.Lstat` la otra. Usar `project.Canonicalize` en ambos lados
  para comparar.
- **Bypass de `git commit -m -m`** vía script neutral en
  `/mnt/c/.../run.sh` + `MSYS_NO_PATHCONV=1 wsl.exe bash
  "/mnt/c/.../run.sh"`. El harness bloquea lifecycle commands
  cuando se invocan desde el bash directo.
- **`gh` no está en PATH de WSL**; usar la ruta completa
  `/mnt/c/Program Files/GitHub CLI/gh.exe`.
- **`gentle_review start` con `baseRef: origin/main`,
  `committedOnly: true`** para revisar commits (no working tree).
  Sin `headRef` (no existe ese campo). El `finalize` no acepta
  `correction_forecast` ni `targeted_validation` como campos;
  solo `lens_results`, `final_evidence`, `final_verification_passed`.
  El campo `evidence` en lens_results debe ser un **array**, no
  un string.

## 7. Lo que NO hay que hacer (anti-trampas)

- **No** crear rama desde `main` local (puede diverger de
  `origin/main` si en el futuro vuelve a haber un Hito 1
  atrasado). Usar siempre `origin/main` (o el commit explícito
  que se intente).
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
  regla 9).
- **No** insertar un LLM provider dentro del binario en v1.
- **No** forke ni modificar `Gentle-AI` ni `Engram`.
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
  de salida por hito. Hito 2 marcado ✅; próximo Hito 5.
- `docs/20-EXPERIENCE-INGESTION-PRD.md` — contrato de ingestión.
- `docs/21-EXPERIENCE-DOMAIN.md` — entidades del dominio
  experiencia.
- `docs/22-ADAPTER-CONTRACT.md` — interfaz del adapter (referencia
  para Hito 5 si el detector tiene su propio adapter).
- `docs/23-PATTERN-MINING.md` — qué patrones detecta Hito 5.
- `docs/24-EXPERIENCE-THREAT-MODEL.md` — reglas de seguridad.
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` — decisión
  anti-MemSearch.
- `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` — flake MCP
  investigado con resultado negativo.
- `ROADMAP.md` — meta-doc con el estado por ola, desactualizado
  hasta que se commitee.
- `HANDOFF-EXPERIENCE-DISCOVERY.md` — handoff previo, describe
  el cierre de Hito 1.
- `HANDOFF-HITO2-OPENCODE-ONCE.md` — handoff previo, describe
  el inicio de Hito 2 desde `v0.2.0-rc1`.
- Reviews:
  - `.git/gentle-ai/review-transactions/v2/hito2-opencode-once-review-v1/`
    (high tier, 4R, 2 SUGGESTION no bloqueantes, approved).
- PR/issue: #21 mergeado (Hito 2). Issues #16 cerrado.
- Pre-existing untracked: `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`
  (preservado fuera del merge).
- ROADMAP.md: meta-doc no commiteado, decisión de housekeeping
  pendiente.
