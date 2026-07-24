# HANDOFF — Royo-Learn post `v0.2.0-rc1` (arranque de Hito 2)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar nada.
> Cierra la sesión que dejó `v0.2.0-rc1` cortado y `Hito 2` (OpenCode `--once`)
> listo para arrancar. Push de `main` ya ejecutado; tag ya pusheado. Próxima
> sesión continúa con el slice 2.0 (scaffold del adaptador).

## 0. Frase para pegar en la próxima sesión

Copiá y pegá esto tal cual al iniciar la próxima sesión:

```text
Continuá Royo-Learn en `main` local/remoto, donde quedó cortado
`v0.2.0-rc1` (commit `355e173`, tag `706439e`). Antes de actuar:

1. Leé HANDOFF-HITO2-OPENCODE-ONCE.md completo (incluida esta sección 0).
2. Leé CHANGELOG.md — la sección [0.2.0-rc1] describe lo que ya está
   shipped, y el "Version ↔ Ola map" más abajo muestra la ruta
   versionada hasta v1.0.0.
3. Leé docs/lessons.md — patterns operacionales aprendidos (shell
   detection, WSL bypass para el lifecycle interceptor, scope del
   candidate view, base correcta para `gh pr create`).
4. Leé docs/21-EXPERIENCE-DOMAIN.md y docs/22-ADAPTER-CONTRACT.md
   antes de tocar código. Son los contratos congelados de Hito 0 que
   el adaptador OpenCode tiene que implementar.
5. Leé docs/26-IMPLEMENTATION-ROADMAP.md §3 PR #3 para los gates
   de salida de Hito 2.
6. Verificá el estado real con:
   `git log --oneline -5 main`,
   `git status --short --branch`,
   `git rev-list --count origin/main..main` (debe ser 0),
   `git tag --list 'v0.*' | tail -3`,
   `go test -race -count=1 ./internal/experience/...` (debe pasar).

Estado: Hito 1 (experience discovery) está merged a `main` y cortado
como `v0.2.0-rc1` en el remote (tag `706439e`). `origin/main` y
`local main` están sincronizados en `355e173`. El CHANGELOG describe
todo el Hito 1 en la entrada `[0.2.0-rc1] - 2026-07-24` y la
sección [Unreleased] está vacía esperando Hito 2.

Working tree: solo `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked,
preservado intencionalmente fuera del merge (decisión previa, no
tocar salvo que el usuario pida).

Tarea de la próxima sesión, en este orden:

