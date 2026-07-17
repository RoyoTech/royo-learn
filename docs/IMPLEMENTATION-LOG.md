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

---

## Tramo 2 · Recorrido C — Aprobación pública y verificable (2026-07-14)

Cierre del defecto central del Hito 1 (D11): el **agujero de gobernanza**
(publicar en `AGENTS.md` o alcance compartido **sin** aprobación humana) y el
**bloqueo de aprobación** (la aprobación existía en el dominio pero ninguna
interfaz pública podía otorgarla). Ambos se reparan juntos.

### Commits

| Hash | Mensaje |
|------|---------|
| `0594414` | `test: prove AGENTS.md publishes without approval (governance hole)` (prueba roja) |
| `c5b9d69` | `fix: require human approval for shared and AGENTS.md destinations` |
| `421bfea` | `feat: share one canonical curation-decision allowlist across CLI and MCP` |
| `24dccaf` | `feat: expose publication approval via CLI and MCP and gate publish on it` |
| `474377e` | `refactor: bind preview hash to the policy outcome for approval invalidation` |
| `c6bd3f8` | `docs: restore the learning_approve step in the publish-learning skill` |
| `5bd9ef7` | `test: prove the CLI approval gate end to end` |

### Qué se hizo

1. **Se cerró la tautología (Problema 1, el titular).**
   `policySharedScopeRequiresApproval` y `policyAgentsRuleRequiresApproval`
   (`internal/publish/policy.go`) ya no dependen de la decisión de curación que
   *derivó* el destino —comprobación inalcanzable— sino del **destino efectivo**:
   cualquier destino `shared` o `agents_rule` marca `requires_approval: true`.
   Un aprendizaje **no-`preference`** dirigido a `AGENTS.md` ahora exige
   aprobación. Se corrigieron las dos pruebas que consagraban el agujero
   (`TestEvaluatePolicies_ProcedureType_Shared`,
   `TestEvaluatePolicies_AgentsRuleApproved`).
2. **Se rompió el bloqueo (aprobación pública).** Nueva tool MCP
   `learning_approve` (`{learning_id, preview_hash, approved_by, reason,
   approval_evidence, expires_at}`, perfil agent/admin, escritura) y comando CLI
   `royo-learn approve <id> --preview-hash --approved-by --reason
   --approval-evidence [--expires-at]`. Ambos son el **primer llamador de
   producción** de `publish.Service.Approve`; no se reimplementó el store (regla 1.3).
3. **`learning_publish` exige `approval_id`** cuando el preview indica
   `requires_approval: true`. No basta con "alguna aprobación compatible": el
   `approval_id` debe ser el ligado a **ese** `preview_hash`
   (`internal/publish/publish_op.go`). Se añadió `--approval-id` al CLI y
   `approval_id` al schema MCP.
4. **Lista blanca canónica única (D11 §11.2).** `internal/domain` define
   `ValidCurationDecisions` / `ParseCurationDecision`; el CLI (`parseCurateAction`)
   y el handler MCP (`curate_learning`) validan **contra ella y nada más**. El
   MCP dejó de pasar la decisión en crudo. Prueba de contrato
   `TestContract_CLIAndDomainShareCurationAllowlist`: ambos aceptan y rechazan
   exactamente el mismo conjunto.
5. **Preview hash ligado a la política.** El hash del preview incorpora ahora la
   firma de la evaluación de políticas (`PolicySignature`), además del diff
   combinado que ya reflejaba destinos y contenido previo. Un cambio de política
   produce un hash distinto e invalida la aprobación previa.
6. **Skill restituida (D15).** `skills/publish-learning/SKILL.md` (v4.0.0)
   vuelve a citar `learning_approve` en el paso 4 y a exigir el `approval_id` en
   el paso 5, ahora que la tool existe.

### `learning_publish` movido a `agent` (D2)

**Sí, se movió `learning_publish` del perfil `admin` al perfil `agent`.** La
cláusula vinculante de D2 lo condicionaba a que existieran las políticas por
destino y `learning_approve`; el Recorrido C entrega ambas. La protección deja de
ser el perfil y pasa a ser la aprobación: los destinos sensibles reportan
`requires_approval: true` y `learning_publish` rehúsa sin un `approval_id`
coincidente. Se actualizaron `internal/mcpserver/conformance_test.go` (perfiles
`standard`/`agent`/`admin`) y se retiró `learning_approve` de `pendingTools` en
`internal/mcpserver/contract_test.go`.

### Estado de las seis condiciones de invalidación (D11 §11.1)

| # | Condición | Mecanismo | Prueba |
|---|-----------|-----------|--------|
| 1 | Cambia el preview | La aprobación se busca por `preview_hash`; un preview distinto es otro hash sin aprobación | `TestApprovalGate_ApprovalForDifferentPreviewIsRejected` |
| 2 | Cambia un destino | El destino está en el diff → cambia el `preview_hash` | igual que #1 (dos aprendizajes, hashes distintos) |
| 3 | Cambia el contenido previo de un destino | El contenido previo está en el diff → cambia el `preview_hash` | cubierto por la composición del hash (`preview.go`); regenerar el preview produce otro hash |
| 4 | Expira | `Service.CheckApproval` rechaza `ExpiresAt` pasado | `TestApprovalGate_ExpiredApprovalIsRejected`, `TestCLI_ApprovalGate` |
| 5 | Se revoca | `GetApprovalByHash` filtra revocadas; `CheckApproval` rechaza `RevokedAt` | soportado en dominio/almacenamiento (`RevokeApproval`); **sin interfaz pública de revocación en este recorrido** (fuera del alcance C) |
| 6 | Cambia la política relevante | `PolicySignature` entra en el `preview_hash` | `TestPolicySignature_ChangesWithPolicyOutcome` |

Nota sobre #5: la revocación se **valida** correctamente (una aprobación con
`revoked_at` se rechaza), pero el Recorrido C no añade un comando/tool público de
revocación; el plan no lo pide en C. Queda como capacidad de dominio disponible.

### Pruebas obligatorias (todas por interfaz pública)

| Prueba | Ubicación |
|--------|-----------|
| Agujero: no-`preference` → `AGENTS.md` → publicar sin aprobación **bloqueado** (`requires_approval: true`) | `internal/mcpserver/approval_gate_test.go::TestApprovalGate_NonPreferenceAgentsRuleRequiresApproval` (roja primero) |
| Publicación sensible sin aprobación → bloqueada | `TestApprovalGate_SensitiveWithoutApprovalIsBlocked`, `TestCLI_ApprovalGate` |
| Aprobación de otro `preview_hash` → rechazada | `TestApprovalGate_ApprovalForDifferentPreviewIsRejected` |
| Aprobación expirada → rechazada | `TestApprovalGate_ExpiredApprovalIsRejected` |
| Aprobación válida → aceptada | `TestApprovalGate_ValidApprovalIsAccepted`, `TestCLI_ApprovalGate` |
| Aprobación reutilizada para otro preview → rechazada | `TestApprovalGate_ApprovalForDifferentPreviewIsRejected`, `TestCLI_ApprovalGate` (approval_id ajeno) |
| Lista blanca única CLI↔MCP | `cmd/royo-learn/curate_allowlist_test.go::TestContract_CLIAndDomainShareCurationAllowlist` |
| Toda política tiene una entrada que la hace fallar | `TestEvaluatePolicies_SharedWithoutApproval`, `_AgentsRuleNotApproved`, `_PreferenceType` |

