# Registro de implementación — royo-learn

> Bitácora continua del plan `docs/PLAN-recuperacion-contrato.md`.
> Una entrada por tramo o recorrido. Se añade al final; no se reescribe la historia.

---

## Entrada 1 — Tramo 0: Congelar y caracterizar la base real

**Fecha:** 2026-07-14
**Tramo:** 0 — Congelar y caracterizar la base real
**Rama:** `fix/v019-contract-recovery`
**Commit de partida:** `60b94629fbec956e89f78e7add8c83437c021f0d`
**Restricción del tramo:** no modificar comportamiento. **Cumplida: cero líneas de
código alteradas.** El tramo solo ejecutó comandos de diagnóstico y creó dos
documentos nuevos.

### Commits creados

| Hash | Mensaje |
|------|---------|
| `d68b29c0e37f1ee83a7b6abdb798460459401925` | `docs: add baseline gap report and implementation log for tramo 0` |

Archivos añadidos por ese commit:

- `docs/BASELINE-GAP-REPORT.md`
- `docs/IMPLEMENTATION-LOG.md`

> Nota: un commit no puede contener su propio hash. `d68b29c` es el commit de
> entrega que aporta ambos documentos. Le sigue un commit menor
> (`docs: record tramo 0 baseline commit hash in implementation log`) cuyo único
> propósito es dejar ese hash registrado en esta bitácora; su hash no se cita
> aquí por la misma razón.

Ningún archivo bajo `cmd/`, `internal/` o `skills/` fue tocado. `go.mod` y
`go.sum` sin cambios.

### Comandos ejecutados y resultados reales

| Comando | Resultado |
|---------|-----------|
| `go version` | `go version go1.26.5 windows/amd64` |
| `git status` | Limpio; único elemento sin seguimiento: `.playwright-mcp/` |
| `git rev-parse HEAD` | `60b94629fbec956e89f78e7add8c83437c021f0d` |
| `git describe --tags --always` | `v0.1.9-1-g60b9462` |
| `git diff --stat main v0.1.9` | Sin salida — **`main` y `v0.1.9` son el mismo commit `a00143f`** |
| `git diff --stat v0.1.9 HEAD` | Solo 2 archivos documentales (1086 inserciones), **cero código** |
| `go mod verify` | `all modules verified` |
| `go vet ./...` | Limpio, salida `0` |
| `go build ./cmd/royo-learn` | Correcto, salida `0` |
| `go test ./...` | **FALLO** solo en `internal/buildinfo` — fallo ambiental, ver abajo |
| `go test -race ./...` | **Todos los paquetes correctos**, incluido `internal/buildinfo` |
| `royo-learn version` | `royo-learn dev` / `commit: unknown` / `built: unknown` / `go: go1.26.5` |
| `royo-learn --help` | 17 comandos listados, uno de ellos inexistente (`search`) |
| `royo-learn e2e --temp` | `{"passed": 9, "failed": 0, "total": 9}` — salida `0` |
| `royo-learn mcp-serve --profile minimal` (stdio) | `initialize` correcto; `tools/list` → **3 tools** |
| `royo-learn mcp-serve --profile standard` (stdio) | `initialize` correcto; `tools/list` → **9 tools** |
| `royo-learn mcp-serve --profile full` (stdio) | `initialize` correcto; `tools/list` → **10 tools** |

### Investigación del fallo de `internal/buildinfo`

```text
fork/exec C:\go-tmp\go-build685297445\b286\buildinfo.test.exe: Access is denied.
```

Se probaron dos redirecciones de `GOTMPDIR` (al scratchpad de la sesión y a un
`.gotmp` local del repositorio, luego eliminado). **Ambas fallaron con el mismo
error**, lo que descarta la ruta temporal concreta como causa.

Prueba decisiva: compilar el binario de test a una ruta estable y ejecutarlo.

```text
$ go test -c -o .gotmp/buildinfo.test.exe ./internal/buildinfo/
$ ./.gotmp/buildinfo.test.exe -test.v
--- PASS: TestDevelopmentMetadataDefaults (0.00s)
--- PASS: TestHumanString (0.00s)
--- PASS: TestVersionJSON (0.00s)
PASS
```

Las tres pruebas pasan. Además `go test -race ./...` aprueba el paquete.

**Clasificación: FALLO AMBIENTAL (política de ejecución / antivirus de Windows
bloqueando binarios en árboles `go-build`). NO es una brecha de código y no debe
bloquear ninguna puerta de salida futura.** En este entorno, la señal fiable para
la suite completa es `go test -race ./...`.

### Decisiones tomadas

1. **No corregir nada en este tramo.** Todos los defectos hallados —incluido el
   bloqueo de aprobación, que es crítico— se documentaron sin tocar el código,
   conforme a la restricción del Tramo 0.