1. Crear rama `feat/hito2-opencode-once` desde `origin/main` (no
   desde local main, para evitar el mismo problema de scope inflado
   que tuvimos con PR #18 — ver docs/lessons.md §4).
2. Slice 2.0 — scaffold del paquete `internal/experience/opencode/`
   con stubs de la interfaz `ExperienceAdapter` (definida en
   docs/22-ADAPTER-CONTRACT.md §1) y tabla de tests del contrato.
   RED primero, GREEN después. Sin lógica todavía.
3. Slice 2.1 — `Discover()`: encuentra instancias de OpenCode en
   roots permitidas (sin symlinks). Verifica path security per
   docs/24-EXPERIENCE-THREAT-MODEL.md.
4. Slice 2.2 — `Health()`: DB existe, es legible, esquema esperado
   (read-only check, no muta la fuente).
5. Slices 2.3-2.7 en orden, TDD estricto. El detalle completo está
   en HANDOFF-HITO2-OPENCODE-ONCE.md §4 más abajo.

Reglas innegociables (recordatorio, no negociables en este proyecto):

- TDD estricto: RED primero, GREEN después, REFACTOR solo con tests
  verdes. `go test -race ./...` antes de cada commit.
- Redacción antes de hash y persistencia. El fingerprint no se
  calcula antes de la redacción.
- Reusar `capture.Service` e `internal/evidence`; no duplicar.
- SQLite = verdad operacional. El adaptador es read-only sobre la
  fuente OpenCode, escribible solo sobre la DB de Royo-Learn.
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

Criterios de "Hito 2 hecho" (per docs/26 §3 PR #3, gates de salida):

- Lee fixture SQLite de OpenCode anonimizada.
- Ignora turnos incompletos.
- Reinicio no duplica (idempotencia por source + external IDs).
- Cero side effects en la fuente OpenCode.
- Path fuera de raíz bloqueado (incluye rechazo de symlinks).
- `go test -race ./...` verde; cross-build windows/linux/darwin
  verde; cobertura de `internal/experience/opencode/` ≥ 80% (umbral
  de dominio per AGENTS.md §Calidad mínima).
```

---

## 1. Qué cambió en esta sesión

- Investigamos ADR-0002 §4 con 40 iteraciones (count=20 en HEAD + count=20
  en base `4fe9774` vía worktree efímero). Resultado: el flake
  `ListTools: context deadline exceeded` no se reproduce en este
  ambiente. `internal/mcpserver` es bit-identical entre base y HEAD.
  ADR-0002 sigue `Proposed` con un §7 nuevo describiendo el resultado
  negativo. Documentado en `docs/IMPLEMENTATION-NOTES.md:466` per
  ADR §4.4. Commit `f58a33b`.
- Refrescamos el HANDOFF anterior (Hito 1 closure) y lo committeamos
  (`9afb09a`).
- Cerramos la grieta documental con PR #19: trajimos `docs/20-26`,
  `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`, y `ADR-0001` desde
  `d812709` a `main`. Merge commit `bc930b3`.
- Creamos `CHANGELOG.md`, `docs/lessons.md`, `CLAUDE.md`, y la
  referencia cruzada en `AGENTS.md` como PR #20. Merge commit
  `4ab9762`. El CHANGELOG incluye el "Version ↔ Ola map" que
  formaliza la ruta versionada.
- Pusheamos `main` por primera vez (21 commits de Hito 1 + los
  PRs mergeados) con merge de sincronización `21c0944`.
- Cortamos `v0.2.0-rc1` (tag anotado `706439e`) apuntando al main
  con el CHANGELOG actualizado. El tag incluye todo Hito 1, los
  contratos congelados, las lecciones operacionales, y la
  documentación meta.
- No compilamos binarios cross-platform; no creamos un GitHub
  Release con assets adjuntos. El tag existe en el remote pero los
  binarios no.

## 2. Estado actual del repositorio

- `main` HEAD local: `355e173` (sincronizado con `origin/main`).
- `origin/main` HEAD: `355e173`.
- Tag `v0.2.0-rc1` en remote: `706439e`, anotado.
- `v0.1.10` sigue siendo el último release "production-tagged"
  previo (sin Hito 1).
- Working tree: solo `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked
  (preservado fuera del merge, decisión previa, no tocar).
- PRs abiertos: ninguno. Los 2 PRs de esta sesión (`#19`, `#20`)
  están mergeados.

## 3. Invariantes innegociables (recordatorio)

- Go + SQLite como núcleo; SQLite es la verdad operacional.
- Redacción **antes** de hash y persistencia.
- Experiencia observada ≠ conocimiento aprobado.
- Promoción únicamente vía `capture.Service`.
- Sin Python/Bash/`os.system`/shell en runtime; sin red obligatoria;
  sin daemon.
- Preservar CLI/MCP actuales, JSON estable, Windows/Linux/macOS.
- Preview hash, aprobación, publicación atómica, verificación,
  rollback intactos.
- Un hito por PR; commits por unidad de trabajo; conventional commits,
  sin atribución de IA.
- Toda publicación compartida o cambio de `AGENTS.md` requiere
  aprobación humana verificable.
- El path-mangling de `wsl.exe` desde Git Bash requiere
  `MSYS_NO_PATHCONV=1` (ver `docs/lessons.md` §2).

## 4. Slice breakdown de Hito 2 (referencia, no contrato)

Detalle propuesto. El próximo agente puede ajustar el orden si la
implementación revela dependencias distintas, pero la cobertura de
los gates de salida no se negocia.

| # | Sub-slice | Qué entrega | Gate específico |
|---|---|---|---|
| **2.0** | Scaffold | Paquete `internal/experience/opencode/` con stubs de `ExperienceAdapter` + tabla de tests del contrato | Compila; tests del contrato fallan (RED) |
| **2.1** | `Discover()` | Encuentra instancias de OpenCode en roots permitidas (sin symlinks) | Path fuera de raíz → error tipado; symlink → error tipado |
| **2.2** | `Health()` | DB existe, es legible, esquema esperado, read-only | No muta la DB; reporta health en JSON estable |
| **2.3** | `Scan()` | Lee sesiones + turnos, construye `ExperienceEnvelope`, ignora incompletos | Turno sin `complete=true` → skip; fingerprint estable |
| **2.4** | Idempotencia | `(source, external_session_id, external_turn_id)` único, cursor estable | Segunda corrida con misma fixture no duplica filas |
| **2.5** | `ResolveTrace()` | Excerpt por locator sin ejecutar contenido | Output máximo `OutputHint` (no contenido completo); secrets redactados |
| **2.6** | CLI `experience opencode scan --once` | Orquesta el adaptador + pasa al núcleo por CLI/MCP | Flags `--once`, `--project-root`, `--fixture`; JSON estable en stdout; errores tipados en stderr |
| **2.7** | Acceptance + docs | Tests de seguridad (path traversal, symlinks, secretos), `docs/26` update con lecciones del PR | `-race` verde; cross-build verde; cobertura ≥ 80% |

**Total esperado**: 8 commits atómicos en una sola rama
`feat/hito2-opencode-once` desde `origin/main`. Un solo PR al
cierre (PR #3 de la roadmap).

**Contratos a respetar** (todos en `origin/main` ahora):

- `docs/21-EXPERIENCE-DOMAIN.md` — entidades `ExperienceSource`,
  `ExperienceSession`, `ExperienceTurn`, `TranscriptLocator`.
- `docs/22-ADAPTER-CONTRACT.md` — interfaz `ExperienceAdapter`,
  `ExperienceEnvelope`, "El adaptador NO puede" lista.
- `docs/24-EXPERIENCE-THREAT-MODEL.md` — reglas de seguridad de
  path.
- `docs/26-IMPLEMENTATION-ROADMAP.md` §3 PR #3 — gates de salida.

## 5. Próximo trabajo (cola)

### 5.1. (Si la próxima sesión arranca con Hito 2) Slice 2.0

El scaffold es chico. TDD estricto: escribir los tests del contrato
primero (la tabla de tests que demuestra que los 4 métodos de
`ExperienceAdapter` existen con la firma correcta), ver que fallen
(RED), implementar los stubs mínimos, ver que pasen (GREEN). Sin
lógica todavía.

Rama: `feat/hito2-opencode-once` desde `origin/main` (no desde
local main — ver `docs/lessons.md` §4 sobre el scope inflado).

### 5.2. (Opcional, antes de Hito 2) Binarios de v0.2.0-rc1

El tag existe pero no hay binarios compilados. Si alguien quiere
instalar v0.2.0-rc1 hoy, tiene que compilar del tag con
`go build`. GoReleaser puede producir los 6 binarios
(3 OS × 2 arch) en ~10 min. Es trabajo mecánico, no creativo. Si
el usuario lo pide, ejecutar:

```bash
git checkout v0.2.0-rc1
goreleaser release --snapshot --clean
# Subir los binarios al GitHub Release manualmente o via workflow.
```

### 5.3. (Después de Hito 2) Hito 5, 6, 7, 4

Hitos restantes de Ola 1. Cubren detectores deterministas,
patrones + clustering, promoción vía `capture.Service`, y trace
progresivo. Cada uno es un PR.

Al cerrar el último (Hito 4), el trigger table del CHANGELOG dice
"cortar `v0.2.0`". En ese momento el Version ↔ Ola map queda
íntegramente ratificado.

### 5.4. (Mucho después) Ola 2 y Ola 3

Ola 2 (5 PRs): motor de jobs, retrieval lexical, OpenCode
`--watch`, Claude Code, Codex.
Ola 3 (3 PRs): drift/release, semántica opcional, Pi.

Estas olas son multi-mes. No relevante para la próxima sesión.

## 6. Referencias operacionales

- `CHANGELOG.md` — versión actual, historia, version↔ola map,
  trigger table.
- `docs/lessons.md` — 4 patterns operacionales (shell, WSL,
  review, PR base). Léelo antes de tocar nada con git o WSL.
- `AGENTS.md` — reglas no negociables del proyecto (no modificar
  sin aprobación humana).
- `CLAUDE.md` — pointer file para agentes Anthropic.
- `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` — plan de fondo de las
  capacidades a absorber.
- `docs/26-IMPLEMENTATION-ROADMAP.md` — roadmap por ola, gates de
  salida por hito.
- `docs/21-EXPERIENCE-DOMAIN.md` y `docs/22-ADAPTER-CONTRACT.md` —
  contratos congelados del adaptador.
- `docs/24-EXPERIENCE-THREAT-MODEL.md` — reglas de seguridad.
- `docs/ADR-0001-NO-MEMSEARCH-RUNTIME.md` — decisión de no
  MemSearch runtime.
- `docs/ADR-0002-MCP-LISTTOOLS-TIMEOUT.md` — flake MCP investigado
  con resultado negativo.
- `HANDOFF-EXPERIENCE-DISCOVERY.md` — handoff previo, ya
  commiteado. Describe el cierre de Hito 1.

## 7. Lo que NO hay que hacer (anti-trampas)

- **No** crear rama desde `main` local (20 commits ahead está bien
  sincronizado ahora, pero si en el futuro vuelve a divergir, el
  PR se infla. Usar siempre `origin/main`).
- **No** abrir PR sin que el usuario lo pida explícitamente.
- **No** modificar `AGENTS.md` sin aprobación humana.
- **No** commitear el `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` untracked.
- **No** "arreglar" el flake MCP reintroduciendo retry masking o
  relajando timeouts — ADR-0002 §2.2 lo prohíbe explícitamente.
- **No** usar embeddings o base vectorial en v1 (AGENTS.md regla 9).
- **No** insertar un LLM provider dentro del binario en v1.
- **No** forke ni modificar `Gentle-AI` ni `Engram`.
- **No** escribir en bases internas de terceros.
- **No** capturar conversaciones completas por defecto.
- **No** guardar razonamiento privado del modelo.
- **No** modificar globalmente Codex, OpenCode o Claude sin backup
  y consentimiento.
- **No** generar instaladores que descarguen y ejecuten código no
  fijado sin mostrar procedencia.
- **No** sustituir pruebas por mocks cuando el comportamiento real
  pueda verificarse localmente.
- **No** eliminar aprendizajes; usar estados `rejected`,
  `superseded` o `archived`.