Ninguna prueba fabrica una `Approval` escribiendo en el almacenamiento: cada
aprobación se obtiene por `learning_approve` / `royo-learn approve`.

### Comandos ejecutados (resultado real)

- `go build ./...` — limpio.
- `go vet ./...` — limpio.
- `go test -p 1 -count=1 ./...` — todo **ok** salvo `internal/buildinfo`, que
  falla con `Access is denied` al abrir/ejecutar `buildinfo.test.exe` en el árbol
  `go-build`. Es la contención de antivirus conocida de esta máquina; el paquete
  no fue tocado por este recorrido.
- `go test -race -p 1 ./internal/publish ./internal/mcpserver ./internal/domain ./internal/integration ./cmd/royo-learn` — todo **ok**, sin data races.

### Puerta de salida del Recorrido C

| # | Criterio | Estado |
|---|----------|--------|
| 1 | El agujero está cerrado: `AGENTS.md`/compartido no-`preference` exige aprobación | **PASS** |
| 2 | La aprobación es obtenible por interfaz pública (CLI y MCP) | **PASS** |
| 3 | `learning_publish` exige el `approval_id` ligado al `preview_hash` | **PASS** |
| 4 | Lista blanca de decisiones única CLI↔MCP | **PASS** |
| 5 | Las cinco pruebas del plan + la del agujero, por interfaz pública | **PASS** |
| 6 | Skill restituye `learning_approve` (D15) | **PASS** |
| 7 | `go build` / `go vet` limpios; suite verde salvo `buildinfo` ambiental | **PASS** |

**Resultado del Recorrido C: PASS. Sin FAIL.** El único fallo de
`go test ./...` es la contención de antivirus en `internal/buildinfo`, ambiental
y ajena a este recorrido.

---

## Tramo 2 · Recorrido D — Publicación segura y estados verdaderos (2026-07-14)

Objetivo: el sistema **nunca** informa éxito parcial y **nunca** deja archivos
modificados tras un fallo tardío. `published` se alcanza **solo** después de que
todas las escrituras terminen, las verificaciones pasen, el registro de
publicación persista y la auditoría quede escrita.

### Commits

| Hash | Mensaje | Rol |
|------|---------|-----|
| `d8c71ae` | `test: bind the preview hash to the whole publication plan` | Prueba roja (no compila: falta `domain.PublicationPlanTarget`). |
| `3db951d` | `feat: bind preview hash to per-destination prior and posterior hashes` | Verde: preview persiste el plan completo por destino. |
| `5230363` | `test: require --apply to write and dry-run by default` | Prueba roja (falta el campo `Apply`). |
| `e32bf41` | `feat: dry-run publish by default and require --apply to write` | Verde: D7 en servicio, CLI y MCP. |
| `4e4736e` | `refactor: order the publish flow and add a fault-injection seam` | Reordena el flujo, compensa todo fallo tardío y añade la costura de inyección. Toda la suite previa queda verde. |
| `5b856a2` | `test: prove publication leaves no false published under seven faults` | Las siete pruebas de inyección de fallo. |
| *(este)* | `docs: record Recorrido D closure in the recovery log` | Documentación. |

### Qué se hizo

1. **Preview que liga el plan completo (no solo el diff).** El preview persiste
   ahora, por destino, `root`, `path`, operación, **hash previo** y **hash
   posterior** (`domain.PublicationPlanTarget`, `internal/domain/types.go`). El
   `preview_hash` se calcula sobre `PlanSignature(targets)` +
   `PolicySignature(policies)` (`internal/publish/policy.go`,
   `internal/publish/preview.go`). No se estrecha respecto de Recorrido C: el
   hash del contenido subsume el diff combinado y añade explícitamente la
   operación y la raíz por destino. Se persiste en `plan_json` (columna JSON), sin
   migración de esquema.
2. **Dry-run por defecto (D7).** `PublishInput.Apply` (default `false`). Sin él,
   `Service.Publish` valida el plan y **no escribe nada**, devolviendo un
   resultado marcado `DryRun`. Escribir exige `--apply` (CLI) / `apply: true`
   (MCP); `--apply` y `--dry-run=false` son equivalentes. La puerta de dry-run
   vive en el servicio: es la segunda línea de defensa, independiente de la
   aprobación, y ningún handler puede escribir por accidente.
3. **Flujo de aplicación en el orden exacto del plan**
   (`internal/publish/publish_op.go`):
   `validar aprendizaje → validar preview → validar aprobación → validar hashes
   actuales → adquirir lock → crear backups → registrar intento (journal) →
   escribir → verificar → persistir resultado → marcar published → cerrar
   journal`.
4. **`published` solo tras el éxito total.** `learning.Status = Published` y el
   registro de publicación se persisten en **una sola transacción**. Cualquier
   fallo posterior a la primera escritura ejecuta compensación (rollback),
   registra el resultado en el journal, **no** marca `published` y devuelve un
   error estructurado.
5. **Instrucción de recuperación cuando el rollback falla.** `Service.compensate`
   devuelve un `domain.DomainError` con código `rollback_failed`, los backups
   pendientes y la instrucción (`royo-learn rollback --journal-id … && royo-learn
   doctor`). El journal registra el estado `rollback_failed`. El defecto que se
   corrige: antes, un fallo de journal o de la actualización final de SQLite tras
   escribir dejaba archivos modificados **sin** rollback.

### Las siete pruebas de inyección de fallo

Todas en `internal/publish/fault_injection_test.go`, conducidas por
`Service.Publish` con una **costura real** (`FileWriter` inyectable + `FaultHooks`,
`internal/publish/publish.go`); ninguna manipula el almacenamiento por detrás del
servicio.

| # | Punto de fallo | Prueba | Asalto |
|---|----------------|--------|--------|
| 1 | Escritura del primer archivo | `TestFault_FirstFileWriteFails` | writer falla en la llamada 1 |
| 2 | Escritura del segundo archivo | `TestFault_SecondFileWriteFails` | multi-destino; writer falla en la llamada 2 |
| 3 | Verificación | `TestFault_VerificationFails` | writer corrompe el contenido; el hash on-disk no cuadra |
| 4 | Journal (registro de intento) | `TestFault_JournalAttemptFails` | `FaultHooks.BeforeJournalAttempt` |
| 5 | Actualización final de SQLite | `TestFault_FinalSQLiteUpdateFails` | `FaultHooks.BeforeDBCommit` |
| 6 | El rollback mismo | `TestFault_RollbackItselfFailsEmitsRecoveryInstruction` | corrupción + `FaultHooks.FailRollback` |
| 7 | Destino modificado tras el preview | `TestFault_DestinationModifiedAfterPreviewIsRefused` | edición out-of-band; `target_changed` |

Cada prueba afirma: **cero `published` falso** (el aprendizaje sigue `approved`)
y **cero archivo modificado** — el rollback restaura byte a byte (`#3`, `#5`) o
elimina el archivo nuevo (`#1`, `#2`). En `#6`, con el rollback roto, se afirma
que se emite una instrucción de recuperación con código `rollback_failed`. En
`#7` se rehúsa **antes** de escribir.

