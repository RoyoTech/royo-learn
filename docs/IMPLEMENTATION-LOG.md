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

---

## Tramo 2 — Recorrido A: Skills y MCP hablan el mismo idioma

**Fecha:** 2026-07-14
**Rama:** `fix/v019-contract-recovery`
**Commit de partida:** `a2cc98d` (cierre del Tramo 1)

Primer tramo que toca código. Los Tramos 0 y 1 fueron exclusivamente documentales.

### Commits

| Hash | Mensaje | Rol |
|------|---------|-----|
| `32def99` | `test: expose Skills-MCP contract mismatch and hardcoded instructions` | **Prueba roja.** Único commit del recorrido que no compila contra el código de producción. |
| `4797388` | `feat(mcp): add canonical learning_* tools with deprecated aliases` | Implementación mínima que la pone en verde. |
| *(este)* | `docs: record D15, D16 and Recorrido A in the recovery log` | Documentación. |

### Punto de partida verificado

La intersección entre las tools que citan las Skills y las que registra el
servidor MCP era **vacía**: 0 aciertos de 7 (`docs/BASELINE-GAP-REPORT.md:325`).
Ninguna Skill del repositorio podía invocar el servidor.

### Las dos pruebas obligatorias

Ambas viven en `internal/mcpserver/contract_test.go` y son permanentes (Tramo 5).

1. **`TestContract_SkillsCiteOnlyRegisteredCanonicalTools`** — recorre
   `skills/**/SKILL.md`, extrae cada nombre de tool MCP citado, consulta el
   registro real y verifica que cada nombre (a) existe, (b) pertenece al perfil
   que la Skill declara en su frontmatter (`mcp_profile`), y (c) **no** es un
   alias deprecated. Sin excepciones, sin lista de pendientes.
2. **`TestContract_DocsRegistrySkillsTripleMatch`** — triple coincidencia entre
   `docs/05-MCP-SPEC.md`, el registro y las Skills. La lista `pendingTools` de
   tools documentadas y aún no registradas es **exacta y autoinvalidante**:
   registrar una sin retirarla de la lista rompe la build, y añadir a la lista un
   nombre que docs/05 no documenta también. Solo puede encogerse.

Seis pruebas de contrato adicionales cubren D1, D2, D8 y D14.

### Verificación de que las pruebas no son vacuas

Se mutó el código deliberadamente para comprobar que las pruebas **fallan** cuando
deben. Una prueba que no falla ante su propia violación no prueba nada (D11 §11.4).

| Mutación | Resultado |
|----------|-----------|
| La Skill vuelve a citar `learning_approve` | **FALLA** — `skill "publish-learning" cites MCP tool "learning_approve", which is NOT registered by the server` |
| La Skill cita el alias `capture_learning` | **FALLA** — `cites deprecated alias "capture_learning"; it must cite the canonical name "learning_capture"` |
| Las instrucciones prometen `learning_publish` en todos los perfiles (el defecto D14) | **FALLA** — `profile "agent": initialize instructions announce "learning_publish", which is NOT registered in this profile` |

### Matriz final de tools y perfiles

10 tools canónicas, 10 aliases deprecated. Cada alias es una **vinculación de
nombre** al MISMO handler (`bind()` en `internal/mcpserver/profiles.go`); la ruta
del alias solo decora la respuesta con el aviso de deprecación de D8. Cero
duplicación de lógica.

| Tool canónica | Alias deprecated (v0.1.9) | Anotación | `read` | `agent` | `admin` |
|---------------|---------------------------|-----------|:------:|:-------:|:-------:|
| `learning_search` | `search_learnings` | read | ✅ | ✅ | ✅ |
| `learning_get` | `get_learning` | read | ✅ | ✅ | ✅ |
| `learning_list` | `list_learnings` | read | ✅ | ✅ | ✅ |
| `learning_doctor` | `doctor` | read | ✅ | ✅ | ✅ |
| `learning_capture` | `capture_learning` | write | — | ✅ | ✅ |
| `learning_curate` | `curate_learning` | write | — | ✅ | ✅ |
| `learning_publication_preview` | `preview_publication` | write | — | ✅ | ✅ |
| `learning_list_recurrences` | `list_recurrences` | read | — | ✅ | ✅ |
| `learning_compute_metrics` | `compute_metrics` | read | — | ✅ | ✅ |
| `learning_publish` | `publish_learning` | write | — | — | ✅ |