2. **Registrar el fallo de `buildinfo` como ambiental**, con evidencia
   reproducible, para que no vuelva a interpretarse como brecha de código ni
   bloquee una puerta de salida.
3. **Corregir cuatro hallazgos del plan** (6, 7, 9 y 10) que estaban mal
   formulados o incompletos, en lugar de propagarlos. Se documentan con la
   formulación correcta y su evidencia en `docs/BASELINE-GAP-REPORT.md` §3.
4. **Elevar el bloqueo de aprobación a Hallazgo 14 (HALLAZGO PRINCIPAL).** El
   plan no lo contemplaba y es el defecto central del producto.
5. **Añadir cuatro hallazgos nuevos** (15 a 18) surgidos de la verificación:
   comando fantasma `search`, paquete `internal/evidence` huérfano, asimetría de
   `rollback` entre CLI y MCP, e instrucciones MCP que anuncian tools no
   registradas.
6. **Inventariar las tools MCP por observación real y no por derivación del
   código**, conduciendo el servidor por stdio con JSON-RPC 2.0.

### Hallazgos que contradicen el plan

- **Hallazgo 14 (nuevo, principal):** `publish.Service.Approve`
  (`internal/publish/approval.go:16`) tiene **cero llamadores** en todo el
  repositorio. `publish` exige `CheckApproval` cuando el preview marca
  `requires_approval` (`internal/publish/publish_op.go:62-63`), y las políticas
  lo marcan para destino compartido y `AGENTS.md`
  (`internal/publish/policy.go:13-14`). **Consecuencia: siempre que se requiera
  aprobación, publicar es imposible por toda interfaz pública.** Existe además un
  segundo cerrojo: el CLI no puede emitir las decisiones de curación
  `approve_shared_knowledge` ni `approve_agents_rule` que las políticas exigen
  (`cmd/royo-learn/main.go:756-764` frente a `internal/publish/policy.go:55,75`).
- **Hallazgo 15 (nuevo):** `search` se anuncia en el help
  (`cmd/royo-learn/main.go:109`) pero no tiene `case` en el dispatcher ni
  implementación. Devuelve código `2` con un mensaje de error que habla de
  `version --json`.
- **Hallazgo 16 (nuevo):** `internal/evidence` no lo importa nadie.
  `evidence.Redact` nunca se ejecuta. La justificación del E2E en
  `cmd/royo-learn/e2e.go:273-275` para no verificar la redacción **es falsa**.
- **Hallazgo 17 (nuevo):** `rollback` existe en CLI y falta en MCP.
- **Hallazgo 18 (nuevo):** las instrucciones de `initialize` enumeran las 10
  tools en los tres perfiles, incluidas las no registradas en ese perfil.
- **Hallazgos 6, 7, 9 y 10 del plan: corregidos.** Detalle en
  `docs/BASELINE-GAP-REPORT.md` §3 y §8.

### Estado real de la base

- **Build:** correcto.
- **`go vet`:** limpio.
- **Pruebas:** `go test -race ./...` pasa al 100 %. `go test ./...` falla solo por
  el fallo ambiental de `buildinfo`.
- **E2E:** reporta 9/9 y salida `0` — **pero es una prueba falsa**: 9 falsos
  positivos documentados en `docs/BASELINE-GAP-REPORT.md` §6.
- **Matriz de operaciones:** de 26 operaciones, **2 son `FUNCTIONAL`**
  (`doctor`, `self-update`), y ninguna de ellas pertenece al recorrido principal.
  Todo el recorrido principal (capture → curate → preview → approve → publish)
  está en `BROKEN_INTEGRATION`.

### Puerta de salida del Tramo 0

| # | Ítem | Estado |
|---|------|--------|
| 1 | Commit de baseline registrado | **PASS** |
| 2 | `docs/BASELINE-GAP-REPORT.md` completo con la matriz (26 operaciones, 8 columnas) | **PASS** |
| 3 | Comandos CLI reales inventariados | **PASS** — 17 en el help, 16 en el dispatcher; `search` identificado como fantasma |
| 4 | Tools MCP reales inventariadas por perfil | **PASS** — observado por stdio: `minimal` 3, `standard` 9, `full` 10 |
| 5 | Skills incompatibles listadas con los tools que citan | **PASS** — 7 de 7 tools citadas no existen; tabla por archivo |
| 6 | Falsos positivos del E2E documentados con línea exacta | **PASS** — 9 falsos positivos (FP-1 a FP-9) con cita literal del código |

**Resultado del Tramo 0: PASS en los 6 ítems. Sin FAIL.**

### Siguiente paso