### Determinación sobre el outbox (regla dura)

**Journal + compensación + `doctor` bastan. No se introdujo outbox.** Ninguna de
las siete pruebas de corte demostró una ventana irrecuperable que journal +
compensación no cubran:

- El **registro de intento** se escribe en el journal **antes** de cualquier
  escritura, con la publicación, sus destinos y los backups: un corte entre ahí
  y el final es recuperable solo desde el journal.
- Todo fallo posterior a la escritura compensa y registra el desenlace
  (`rolled_back` o `rollback_failed`) en el journal.
- El caso #6 (rollback roto) **no** es una ventana silenciosa: el sistema emite
  una instrucción de recuperación y deja el rastro durable (journal +
  backups) para `royo-learn rollback` y `royo-learn doctor`.

Por tanto **no se paró para revisión humana de outbox**: no había necesidad
demostrada (regla §1.2 del plan y D6).

### Ajustes de firma en pruebas previas (justificados, no debilitados)

`Apply` es un campo obligatorio nuevo. Se añadió `Apply: true` a los llamadores
de `Service.Publish` de las pruebas M1/M2/M3/E2E y de integración
(`internal/publish/publish_test.go`, `internal/publish/skill_area_explicit_test.go`,
`internal/integration/*`), y `--apply` / `apply: true` a las dos pruebas de
Recorrido C que ejercen una publicación **exitosa** (`TestCLI_ApprovalGate`,
`TestApprovalGate_ValidApprovalIsAccepted`) y a `main_test.go`. Las pruebas de
bloqueo de C (aprobación ausente/errónea) **no** se tocaron: la validación de
aprobación ocurre antes de la puerta de dry-run, así que siguen bloqueando. No se
debilitó ninguna aserción; solo se declara el flag que D7 introduce.

### Nota de interpretación — «adquirir lock»

El flujo del plan lista «adquirir lock». El sistema no tiene un lock de sistema
operativo y no se introdujo uno (sería rediseño §1.2 sin necesidad probada). Se
implementó como la comprobación de árbol sucio (`CheckDirtyWorktree`) más la
línea base de bloqueo optimista por hash ya existente (M3). No es una
contradicción de contrato; no requiere número D.

### Comandos ejecutados — resultado real

- `go build ./...` — `exit=0`.
- `go vet ./...` — `exit=0`.
- `go test -p 1 -count=1 ./...` — todo **ok** salvo `internal/buildinfo`
  (`fork/exec … Access is denied`), contención de antivirus conocida de esta
  máquina; el paquete no lo toca este recorrido. Verificado en aislamiento
  compilando a ruta estable: `PASS`.
- `go test -race -p 1 ./...` — **todos los paquetes ok, sin data races**
  (incluido `internal/buildinfo`, que en modo `-race` no sufre la contención).

### Puerta de salida del Recorrido D

| # | Criterio | Estado |
|---|----------|--------|
| 1 | `published` solo tras escritura+verificación+persistencia+auditoría | **PASS** |
| 2 | Un fallo tardío nunca deja `published` falso | **PASS** — 7/7 pruebas |
| 3 | Rollback restaura byte a byte (o elimina el archivo nuevo) | **PASS** — `#1`–`#5` |
| 4 | Instrucción de recuperación si el rollback falla | **PASS** — `#6`, código `rollback_failed` |
| 5 | Destino modificado tras el preview → rehúsa | **PASS** — `#7`, `target_changed` |
| 6 | Dry-run por defecto; escribir exige `--apply` | **PASS** |
| 7 | El preview hash liga el plan completo | **PASS** |
| 8 | Journal + compensación suficientes; sin outbox | **PASS** |
| 9 | `go build` / `go vet` limpios; `-race` verde | **PASS** |

**Resultado del Recorrido D: PASS. Sin FAIL.** El único fallo de
`go test ./...` es la contención de antivirus en `internal/buildinfo`, ambiental
y ajena a este recorrido; en `-race` no aparece.

### Siguiente paso

Recorrido E — reemplazar el E2E permisivo (`cmd/royo-learn/e2e.go`) por los
escenarios CLI (19 pasos) y MCP reales. **No tocado en este recorrido.**

---

## 2026-07-14 — Recorrido E: E2E que demuestra el producto (+ expansión Hito 1)

Sesión reanudada dos veces tras corte por límite de sesión del ejecutor. El
trabajo se retomó desde los commits reales, sin descartar ni rehacer nada.

### Decisión de alcance previa (Bloque 0)

El escenario literal del Recorrido E exige seis operaciones públicas (`get`,
`search`, `occurrence` por CLI; `learning_report_occurrence`, `learning_status`,
`learning_rollback` por MCP) que los Recorridos A–D no construyeron, y sobre las
que el contrato se contradecía (D1/D12 las ponían en Hito 1; D8/§4 las diferían
a Hito 2). Se resolvió con **D17**: se adelantan al Hito 1, reusando los motores
existentes. El humano aprobó explícitamente esta expansión antes de ejecutarla.

### Commits creados

- `2b15f55` docs: pull six public operations into Hito 1 (D17)
- `073d78c` test: require public CLI get, search and occurrence
- `86da1ba` feat: add public CLI get, search and occurrence (D17)
- `8796596` test: require MCP report_occurrence, status and rollback
- `29cc171` feat: add MCP learning_report_occurrence, learning_status and learning_rollback (D17)
- `6a96171` test: replace permissive e2e with strict CLI and MCP scenarios

### E2E nuevo (37 pasos, sin soft-passes)

Tres escenarios en `cmd/royo-learn/e2e.go`:

1. **CLI sensible** (19 pasos): repo Git temporal → init → doctor → captura con
   evidencia → get → search → curate → preview → publish sin aprobación
   (rechazado) → approve → publish `--apply` → verificar archivo escrito →
   verificar estado published → report occurrence → métricas → rollback →
   restauración byte a byte → occurrence listada → doctor final.
2. **CLI bajo impacto** (6 pasos): publicación a scope de proyecto que NO exige
   aprobación; prueba que la política no sobre-bloquea.
3. **MCP sensible** (12 pasos): cliente real por stdio ejecutando el ciclo
   completo hasta `learning_rollback`, verificando schemas, nombres canónicos,
   códigos de error y cambios de estado.

Cada paso asevera un efecto de negocio. El cuerpo permisivo anterior (9 pasos,
soft-passes en `:151/:154/:157`) fue eliminado por completo; la única aparición
de «acceptable» es un comentario que describe lo que se reemplazó.

### Dos bugs reales encontrados al reanudar (arreglados)

El ejecutor reportó «37 pasos verdes» corriendo solo el binario suelto, pero
murió antes de correr `go test`. La verificación posterior encontró dos defectos
que el ejecutor no llegó a ver:

1. **`os.Executable()` bajo `go test`**: el escenario MCP lanzaba `mcp-serve`
   sobre `os.Executable()`, que bajo `go test` es el binario de test, no
   `royo-learn` → `mcp-sensitive/connect` fallaba y el test Go daba 26/1-fallo
   mientras el binario suelto daba 37/0. Corregido: el spawn honra
   `ROYO_LEARN_E2E_BIN`; el test compila un binario real y lo apunta ahí. El
   path del binario suelto queda intacto (`os.Executable()`).