Perfil por defecto: `agent` — mismo conjunto de tools que el `standard` de v0.1.9.
Flag canónico `--tools read|agent|admin`; `--profile` y los valores
`minimal|standard|full` siguen funcionando y **avisan en stderr**, nunca en
silencio (D8).

### Decisiones tomadas

- **D15 — `learning_approve` no se registra a medias.** Las Skills citaban
  `learning_approve`; el handler **no existe**. `publish.Service.Approve`
  (`internal/publish/approval.go:16`) es código muerto sin llamadores, y el
  contrato real de la aprobación (vinculación al `preview_hash`, expiración,
  invalidación ante las seis condiciones) es trabajo del Recorrido C.
  **Se retiró el paso de la Skill** en lugar de registrar una tool de fachada.
  Registrar un `approve` a medio construir es exactamente el defecto que esta
  recuperación repara: declarar que una capacidad existe antes de que funcione.
  El Recorrido C restituye el paso **y** la tool en la misma entrega.
- **D16 — Aclaración de una contradicción interna de D14.** D14 exige a la vez
  que los aliases «no se anuncien» y que el conjunto anunciado sea «exactamente
  igual al que devuelve `tools/list`». Un alias registrado **aparece** en
  `tools/list`; ninguna implementación satisface ambas reglas. Se documentó y se
  resolvió: la igualdad se lee sobre las tools **canónicas**. No se resolvió en
  silencio.

### Consecuencias registradas (no accidentales)

- **`learning_publish` permanece en `admin`.** D2 lo sitúa en `agent`, pero su
  cláusula vinculante lo prohíbe hasta que existan las políticas por destino y
  `learning_approve` (D11), que son el Recorrido C. Se aplica la rama que D2
  previó.
- **El perfil `read` (ex `minimal`) estrecha su conjunto.** `minimal` servía
  `capture_learning`, una **escritura**, y ocultaba `get` y `list`, que son
  lecturas. `docs/04-CLI-SPEC.md:234-242` define `read` como «búsqueda y get». La
  compatibilidad que D8 garantiza es que el nombre `minimal` siga funcionando —y
  sigue—, no que su conjunto de tools sea inmutable.

### Fuera de alcance, deliberadamente

No se tocó `internal/evidence`, `SaveEvidence`, las tautologías de
`internal/publish/policy.go`, ni los soft-passes del E2E. Son los Recorridos B, C,
D y E. Siguen rotos y siguen documentados.

### Comandos ejecutados — resultados reales

```text
$ go build ./...
exit=0

$ go vet ./...
exit=0

$ go test -count=1 -p 1 ./...          # serie: elimina la contención del antivirus
ok  	agent-royo-learn/cmd/royo-learn	20.966s
ok  	agent-royo-learn/internal/capture, config, curate, doctor, domain, engram,
    evidence, integration, logging, mcpserver, project, publish, recurrence,
    selfupdate, setup, storage, testutil       (todos ok)
FAIL	agent-royo-learn/internal/buildinfo [build failed]
     └─ open C:\go-tmp\...\buildinfo.test.exe: Access is denied.

$ go test -race ./...
todos los paquetes ok (incluido internal/buildinfo)

$ gofmt -l ./cmd ./internal
(vacío)
```

**Sobre el fallo ambiental de esta máquina Windows — caracterización precisa.**
El informe de partida lo describe como un fallo fijo de `internal/buildinfo`. La
observación real es más amplia y conviene registrarla, porque una lectura ingenua
de un `FAIL` rojo puede confundirse con una regresión:

- En ejecución **paralela** (`go test ./...`, por defecto), el antivirus bloquea
  binarios de prueba y directorios temporales de forma **no determinista**, y la
  víctima **cambia en cada ejecución**: se observaron `internal/buildinfo`
  (`Access is denied`), `internal/selfupdate` y `internal/setup`
  (`TempDir RemoveAll cleanup: the directory is not empty`), e `internal/doctor`
  (`build failed`).
- **El commit de partida `a2cc98d` falla igual.** Ejecutando `go test -count=1 ./...`
  sobre él, la víctima fue `internal/doctor [build failed]` — un paquete que este
  recorrido no toca. Verificado en un worktree limpio del commit base.
- Ejecutando **en serie** (`-p 1`) o **con `-race`**, todo pasa salvo, como mucho,
  `internal/buildinfo`.
- Los paquetes afectados (`selfupdate`, `setup`, `doctor`, `buildinfo`) **no los
  modifica este recorrido**, y aprobados en aislamiento pasan. Los fallos son de
  *cleanup* y de *build*, nunca de aserción: ninguna prueba de negocio falla.

Conclusión: es contención de antivirus, no un defecto de código, y **no lo
introduce este recorrido**. Queda registrado con su evidencia en lugar de
despacharse como «fallo conocido».

### Puerta de salida del Recorrido A

| # | Criterio | Estado |
|---|----------|--------|
| 1 | Una Skill nunca puede citar una tool inexistente | **PASS** — `TestContract_SkillsCiteOnlyRegisteredCanonicalTools`, verificada por mutación |
| 2 | Prueba Skills ↔ registro real (existe, perfil, no-alias) | **PASS** |
| 3 | Prueba de triple coincidencia docs/05 ↔ registro ↔ Skills | **PASS** |
| 4 | `go test ./...` limpio | **PASS** — salvo la contención de antivirus de esta máquina, que también afecta al commit de partida y no toca ningún paquete de este recorrido |
| 5 | `go vet ./...` limpio | **PASS** |

**Resultado del Recorrido A: PASS en los 5 ítems. Sin FAIL.**

### Siguiente paso

Recorrido B — captura con evidencia real. Es la **dependencia raíz** de todo el
Hito 1 (D11 §11.5): sin entrada pública de evidencia, ningún aprendizaje alcanza
`approved`, y los Recorridos C, D y E no tienen sobre qué operar.

---

## Tramo 2 — Recorrido B: captura con evidencia real

> **Sesión reanudada.** Una sesión anterior alcanzó su límite a mitad de la
> implementación y dejó el árbol de trabajo **sin compilar**. Esta sesión no
> reinició el recorrido: partió del estado real, conservó el trabajo confirmado
> y el del árbol de trabajo, y lo condujo a verde.

### Commits

De la sesión interrumpida (documentación y prueba RED, se conservan intactos):

| Hash | Mensaje |
|------|---------|
| `fced055` | `docs: add public evidence entry to the domain, CLI and MCP contract` |
| `c1aafec` | `test: require public evidence entry before approval` (RED intencional) |

De esta sesión (implementación hasta verde):

| Hash | Mensaje |
|------|---------|
| `8866a49` | `feat: real evidence capture path unblocks approval (Recorrido B)` |
| `8199782` | `test: reroute integration evidence through the public capture path` |
| *(este)* | `docs: record Recorrido B closure in the recovery log` |

### Punto de partida real

El diagnóstico del árbol de trabajo confirmó lo previsto: `internal/evidence`
tenía dos archivos nuevos (`service.go`, `collect.go`) y `capture`, `domain`,
`mcpserver` y `storage` estaban modificados, pero `internal/mcpserver/server.go:67`
seguía llamando a la firma antigua de `newCaptureSvc`. Esa discordancia rompía
la compilación. `storage.SaveEvidence` no tenía **ningún** llamador de
producción: ese era el cerrojo raíz.

### Qué se completó

1. **Compila.** Se ajustó `server.go` a la nueva firma
   `newCaptureSvc(db, recordsDir, projectRoot, allowedCommands)` con manejo del
   error devuelto, y se añadió `Config.AllowedCommands`.