Tramo 1 — `docs/CONTRACT-DECISIONS.md` con las diez decisiones del plan.
Se recomienda **añadir una decisión más (D11)** sobre el bloqueo de aprobación
(Hallazgo 14): qué interfaz expone la aprobación, cómo se alcanzan las decisiones
de curación `approve_shared_knowledge` y `approve_agents_rule`, y si el CLI y el
MCP deben validar el mismo conjunto de decisiones. Sin esa decisión, el Recorrido C
del Tramo 2 no tiene contrato al que implementar.

---

## Entrada 2 — Tramo 1: Fijar el contrato público

**Fecha:** 2026-07-14
**Tramo:** 1 — Fijar el contrato público
**Rama:** `fix/v019-contract-recovery`
**Commit de partida:** `08cf40c` (HEAD tras el Tramo 0)
**Restricción del tramo:** no implementar nada en código. **Cumplida: cero líneas
de código alteradas.** El único entregable es un documento.

### Commits creados

| Hash | Mensaje |
|------|---------|
| `a14f679629e7ce34c53346176d98fb8c7273424c` | `docs: fix public contract decisions D1-D14 for recovery` |

Archivo añadido: `docs/CONTRACT-DECISIONS.md`.

Verificación de la restricción:

```text
$ git diff --stat v0.1.9 HEAD -- cmd/ internal/ skills/ go.mod go.sum
(sin salida)
```

Ningún archivo bajo `cmd/`, `internal/` o `skills/` fue tocado. `go.mod` y `go.sum`
sin cambios.

### Decisiones tomadas

Catorce decisiones resueltas y fechadas: las diez del plan (D1–D10) más cuatro que
el Tramo 0 forzó (D11–D14). Cada una con Contexto (evidencia `archivo:línea`),
Opciones consideradas, Decisión, Justificación y Fecha.

| # | Decisión |
|---|----------|
| D1 | 15 tools MCP canónicas `learning_*`; 10 aliases deprecated apuntando al mismo handler, cero duplicación de lógica |
| D2 | Perfiles canónicos `read/agent/admin` con flag `--tools`; `--profile` y `minimal/standard/full` deprecated; `learning_publish` en `agent`; nada destructive en `read` ni `agent` |
| D3 | Se conserva sin cambios el umbral de evidencia ya implementado (mínimo `moderate` **y** ≥1 registro de evidencia persistido); lo que falta no es el umbral, es la entrada pública que permita satisfacerlo |
| D4 | Aprobación humana **siempre** para `AGENTS.md`, `shared`, actualización de Skill existente, reglas globales, archivos fuera del proyecto y cambios que sustituyen una regla; políticas basadas en destino, no en la decisión de curación |
| D5 | Semántica de `idempotency_key`: reintento técnico ≠ evento equivalente ≠ deduplicación conservadora |
| D6 | SQLite = fuente operacional; Markdown = derivado portable; audit log = append-only; prohibido declarar equivalencia transaccional; sin outbox salvo prueba de fallo |
| D7 | Dry-run por defecto; escribir exige `--preview-hash` + `--approval-id` + `--apply` |
| D8 | **Hito 1 = `v0.1.10`**; ninguna decisión retira nada de `v0.1.9`; aviso de deprecación obligatorio (stderr en CLI, campo `deprecation` en MCP); retiro de aliases en `v0.2.0` |
| D9 | `setup`, `recurrences`, `metrics` → documentar en docs/04; `mcp-serve` → alias de `mcp`; `engram-health` → plegar bajo `doctor`; `engram-search` → plegar bajo `search --include-engram`. Ningún comando en limbo |
| D10 | `mcp` canónico; `mcp-serve` alias deprecated |
| D11 | **Los cuatro cerrojos del bloqueo de aprobación se reparan como un único contrato.** Lista blanca canónica de decisiones de curación compartida por CLI y MCP (obligatorio). Escapatoria para `preference` + `shared`/`AGENTS.md` = aprobación humana explícita vía `learning_approve` |
| D12 | **`search` se implementa en el Hito 1** (adelantado desde el Tramo 4), no se retira del help |
| D13 | Se confirma §1.3 del plan. El Recorrido B **conecta `internal/evidence` por primera vez**, no lo reconecta. La aserción de redacción del E2E debe volverse real, no diferirse |
| D14 | La cadena `instructions` de `initialize` se deriva del registro de tools del perfil activo; nunca se codifica a mano |

### Hallazgos que corrigen el Tramo 0

La verificación de D11 obligó a releer la cadena causal completa del bloqueo de
aprobación. **El informe del Tramo 0 llega a la conclusión correcta por un camino
parcialmente equivocado y omite el cerrojo raíz.** Dos correcciones, ambas
verificadas línea a línea:

1. **Las políticas de destino compartido y `AGENTS.md` son tautologías, no
   bloqueos.** `docs/BASELINE-GAP-REPORT.md:139-148` afirma que
   `policySharedScopeRequiresApproval` (`internal/publish/policy.go:51-68`) y
   `policyAgentsRuleRequiresApproval` (`policy.go:72-88`) marcan
   `requires_approval`. **No pueden.** El destino de una curación se *deriva de la
   decisión* (`internal/curate/curate.go:276-320`), de modo que
   `destino == shared` ⟺ `decisión == approve_shared_knowledge`. La rama de fallo
   de ambas políticas es inalcanzable. **El defecto real es de signo contrario:
   publicar en `AGENTS.md` nunca exige aprobación humana.** Es un agujero de
   gobernanza, no un bloqueo.
2. **El cerrojo raíz está una etapa antes de donde el informe lo situó.**
   `curate.checkEvidenceThreshold` (`internal/curate/curate.go:190-214`) exige al
   menos un registro de evidencia persistido, y `storage.SaveEvidence`
   (`internal/storage/repo_evidence.go:13`) tiene **cero llamadores de
   producción** — solo lo invocan pruebas. **Ningún aprendizaje puede alcanzar
   `approved` por ninguna interfaz pública, ni CLI ni MCP.** Las pruebas de
   integración están verdes porque escriben la evidencia con SQL directo
   (`internal/integration/learning_flow_test.go:70`,
   `internal/integration/p1_procedure_e2e_test.go:100,137`), que es justo lo que el
   criterio de aceptación del Recorrido B prohíbe.

La conclusión del Hallazgo 14 —«siempre que `requires_approval` sea `true`,
publicar es imposible»— se mantiene y se confirma (`internal/publish/approval.go:16`
sigue teniendo cero llamadores).

### Defectos adicionales descubiertos durante el Tramo 1

Ninguno estaba en el informe del Tramo 0. Todos verificados:

- **`domain.ValidateLearning` es código muerto.** Cero llamadores de producción;
  solo `internal/domain/validation_test.go`. Su guardián de `preference` +
  `shared`/`agents_rule` (`internal/domain/validation.go:58-65`) **nunca se
  ejecuta**, y por eso el estado que `policyPreferenceTypeRequiresHuman` bloquea
  **sí es alcanzable** por MCP.
- **El CLI `capture` no tiene flag de destino.** Búsqueda de `destination` en el
  código de producción de `cmd/`: cero coincidencias. Todo aprendizaje capturado
  por CLI propone destino `project` (`internal/capture/capture.go:78-81`).
- **Dos de las cinco acciones de `curate` del CLI son estructuralmente
  inutilizables.** `--action approve_new_skill` y `--action approve_skill_update`
  siempre fallan, porque exigen `ProposedDestination == skill` y el CLI no puede
  producirlo (`internal/curate/curate.go:322-326`).
- **El CLI `capture` tampoco expone `--evidence-level`**
  (`cmd/royo-learn/main.go:459-468`), por lo que todo aprendizaje capturado por CLI
  nace `insufficient` (`internal/capture/capture.go:86-88`) y falla el umbral por
  partida doble.
- **Las pruebas de conformidad consagran el defecto de D14.**
  `internal/mcpserver/conformance_test.go:440,446-464` exige que las instrucciones
  contengan una lista fija de nombres, en lugar de exigir que coincidan con lo que
  `tools/list` devuelve.

### Patrón común

Cinco piezas del producto son **código correcto, probado y muerto**:
`storage.SaveEvidence`, `publish.Service.Approve`, `domain.ValidateLearning`,
`internal/evidence` completo, y la rama de fallo de dos de las tres políticas de
publicación. Regla extraída, que el Tramo 5 debe convertir en prueba permanente:

> Ninguna capacidad se considera existente hasta que una interfaz pública la
> invoque y una prueba de negocio observe su efecto.

### Puerta de salida del Tramo 1

| # | Ítem | Estado |
|---|------|--------|
| 1 | `docs/CONTRACT-DECISIONS.md` con todas las decisiones resueltas y fechadas | **PASS** — D1 a D14, cinco secciones cada una, todas fechadas 2026-07-14 |
| 2 | Ninguna decisión queda implícita; no se implementa nada antes de esto | **PASS** — `git diff v0.1.9 HEAD -- cmd/ internal/ skills/ go.mod go.sum` vacío |

**Resultado del Tramo 1: PASS en los 2 ítems. Sin FAIL.**

### Siguiente paso

Tramo 2, Recorrido A (Skills ↔ MCP). **Advertencia derivada de D11:** el orden
`A → B → C → D → E` del plan es correcto, pero el **Recorrido B (evidencia) es la
dependencia raíz de todo el Hito 1**, no una etapa más. Sin entrada pública de
evidencia, ningún aprendizaje alcanza `approved` y los Recorridos C, D y E no
tienen sobre qué operar.