2. **Código muerto en `TestRunE2ETempCompletesAllSteps`**: `t.Fatalf` precedía
   al bucle que listaba los pasos fallidos, que por `runtime.Goexit` nunca
   corría. Reordenado: se registran las fallas antes del `Fatalf`.

### Eliminación de los falsos positivos FP-1…FP-9 (§0.4 del gap report)

| FP | Comportamiento permisivo original | Cómo se caza ahora |
|----|-----------------------------------|--------------------|
| FP-1 | `curate`: fallo obligatorio aceptado (`e2e.go:150-158`) | Paso `curate` (CLI y MCP) asevera el efecto real de curación; sin soft-pass |
| FP-2 | `preview`: ausencia de efecto aceptada | Paso `preview` asevera preview real con hash; `preview-not-over-blocked` en bajo impacto |
| FP-3 | `recurrences`: solo valida que el JSON sea JSON | Pasos `report-occurrence` + `check-metrics` + `verify-occurrence-listed` aseveran registro y conteo |
| FP-4 | seguridad path-traversal: no ejecuta el ataque | Tests dedicados que SÍ ejecutan el ataque: `curate_test.go:846` (`../escape`), `evidence/blob_test.go:197`, `evidence/path_test.go:18-20`, `config_test.go:85` |
| FP-5 | redacción de secretos: no se comprueba | Tests dedicados: `evidence/redact_test.go`, `mcpserver/evidence_test.go:196`, más el CLI del Recorrido B — recorren cada sink |
| FP-6 | el estado nunca se verifica | Pasos `verify-status-published` (CLI) y `status` (MCP) |
| FP-7 | operaciones críticas nunca ejercitadas | publish/approve/rollback/occurrence ejercitadas en ambos escenarios |
| FP-8 | pasos arrastran estado vacío sin fallar | Cada paso asevera efecto; sin cascada de soft-passes |
| FP-9 | `capture-idempotent` no prueba idempotencia | Test dedicado `mcpserver/occurrence_status_rollback_test.go:48` (D5: reintento técnico, sin segundo registro) |

Seis FP se eliminan con aserciones directas del E2E; FP-4, FP-5 y FP-9 con
tests dedicados que ejecutan el ataque/escenario de verdad — más fuertes que un
paso de E2E. No se afirma que el E2E los cubra todos: se documenta dónde cae
cada uno.

### Prueba de mutación (el E2E muerde)

Se rompió el gate de aprobación (`internal/publish/publish_op.go`, forzando
`if false && preview.RequiresApproval`). El E2E se puso rojo tanto por binario
como por `go test`: cayeron exactamente `cli-sensitive/publish-without-approval-refused`
y `mcp-sensitive/publish-without-approval-refused`, mientras el paso de bajo
impacto `publish-apply-without-approval` siguió verde (proyecto no exige
aprobación). El E2E distingue las dos políticas. Revertido y confirmado.

### Comandos ejecutados — resultado real

- `go build ./...` limpio; `go vet ./...` limpio.
- `go test -p 1 -count=1 -run TestRunE2ETempCompletesAllSteps ./cmd/royo-learn/`
  → **ok** (37 pasos).
- `go test -p 1 -count=1 ./...` → exit 1, pero las TRES fallas son de la clase
  teardown/AV de Windows (`TempDir RemoveAll: directory not empty` en
  `TestCLI_CaptureAcceptsEmbeddedEvidence` y `TestUpdateFullFlowZipWindows` —
  este último en `internal/selfupdate`, paquete no tocado — y `Access is denied`
  en `internal/buildinfo`). Las tres pasan aisladas. Ninguna es una aserción.

### Puerta de salida del Recorrido E

- [x] E2E CLI de 19 pasos, sin soft-passes, con efectos de negocio → **PASS**
- [x] E2E MCP real por stdio → **PASS**
- [x] Dos políticas separadas (bajo impacto / sensible) → **PASS**
- [x] Cada FP-1…FP-9 del Tramo 0 eliminado y mapeado → **PASS**
- [x] Prueba de mutación demuestra que el E2E falla ante ruptura real → **PASS**
- [x] Cuerpo permisivo anterior eliminado por completo → **PASS**

**Resultado del Recorrido E: PASS. Sin FAIL.** Las tres fallas de la suite
completa son ambientales de Windows (teardown/AV), pasan aisladas, y quedan
registradas para endurecer en el Tramo 5 junto con las de los Recorridos B y D.

### Siguiente paso

Recorrido F — actualización segura de Skills instaladas
(`setup status` / `upgrade-skills --dry-run` / `--apply`). **No tocado en este
recorrido.**

---

## 2026-07-14 — Recorrido F: actualización segura de Skills instaladas

**Rama:** `fix/v019-contract-recovery`
**Commit de partida:** `555d8ca` (cierre del Recorrido E)

### Problema reparado (BASELINE-GAP Hallazgo 11)

`internal/setup/skill.go` omitía cualquier Skill ya existente («Existing skills
are skipped, never overwritten», `skill.go:20-21,49-54`). Corregir las Skills del
repositorio (Recorrido A) NO reparaba las copias ya instaladas en las máquinas de
usuarios de ≤v0.1.9: seguían citando tools inexistentes. Actualizar el binario no
ofrecía ninguna ruta para actualizarlas.

### Commits

| Hash | Mensaje | Rol |
|------|---------|-----|
| `75c3801` | `test: require safe upgrade of installed skills` | **Prueba roja.** Los 7 tests fallan por falta de manifiesto y del subcomando `upgrade-skills`; el paquete compila. |
| `d06b934` | `feat: safely upgrade managed skills` | Implementación mínima que los pone en verde. |
| *(este)* | `docs: record Recorrido F closure in the recovery log` | Documentación. |

### Diseño

- **Manifiesto por Skill instalada** (`internal/setup/skillmanifest.go`):
  `{name, version, source_sha256, installed_sha256, managed_by}`. Se persiste en
  el índice oculto `<skills-dir>/.royo-learn/manifests/<name>.json`, junto a las
  Skills pero fuera de cualquier directorio de Skill (no tiene `SKILL.md`, así que
  ni los agentes ni `InstallSkills` lo tratan como Skill). `InstallSkills` ahora
  escribe el manifiesto en cada instalación nueva; los hashes se calculan con
  `HashSkillDir` (SHA-256 determinista sobre los mismos archivos que copia el
  instalador, rutas normalizadas a `/`). La versión sale del frontmatter de
  `SKILL.md`.
- **Comandos** (`cmd/royo-learn/setup.go`): `setup upgrade-skills --dry-run`
  (por defecto) y `--apply`. Cableados en el dispatcher `runSetup`. `setup status`
  ya existía. Reutiliza `resolveSkillsSource` y `copyDir`/`writeFileAtomic`
  existentes; no reimplementa hashing ni copia de archivos.
- **Política exacta** (`internal/setup/skillupgrade.go`, `UpgradeSkills`):
  - hash instalado == `installed_sha256` del manifiesto (intacto) → respaldar,
    actualizar, registrar nueva versión (`upgrade`).
  - hash instalado != manifiesto (modificado por el usuario) → NO sobrescribir;
    crear versión candidata en `.royo-learn/candidates/<name>`, mostrar diff,
    registrar conflicto en `.royo-learn/conflicts/<name>.json` (`conflict`).
  - sin manifiesto de royo-learn / `managed_by` distinto → no tocarla
    (`unmanaged`).
  - hash instalado == hash de origen → `up_to_date` (idempotente).