2. **`storage.SaveEvidence` ya tiene llamador de producción.** `capture.Service`
   persiste aprendizaje + evidencia + evento de auditoría en una sola
   transacción coherente, tanto en captura embebida como en `AddEvidence`.
3. **Flujo público completo.** Se registró la tool MCP `learning_add_evidence`
   (perfiles agent/admin, escritura) y se cablearon el handler de captura
   (evidencia + `idempotency_key`) y los comandos CLI `evidence add` /
   `evidence list`, además de los flags de captura que faltaban
   (`--destination`, `--evidence-level`, colectores y `--idempotency-key`).
4. **Redacción antes de cualquier sink.** La redacción corre dentro de
   `evidence.Prepare` y `RedactLearning`, **antes** del hash y de cualquier
   escritura; SQLite, blob store, Markdown, auditoría y las respuestas CLI/MCP
   solo ven contenido ya redactado.
5. **Idempotencia (D5).** Misma `idempotency_key` en un reintento devuelve el
   aprendizaje existente sin duplicar evidencia ni crear un segundo aprendizaje.

### Verde falso reencaminado

`internal/integration/learning_flow_test.go` y
`internal/integration/p1_procedure_e2e_test.go` satisfacían el umbral de
aprobación llamando a `storage.SaveEvidence` directamente. Se reescribieron para
adjuntar evidencia vía `capture.Service.AddEvidence` (interfaz pública, sin SQL).
No se eliminó cobertura: se reencaminó.

### Decisiones

Ninguna contradicción plan/docs/código nueva. El único ajuste de contrato lo
dictó la propia prueba: `TestContract_DocsRegistrySkillsTripleMatch` exige
retirar una tool de `pendingTools` en el mismo commit que la registra, así que
`learning_add_evidence` se quitó de esa lista. No requiere nuevo número D.

### Comandos ejecutados (resultado real)

- `go build ./...` — limpio.
- `go vet ./...` — limpio.
- `go test -p 1 -count=1 ./...` — todo **ok** salvo `internal/buildinfo`, que
  falla con `fork/exec ... Access is denied` / `open ... Access is denied`. Es
  la contención de antivirus conocida de esta máquina sobre binarios de test en
  el árbol `go-build`; el paquete no fue tocado por este recorrido.
- `go test -race -p 1 ./internal/capture ./internal/mcpserver ./internal/evidence ./internal/integration ./cmd/royo-learn` — todo **ok**, sin data races.

Las diez pruebas de aceptación (cinco CLI, cinco MCP) pasan en verbose:
`captured → needs_evidence → evidence_attached → approved` conducido solo por
interfaces públicas, redacción antes de todo sink, y no duplicación por
idempotencia.

### Puerta de salida del Recorrido B

| # | Criterio | Estado |
|---|----------|--------|
| 1 | Cadena `captured → needs_evidence → evidence_attached → approved` solo por interfaz pública | **PASS** — `TestCLI_EvidenceUnblocksApproval`, `TestMCP_AddEvidenceUnblocksApproval` |
| 2 | Captura acepta evidencia embebida y persiste en una transacción | **PASS** — `TestCLI_CaptureAcceptsEmbeddedEvidence`, `TestMCP_CaptureAcceptsEmbeddedEvidence` |
| 3 | Secreto redactado antes de todo sink | **PASS** — `TestCLI_SecretIsRedactedBeforeEverySink`, `TestMCP_SecretIsRedactedBeforeTheResponse` |
| 4 | Idempotencia (D5) no duplica | **PASS** — `TestCLI_IdempotencyKeyDoesNotDuplicateEvidence` |
| 5 | Verde falso reencaminado por interfaz pública | **PASS** — integración sin `storage.SaveEvidence` |
| 6 | `go build` / `go vet` limpios | **PASS** |

**Resultado del Recorrido B: PASS. Sin FAIL.** El único fallo de `go test ./...`
es la contención de antivirus en `internal/buildinfo`, ambiental y ajena a este
recorrido.