- **Aplicación atómica y recuperable** (`performUpgrade`): se prepara la versión
  nueva en `staging` aparte → se respalda la copia actual ANTES de sobrescribir →
  swap (borrar original, materializar staging). Si el swap falla tras borrar el
  original, se restaura el backup. Dry-run por defecto (disciplina de D7): sólo
  `--apply` escribe; un `--dry-run` explícito gana sobre `--apply`.

### Los siete tests obligatorios

Todos en `cmd/royo-learn/setup_upgrade_test.go`, conducidos SÓLO por la interfaz
pública `royo-learn setup ...` (home aislado vía `HOME`/`USERPROFILE`).

| Test | Qué prueba |
|------|------------|
| `TestUpgradeSkills_FreshInstallWritesManifest` | instalación nueva → instala y escribe manifiesto (`source==installed`, `managed_by=royo-learn`). |
| `TestUpgradeSkills_CleanUpgrade` | upgrade sin modificaciones → respalda y actualiza; manifiesto pasa a v2. |
| `TestUpgradeSkills_UserModifiedNotOverwritten` | upgrade con personalización → `conflict`; original **byte a byte** intacto; candidato con la versión nueva; diff presente. |
| `TestUpgradeSkills_BackupIsRestorable` | el backup se crea antes de sobrescribir y su `SKILL.md` == original (restaurable). |
| `TestUpgradeSkills_DryRunWritesNothing` | `--dry-run` (defecto) no toca Skill ni manifiesto ni crea backups; reporta `upgrade` (lo que HARÍA). |
| `TestUpgradeSkills_IdempotentRepeat` | segunda corrida `--apply` → `up_to_date`, no-op. |
| `TestUpgradeSkills_RecoveryAfterFailure` | fallo a mitad de upgrade (obstáculo en `staging`) → error, original byte a byte intacto, manifiesto en v1; tras quitar el obstáculo, la re-corrida recupera y actualiza. |

Prueba roja verificada antes de implementar: los 7 fallan por «unknown setup
subcommand "upgrade-skills"» y por manifiesto ausente.

### Comandos ejecutados — resultado real

- `go build ./...` limpio; `go vet ./...` limpio.
- `go test -p 1 -count=1 ./...` → exit 1; la ÚNICA falla es
  `internal/buildinfo` (`fork/exec ... buildinfo.test.exe: Access is denied`),
  ruido ambiental de AV/Windows en un paquete no tocado; falla igual en
  aislamiento. `cmd/royo-learn` e `internal/setup` en verde. Las tres flakes de
  teardown de Windows (Recorridos B/D/E) pasaron en esta corrida.
- `go test -race -p 1 -count=1 ./internal/setup/` → **ok**;
  `go test -race -p 1 -count=1 -run 'TestUpgradeSkills|TestSetup' ./cmd/royo-learn/`
  → **ok**.
- Humo con binario real: instalar v1 (manifiesto escrito) → dry-run (nada
  escrito) → `--apply` (backup creado, v2 en disco) → repetición (`up_to_date`);
  y sobre Skill personalizada: `--apply` → `conflict`, hash del `SKILL.md`
  idéntico antes y después (preservado byte a byte), candidato con v2, conflicto
  registrado.

### Puerta de salida del Recorrido F

- [x] Manifiesto por Skill instalada, escrito en instalación nueva → **PASS**
- [x] `setup upgrade-skills --dry-run` (defecto) y `--apply` cableados → **PASS**
- [x] Skill intacta se actualiza con backup; modificada NO se sobrescribe → **PASS**
- [x] Skill sin manifiesto de royo-learn no se toca → **PASS**
- [x] Backup previo a la sobrescritura, restaurable → **PASS**
- [x] Idempotente; recuperación tras fallo sin Skill a medio escribir → **PASS**
- [x] Los 7 tests obligatorios, por la interfaz pública, en verde → **PASS**

**Resultado del Recorrido F: PASS. Sin FAIL.** La única falla de la suite
completa es la ambiental de `internal/buildinfo` (AV/Windows), ajena a este
recorrido. Criterio de aceptación del plan cumplido: actualizar el binario ofrece
una ruta segura para actualizar las Skills incompatibles ya instaladas.

### Siguiente paso

Tramo 3 — puerta de publicación del Hito 1 (`v0.1.10`). **Fuera del alcance de
este recorrido; no tocado.**

---

## 2026-07-14 — Tramo 3: puerta de publicación del Hito 1 (auditoría y preparación)

**Rama:** `fix/v019-contract-recovery`
**Commit de partida:** `9906c20` (cierre del Recorrido F)
**Naturaleza del tramo:** auditar, verificar PASS/FAIL, alinear el README y
**preparar** (no publicar) la versión `v0.1.10`. No se escribió funcionalidad nueva.

### Regla dura respetada

No se ejecutó `git tag`, `git push`, `goreleaser release` ni ninguna operación de
publicación. El tramo termina dejando el comando exacto listo y **detenido** para
aprobación humana. El único cambio de código/docs de este tramo es la alineación
del README y esta entrada de bitácora, ambos en commits locales sin `push`.

### Paso 1 — Auditoría de los 12 ítems de la puerta (PASS/FAIL, con prueba)

Cada ítem se verificó ejecutando la(s) prueba(s) que lo demuestran. Sin PARCIAL.

| # | Ítem | Prueba que lo demuestra | Estado |
|---|------|-------------------------|--------|
| 1 | Skills y MCP coinciden (test de contrato) | `TestContract_SkillsCiteOnlyRegisteredCanonicalTools`, `TestContract_DocsRegistrySkillsTripleMatch` (`internal/mcpserver/contract_test.go`) — paquete `ok` | **PASS** |
| 2 | Captura acepta evidencia | `TestCLI_CaptureAcceptsEmbeddedEvidence`, `TestMCP_CaptureAcceptsEmbeddedEvidence` (`internal/mcpserver/evidence_test.go`, `cmd/royo-learn`) | **PASS** |
| 3 | `evidence add` funciona (CLI y MCP) | `TestCLI_EvidenceUnblocksApproval`, `TestMCP_AddEvidenceUnblocksApproval`; comando real `royo-learn evidence add <id> --summary …` | **PASS** |
| 4 | La curación aprueba por interfaz pública | `TestContract_CLIAndDomainShareCurationAllowlist` (`cmd/royo-learn/curate_allowlist_test.go`); cadena `captured→needs_evidence→approved` sin SQL directo | **PASS** |
| 5 | La aprobación pública queda ligada al `preview_hash` | `TestApprovalGate_ValidApprovalIsAccepted`, `TestApprovalGate_ApprovalForDifferentPreviewIsRejected`, `TestApprovalGate_ExpiredApprovalIsRejected`, `TestCLI_ApprovalGate` | **PASS** |
| 6 | `publish` exige `approval_id` cuando `requires_approval` | `TestApprovalGate_SensitiveWithoutApprovalIsBlocked`, `TestApprovalGate_NonPreferenceAgentsRuleRequiresApproval` | **PASS** |
| 7 | `publish` exige `--apply` para escribir | `TestFault_*` (dry-run por defecto); help real: `publish -apply` (D7), `-dry-run` default `true` | **PASS** |
| 8 | `rollback` compensa fallos posteriores a escritura | Las 7 pruebas `TestFault_*` (`internal/publish/fault_injection_test.go`) | **PASS** |
| 9 | E2E CLI completo (19 pasos, sin soft-passes) | `TestRunE2ETempCompletesAllSteps` (escenario `cli-sensitive`, 19 pasos, `cmd/royo-learn/e2e.go`) — `ok` | **PASS** |
| 10 | E2E MCP completo | `TestRunE2ETempCompletesAllSteps` (escenario `mcp-sensitive` por stdio) — `ok` | **PASS** |
| 11 | Skills instaladas pueden actualizarse (Recorrido F) | Los 7 `TestUpgradeSkills_*` (`cmd/royo-learn/setup_upgrade_test.go`) | **PASS** |
| 12 | El README describe únicamente lo demostrado | Alineado en este tramo (ver Paso 4); commit `docs: align README with demonstrated Hito 1 behavior` | **PASS** |

**Resultado Paso 1: 12/12 PASS. Sin FAIL.**

### Paso 2 — Batería de verificación final (salida real, clasificada)

| Comando | Resultado real | Clasificación |
|---------|----------------|---------------|
| `go fmt ./...` / `gofmt -l ./cmd ./internal` | Sin archivos listados (limpio) | PASS |
| `go mod tidy` | **Cero cambios** en `go.mod`/`go.sum` (`git diff --stat` vacío) | PASS — sin hallazgo |
| `go mod verify` | `all modules verified` | PASS |
| `go vet ./...` | Limpio, `exit=0` | PASS |
| `go build ./cmd/royo-learn` | `exit=0` | PASS |
| `go test -race -p 1 -count=1 ./...` | **`exit=0`, TODOS los paquetes `ok`** (incluido `internal/buildinfo`) | PASS — señal fiable de suite completa |
| `go test -p 1 -count=1 ./...` (serie) | `exit=1`; **única** falla `internal/buildinfo` | Ambiental (AV) |
| `go test ./...` (paralelo) | `exit=1`; **única** falla `internal/buildinfo` | Ambiental (AV) |

**Clasificación de cada falla reportada por la suite:**

- `internal/buildinfo` (`fork/exec … buildinfo.test.exe: Access is denied`):
  **AMBIENTAL**. Reproducido: compilado a ruta estable
  (`go test -c -o .gotmp/buildinfo.test.exe ./internal/buildinfo/`) y ejecutado →
  `PASS` en las 3 pruebas (`TestVersionJSON`, `TestHumanString`,
  `TestDevelopmentMetadataDefaults`). Además pasa bajo `-race`. Antivirus de
  Windows bloqueando binarios de test en el árbol `go-build`. **No es fallo de
  código.**
- Las tres flakes de teardown de Windows conocidas
  (`TestCLI_CaptureAcceptsEmbeddedEvidence`, `TestUpdateFullFlowZipWindows`, y el
  e2e con dir temporal bloqueado por AV): **no se manifestaron** en las corridas
  de este tramo (serie ni paralelo). Quedan clasificadas de antemano como
  ambientales (pasan en aislamiento), pero no hubo que reejecutarlas porque no
  aparecieron.

**Ninguna falla de aserción. Cero FAIL de código.**

### Paso 3 — Cacería de soft-passes (grep en la superficie de pruebas)

Patrón buscado: `acceptable|soft.?pass|failure is acceptable|doesn't crash|any exit code|expected.*failure|skip.*expected`.

| Hit | Veredicto |
|-----|-----------|
| `cmd/royo-learn/setup_test.go:235,251,328` (`expected failure…`, `expected exitFailure`) | **Legítimo** — aserciones de ruta negativa: afirman que un error/`exitFailure` SÍ se devuelve. |
| `internal/setup/codex_test.go:71` (`is acceptable because Codex would never accept a commented section`) | **Legítimo** — el test asevera `if !ok { t.Errorf }`; el comentario documenta un quirk de coincidencia literal en un helper no crítico. Tiene aserción real. |
| `internal/mcpserver/contract_test.go:165` (`no pending list, and no soft pass`) | **Legítimo** — comentario que describe la intención de la prueba de contrato. |
| `internal/publish/fault_injection_test.go:76,100,157,189,221` (`expected a … failure`) | **Legítimo** — aserciones fuertes de inyección de fallo: exigen que el fallo produzca error. |
| `internal/publish/publish_test.go:1798` (`expected journal failure error, got nil`) | **Legítimo** — aserción de ruta negativa. |
| `cmd/royo-learn/e2e.go:20-25` (`failure is acceptable`, `soft pass`) | **Legítimo** — comentario que describe el E2E permisivo que SE REEMPLAZÓ; no es código activo. |

**Ningún soft-pass nuevo en ninguna prueba crítica.** Los ítems 9 y 10 no están
comprometidos.

### Paso 4 — README alineado con lo demostrado (ítem 12)

Commit `docs: align README with demonstrated Hito 1 behavior`. Correcciones:

1. **Perfiles MCP.** El README afirmaba `minimal` = «capture, search, doctor» y
   `standard` = «curate, preview, list, get». Es falso desde el Recorrido A:
   `capture` es escritura y ya **no** vive en `read`/`minimal`, y las lecturas
   incluyen `get`/`list`/`status`. Se reemplazó por los perfiles canónicos
   `read`/`agent`/`admin` (flag `--tools`), con el conjunto real por perfil y la
   nota de que `--profile`/`minimal|standard|full` y los alias de tools siguen
   funcionando como deprecated (D8). Ejemplo `--profile full` → `--tools admin`.
2. **Flujo de publicación del Quick Start.** El ejemplo de `publish` mostraba solo
   `--preview-hash` (sin `--apply` ni `--approval-id`), lo que contradice lo
   demostrado: sin `--apply` es dry-run (D7) y un destino sensible se bloquea sin
   `--approval-id` (Recorrido C). Se añadió el paso `approve` y se corrigió
   `publish` con `--apply` + `--approval-id`. Se añadieron `get`, `search`,
   `evidence add` y `occurrence` (las seis operaciones D17).
3. **Actualización segura de Skills (Recorrido F).** Se documentó
   `royo-learn setup upgrade-skills` (dry-run por defecto, `--apply` para escribir;
   Skills modificadas por el usuario nunca se sobrescriben).

No se afirma ninguna función de Hito 2: no hay `export`/`import`/`rebuild-index`/
`review` ni embeddings en el README. La lista de garantías del binario ya era
exacta.

**Nota (trabajo de Tramo 6, no de este tramo):** los README traducidos
(`docs/README.es.md` y los stubs `de/fr/hi/pt/zh`) ahora quedan **desactualizados**
respecto del README principal (perfiles, flujo de publicación, upgrade de Skills).
No se tocaron por indicación explícita; se registran como pendientes del Tramo 6.

### Paso 5 — Comando de release preparado (NO ejecutado)

Versión del Hito 1: **`v0.1.10`** (D8). Proceso: `.goreleaser.yml`
(`goreleaser release --clean`), owner `RoyoTech/royo-learn`, binario único
multiplataforma, `CGO_ENABLED=0`.

**Checklist previo al tag (todo debe pasar):**

```bash
git status                       # árbol limpio (commitear docs primero)
go test -race ./...              # verde en la máquina de referencia (incluye buildinfo)
go vet ./...
go build ./cmd/royo-learn
goreleaser release --snapshot --clean   # ensayo local, NO publica
```

**Comando exacto de corte y publicación — DEJADO LISTO, NO EJECUTADO. La
publicación es decisión humana:**

```bash
# 1. Llevar la rama a main (decisión humana sobre la estrategia de merge)
git checkout main
git merge --no-ff fix/v019-contract-recovery

# 2. Etiquetar la versión del Hito 1
git tag -a v0.1.10 -m "royo-learn v0.1.10 — Hito 1: estabilización funcional"

# 3. Empujar commit y tag
git push origin main
git push origin v0.1.10

# 4. Cortar el release (GoReleaser lee el tag)
goreleaser release --clean
```

Requisitos: `GITHUB_TOKEN` con permiso sobre `RoyoTech/royo-learn` y `goreleaser`
instalado. `v0.1.10` conserva todos los nombres de `v0.1.9` como aliases (D8): no
rompe compatibilidad.

### Veredicto del Tramo 3

**READY-FOR-RELEASE.** Los 12 ítems de la puerta pasan; la batería completa está
verde salvo el ruido ambiental de AV (`internal/buildinfo`), reproducido como
no-código; no existe ningún soft-pass nuevo; el README describe exactamente el
comportamiento demostrado. El comando de release queda listo y **detenido** para
aprobación humana. No se publicó nada.

---

## 2026-07-16 — Cierre de seguridad del candidato v0.1.10

La revisión posterior a `66a90da` encontró que el veredicto anterior era
prematuro: publicación y rollback aún tenían ventanas de pérdida de identidad,
sobrescritura concurrente, recuperación no convergente y divergencia entre
SQLite y el registro Markdown. Esta entrada **supersede el veredicto operativo
READY-FOR-RELEASE anterior** sin reescribir su evidencia histórica.

### Rama y alcance

- Rama: `fix/v0110-release-safety`.
- Base exacta: `66a90daaac98ed6e64bbe1235dbe425cde7f18c3`.
- Alcance: seguridad de publicación/rollback, verdad derivada, salida de
  recuperación CLI/MCP, CI y documentación de candidato.
- Fuera de alcance: Hito 2, outbox, proveedor LLM, embeddings, push, tag, PR y
  GoReleaser.

### Unidades TDD

| Unidad | RED | GREEN |
|--------|-----|-------|
| Backups, rutas y CAS | `7d3cb3f` | `c4055d6` |
| Intento alcanzable y rollback convergente | `6e9d45e` | `99c6397` |
| Verdad SQLite/Markdown tras publish y rollback | `0db17d3` | `58ba1bb` |
| Errores de recuperación CLI/MCP | `40320ab` | `d0ba329` |
| Matriz CI y portabilidad | — | `29f784d` |

### Resultado técnico

1. La publicación `in_progress` y sus metadatos completos existen antes de la
   primera escritura. El mismo `publication_id` identifica SQLite y journal.
2. Los backups se generan desde un snapshot único, quedan confinados y
   sincronizados, preservan modo y evitan colisiones de basename.
3. Las escrituras usan CAS hasta la frontera final. Un cambio externo se
   conserva y se informa; nunca se reemplaza por una restauración automática.
4. Rollback persiste progreso por destino, reconoce restauraciones ya hechas y
   converge después de fallos de SQLite. Metadatos heredados no verificables
   fallan cerrados y producen un artefacto.
5. `internal/record` materializa la verdad confirmada. Publish no importa
   capture; rollback exitoso confirma `rolled_back` + `approved` en una misma
   transacción.
6. CLI y MCP proyectan el mismo error de dominio. Los conflictos conservan el
   patch de reversión, detalles y siguiente acción; no se degradan a texto.
7. D19 fija explícitamente las claves JSON heredadas `PolicyName`, `Passed` y
   `Reason` para no romper consumidores de `v0.1.x`.

### Evidencia ejecutada

```text
go test -race -count=1 ./internal/record ./internal/capture ./internal/curate ./internal/publish ./internal/storage
  → PASS

go test -race -count=1 ./internal/domain ./internal/mcpserver ./cmd/royo-learn
  → PASS

go test -count=1 ./internal/integration ./internal/publish ./internal/storage
  → PASS

go vet ./...
  → PASS

go test -race -count=1 ./...
  → PASS completo, sin cache; todos los paquetes ok

GOOS={linux,darwin,windows} GOARCH={amd64,arm64} CGO_ENABLED=0 go build ./cmd/royo-learn
  → PASS en las seis combinaciones

binario instalado en GOBIN temporal: init + doctor --json + e2e --temp
  → PASS; E2E 37/37
```

Una corrida anterior de `go test ./...` no llegó a ejecutar
`internal/buildinfo.test.exe`: Windows devolvió `Access is denied`, incluso al
reintentar ese paquete aislado. La corrida final completa, sin cache y con race
sí pasó. El incidente se conserva como flake ambiental intermitente. La CI ahora
exige `-race` en Linux y prueba Windows/macOS sin race sobre Go mínimo y estable;
esa matriz está configurada pero **NOT_RUN** hasta que un humano autorice push.

### Veredicto

**CANDIDATO PREPARADO, RELEASE BLOQUEADO POR CI NO EJECUTADA.** No se creó tag,
no se hizo push, no se abrió PR y no se ejecutó GoReleaser. El informe final
vigente es `docs/FINAL-IMPLEMENTATION-REPORT.md`.

---

## 2026-07-16 — Corrección de la revisión congelada `v0110-release-safety-v1`

La revisión congelada evaluó `9767e25198afc4f789c50d8a1a00b4c4e4ea2da2`
(`git-tree:c3997b860af2243d772023f03c5f27b93c97f8e3`). Sus recibos no se
modificaron. Esta corrección parte de ese commit y atiende los 33 hallazgos
BLOCKER/CRITICAL; los WARNING/SUGGESTION permanecen fuera de esta puerta salvo
cuando una corrección severa los hizo desaparecer incidentalmente.

### Unidades TDD y operativas

| Unidad | RED / contrato | GREEN / cierre |
|--------|----------------|----------------|
| Preview y aprobación ligados al plan persistido | `e82d38e` | `167ac8f` |
| Rutas, CAS, modos y materialización confinada | `7b5ae08` | `4a7f15b` |
| Recuperación, lock, reconciliación y auditoría | `4bc4e83`, `8aeadca` | `2698394`, `1dc250f` |
| Compatibilidad de errores CLI/MCP/self-update | `78b6916` | `8343d6b` |
| Release, instaladores y cobertura obligatoria | `4472e69` | `66dcec5`, `0823b39`, `e9a0ddf`, `24126c3`, `e6d846f` |

### Trazabilidad de los 33 hallazgos severos

| ID | Cierre | Commit(s) | Evidencia ejecutable principal |
|----|--------|-----------|-------------------------------|
| `READABILITY-001` | El artefacto exige y verifica backup; no inventa bytes de borrado | `2698394` | `TestMissingBackupDoesNotCreateDeletionArtifact`, `TestRecoveryArtifactWriteFailureIsSurfaced` |
| `RELIABILITY-001` | Preview ligado al learning y a todos los destinos | `167ac8f` | `plan_enforcement_test.go`, `plan_hash_test.go` |
| `RELIABILITY-002` | Fallback CLI de rollback usa `rollback_failed` | `8343d6b` | `cmd/royo-learn/errors_test.go` |
| `RELIABILITY-003` | Reemplazo existente usa rename atómico sin destino ausente | `4a7f15b` | `filesystem_safety_test.go`, `filesystem_hardening_test.go` |
| `RELIABILITY-004` | Lock de proyecto compartido por publish/rollback | `2698394` | `TestConcurrentPublishIsRejectedByProjectLock`, `TestPublicationLockReportsExistingOwner` |
| `RELIABILITY-005` | Identidad CAS y rollback incluyen modo | `4a7f15b` | `TestRestoreCASIncludesPublishedAndOriginalModes` |
| `RELIABILITY-006` | Backup ausente/corrupto falla cerrado sin patch engañoso | `2698394` | `TestMissingBackupDoesNotCreateDeletionArtifact` |
| `RELIABILITY-007` | Artefactos se regeneran y sus fallos se propagan | `2698394` | `TestRecoveryArtifactIsRegeneratedFromCurrentState`, `TestRecoveryArtifactWriteFailureIsSurfaced` |
| `RELIABILITY-008` | Rollback exitoso audita actor en la transacción de estado | `2698394` | `TestSuccessfulRollbackAuditsActorInStateTransaction` |
| `RELIABILITY-009` | Segundo rollback limpio devuelve conflicto canónico | `2698394` | `TestRollbackRetryReconcilesThenSecondRollbackConflicts` |
| `RELIABILITY-010` | MCP conserva campos top-level heredados y detalles anidados | `8343d6b` | `TestMCPErrorModelRetainsLegacyTopLevelFields` |
| `RELIABILITY-011` | Todo error de self-update sale con código 1 | `8343d6b` | pruebas de `internal/selfupdate` y `cmd/royo-learn` |
| `RELIABILITY-012` | CI impone domain/storage/publish `80/80/90` | `66dcec5`, `0823b39`, `e9a0ddf` | comando exacto CI: `95.5%`, `81.9%`, `90.1%` |
| `RELIABILITY-013` | Ancestros se validan antes de crear directorios | `4a7f15b` | `TestWriterDoesNotCreateThroughSymlinkedAncestor`, `TestRecordMaterializerDoesNotCreateThroughSymlinkedAncestor` |
| `RESILIENCE-001` | Publicaciones concurrentes no pueden compensarse entre sí | `2698394` | pruebas de lock y concurrencia |
| `RESILIENCE-002` | Reemplazo atómico elimina ventana de cuarentena/hard-link | `4a7f15b` | `TestWriterCASPreservesFinalBoundaryReplacement` |
| `RESILIENCE-003` | Apply usa bytes y conjunto exactos persistidos en preview | `167ac8f` | `TestPublishAppliesPreviewedBytesAfterLearningDrift` |
| `RESILIENCE-004` | Estado post-commit tiene reconciliación ejecutable | `2698394` | `reconciliation_contract_test.go`, `materialization_safety_test.go` |
| `RESILIENCE-005` | Journal es legible y CLI lista intentos recuperables | `2698394`, `1dc250f` | `recovery_safety_test.go`, pruebas `rollback --list --json` |
| `RESILIENCE-006` | Patch de conflicto usa estado actual y backup verificado | `2698394` | pruebas de artefacto faltante, regenerado y fallido |
| `RESILIENCE-007` | Errores conservan causa de journal y auditoría con actor | `2698394` | `TestSuccessfulRollbackAuditsActorInStateTransaction`, `TestRecoveryArtifactWriteFailureIsSurfaced` |
| `RESILIENCE-008` | Release exige CI exitoso del SHA etiquetado | `66dcec5` | `TestReleaseWorkflowRequiresSuccessfulCIForTaggedSHA` |
| `RESILIENCE-009` | Umbrales obligatorios medidos en CI | `66dcec5`, `0823b39`, `e9a0ddf` | perfiles de cobertura finales `95.5/81.9/90.1` |
| `RESILIENCE-010` | Instaladores fallan cerrados y restauran binario previo | `66dcec5`, `24126c3` | `scripts/test-install.sh`, `scripts/test-install.ps1` |
| `RISK-001` | Hash/plan validan el learning solicitado | `167ac8f` | `TestPublishRejectsPreviewOwnedByAnotherLearning` |
| `RISK-002` | No hay recomposición durante apply | `167ac8f` | `TestPublishAppliesPreviewedBytesAfterLearningDrift` |
| `RISK-003` | Aprobación deriva del impacto resuelto, no del boolean persistido | `167ac8f` | `plan_enforcement_test.go`, `safety_edges_test.go` |
| `RISK-004` | Snapshot de preview rechaza target y padre symlink | `4a7f15b`, `e6d846f` | `TestSymlinkSafetyBranchesWhenSupported` |
| `RISK-005` | Operaciones destructivas ocurren dentro de `os.Root` | `4a7f15b` | `filesystem_safety_test.go`, `filesystem_hardening_test.go` |
| `RISK-006` | Materializador de records usa operaciones rooted/no-follow | `4a7f15b` | `TestRecordMaterializerDoesNotCreateThroughSymlinkedAncestor` |
| `RISK-007` | Rollback compara hash y modo publicados | `4a7f15b` | `TestRestoreCASIncludesPublishedAndOriginalModes` |
| `RISK-008` | Rollback antiguo no pisa destino de publicación nueva | `2698394` | `TestRollbackRejectsOlderOverlappingPublication` |
| `RISK-009` | Lock real serializa publish, rollback y compensación | `2698394` | `TestConcurrentPublishIsRejectedByProjectLock` |

### Verificación final local

```text
go fmt ./...                                      PASS
go test -race ./...                              PASS
go test -race -count=1 ./internal/publish        PASS en e6d846f
go vet ./...                                     PASS
go mod verify                                    PASS
coverage CI exacta domain/storage/publish        95.5% / 81.9% / 90.1%
6 cross-builds CGO_ENABLED=0                     PASS
install/update/mismatch/rollback/uninstall       PASS en PowerShell y WSL
candidato v0.1.10 doctor --json                  ok=true (6 degradaciones explícitas)
candidato v0.1.10 e2e --temp                     37/37 PASS, incluido MCP stdio real
```

Una corrida sin race y sin cache encontró el flake ambiental ya documentado:
Windows negó la ejecución temporal de `internal/buildinfo.test.exe` y demoró el
cleanup de un `TempDir` de evidence. El mismo test de buildinfo compilado con
`go test -c` y ejecutado desde ruta estable pasó 3/3; la suite completa con race
pasó después. No hubo fallo de aserción.

GitHub Actions continúa **NOT_RUN** porque no hubo push. No se creó tag, PR,
issue ni release, no se ejecutó GoReleaser y no se alteró la línea de revisión
congelada.
