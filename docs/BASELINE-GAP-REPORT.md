# Informe de brechas de la base real — royo-learn

> Entregable del **Tramo 0** de `docs/PLAN-recuperacion-contrato.md`.
> Redactado el 2026-07-14. Ninguna línea de código fue modificada para producirlo.
> Cada celda de las tablas está respaldada por un `archivo:línea` efectivamente leído
> o por la salida real de un comando ejecutado en esta máquina.

---

## 1. Registro de partida (§0.1)

| Dato | Valor |
|------|-------|
| Rama de trabajo | `fix/v019-contract-recovery` |
| Commit de partida (HEAD) | `60b94629fbec956e89f78e7add8c83437c021f0d` |
| Commit padre | `a00143fc39f2aaa1f55053789666d562e45c0f18` |
| Tag `v0.1.9` (objeto anotado) | `aa1dbbe070add63fb38503837c69eff9e5c82427` |
| Tag `v0.1.9` (commit apuntado) | `a00143fc39f2aaa1f55053789666d562e45c0f18` |
| Commit actual de `main` | `a00143fc39f2aaa1f55053789666d562e45c0f18` |
| `git describe --tags --always` | `v0.1.9-1-g60b9462` |
| Versión de Go | `go1.26.5 windows/amd64` |
| Sistema operativo | Windows 11 Home Single Language 10.0.26200 |
| Estado del repositorio | Limpio; único elemento sin seguimiento: `.playwright-mcp/` |

### Diferencia entre `main` y `v0.1.9` — verificada, no asumida

```text
$ git diff --stat main v0.1.9
(sin salida)
```

`main` y el tag `v0.1.9` apuntan **exactamente al mismo commit** (`a00143f`).
El Hallazgo 1 del plan queda confirmado por ejecución.

### Diferencia entre `v0.1.9` y el HEAD de partida

```text
$ git diff --stat v0.1.9 HEAD
 PROMPT_FOR_EXECUTOR.md             | 371 +++++++++++++++++++
 docs/PLAN-recuperacion-contrato.md | 715 +++++++++++++++++++++++++++++++++++++
 2 files changed, 1086 insertions(+)
```

El único commit por delante de `v0.1.9` (`60b9462`, "docs: add contract recovery
plan and per-session executor prompts") es **exclusivamente documental**. No
introduce ni altera código. La base funcional analizada en este informe es, por
tanto, idéntica a `v0.1.9`.

---

## 2. Baseline ejecutable (§0.2)

| Comando | Resultado real |
|---------|----------------|
| `go version` | `go version go1.26.5 windows/amd64` |
| `go mod verify` | `all modules verified` |
| `go vet ./...` | Limpio, código de salida `0` |
| `go build ./cmd/royo-learn` | Correcto, código de salida `0` |
| `go test ./...` | **FALLO** únicamente en `internal/buildinfo` — ver §2.1 |
| `go test -race ./...` | **TODOS LOS PAQUETES CORRECTOS**, incluido `internal/buildinfo` |
| `royo-learn version` | `royo-learn dev` / `commit: unknown` / `built: unknown` / `go: go1.26.5` |
| `royo-learn --help` | Lista 17 comandos — ver §4.1 (incluye un comando fantasma) |
| `royo-learn e2e --temp` | `{"passed": 9, "failed": 0, "total": 9, ...}` — salida `0`, ver §6 |

### 2.1 FALLO AMBIENTAL — `internal/buildinfo` (no es una brecha de código)

```text
$ go test ./...
fork/exec C:\go-tmp\go-build685297445\b286\buildinfo.test.exe: Access is denied.
FAIL	agent-royo-learn/internal/buildinfo	0.410s
```

**Clasificación: FALLO AMBIENTAL. No es un defecto del código y no debe bloquear
ninguna puerta de salida futura.**

Evidencia que sostiene la clasificación:

1. `go vet ./internal/buildinfo/` está limpio.
2. Se intentó redirigir `GOTMPDIR` a dos directorios locales con permisos de
   escritura (el scratchpad de la sesión y un `.gotmp` dentro del repositorio).
   **Ambos fallaron con el mismo error**, lo que descarta que el problema sea el
   directorio temporal concreto: el bloqueo aplica a cualquier `.exe` generado
   dentro de un árbol `go-build`.
3. Prueba decisiva — compilar el binario de test a una ruta estable y ejecutarlo
   directamente:

   ```text
   $ go test -c -o .gotmp/buildinfo.test.exe ./internal/buildinfo/
   $ ./.gotmp/buildinfo.test.exe -test.v
   --- PASS: TestDevelopmentMetadataDefaults (0.00s)
   --- PASS: TestHumanString (0.00s)
   --- PASS: TestVersionJSON (0.00s)
   PASS
   ```

   Las tres pruebas **pasan**.
4. `go test -race ./...` ejecuta y aprueba `internal/buildinfo` sin incidencias.

Causa: la política de ejecución / antivirus de esta máquina Windows deniega el
enlazado o la ejecución de binarios de prueba recién creados dentro de los
directorios `go-build`. La prueba nunca llega a correr. El código es correcto.

**Consecuencia operativa:** en este entorno, la señal de confianza para la suite
completa es `go test -race ./...`, que pasa al 100 %.

---

## 3. Tabla de hechos verificados

Re-confirmación de los hallazgos del §0 del plan contra el HEAD de partida.
Los marcados **CORREGIDO** estaban mal formulados en el plan original: la
formulación correcta es la que figura aquí.

| # | Hallazgo (formulación correcta) | Evidencia | Estado |
|---|--------------------------------|-----------|--------|
| 1 | `main` y el tag `v0.1.9` apuntan al mismo commit (`a00143f`); el diff es vacío | `git diff --stat main v0.1.9` sin salida | VERIFICADO |
| 2 | Las Skills incluidas invocan tools `learning_*` | `skills/*/SKILL.md` — ver §5 | VERIFICADO |
| 3 | El servidor MCP registra nombres distintos (`capture_learning`, `search_learnings`, …) | `internal/mcpserver/profiles.go:9-119` | VERIFICADO — la intersección con los nombres citados por las Skills es **vacía** |
| 4 | No existe tool MCP de aprobación, evidencia posterior, rollback, occurrence ni status | `internal/mcpserver/profiles.go:8-119` (10 tools, ninguna de ellas) | VERIFICADO |
| 5 | Perfiles en código: `minimal`/`standard`/`full` (default `standard`); el contrato exige `--tools read\|agent\|admin` | `cmd/royo-learn/mcp.go:26`; `docs/04-CLI-SPEC.md:229-242` | VERIFICADO — además el **flag** difiere: el código usa `--profile`, el contrato `--tools` |
| 6 | **CORREGIDO.** `publish` **sí es alcanzable** en una instalación por defecto a través del CLI (`cmd/royo-learn/main.go:69`), de forma independiente del perfil. Lo que está restringido al perfil `full` es **únicamente la superficie MCP** (`publish_learning`), quedando fuera del perfil por defecto `standard`. | `cmd/royo-learn/main.go:69`; `internal/mcpserver/profiles.go:56`; `cmd/royo-learn/mcp.go:26` | CORREGIDO |
| 7 | **CORREGIDO.** `curate` no expone dos subcomandos posicionales. Expone **cinco acciones** mediante el flag `--action`: `approve`, `approve_new_skill`, `approve_skill_update`, `reject`, `needs_evidence`. | `cmd/royo-learn/main.go:756-764`; uso real en `cmd/royo-learn/e2e.go:146` | CORREGIDO |
| 8 | CLI documentada pero ausente del dispatcher: `get`, `list`, `approve`, `occurrence`, `review`, `export`, `import`, `rebuild-index`, `mcp` | `docs/04-CLI-SPEC.md:91,100,158,186,199,209,217,225,229` frente a `cmd/royo-learn/main.go:57-88` | VERIFICADO — y `search` es un caso peor: ver Hallazgo 15 |
| 9 | **CORREGIDO.** El plan sobredimensiona esta brecha. `setup` **sí está documentado** (`docs/08-GENTLE-AI-CODEX-INTEGRATION.md:138-180`) y `mcp-serve` **aparece** en `docs/FINAL-IMPLEMENTATION-REPORT.md:139`. `engram-search` se menciona una vez en `docs/FINAL-IMPLEMENTATION-REPORT.md:105`. El único comando **sin mención alguna** en toda la documentación es `engram-health`. Ninguno de estos comandos, sin embargo, tiene entrada en `docs/04-CLI-SPEC.md`, que es el contrato del CLI. | citas anteriores; `cmd/royo-learn/main.go:75,77,79,81,85` | CORREGIDO |
| 10 | **CORREGIDO — el plan se queda corto.** El E2E **sí intenta** la aprobación (`curate --action approve`, `cmd/royo-learn/e2e.go:139-149`) y a continuación **absorbe el fallo** en `:151-157`. El soft-pass no es meramente permisivo: **enmascara activamente la ruta de aprobación rota** e impide que CI la detecte. | `cmd/royo-learn/e2e.go:139-149,151,157` | CORREGIDO |
| 11 | El instalador de Skills omite toda Skill existente ("Existing skills are skipped (never overwritten)") | `internal/setup/skill.go:20,49-52` | VERIFICADO |
| 12 | Existen piezas internas reutilizables (`internal/evidence`, `internal/publish`, `internal/recurrence`, `internal/curate`, `internal/doctor`) | árbol `internal/` | VERIFICADO — **con una salvedad grave**: `internal/evidence` es un paquete **huérfano** (ver Hallazgo 16) |
| 13 | `publish` CLI ya exige `--preview-hash`; el preview expone `requires_approval` | `cmd/royo-learn/main.go:852`; `internal/publish/preview.go:138,143` | VERIFICADO — faltan `--approval-id` y `--apply` (`cmd/royo-learn/main.go:851-855`) |
| **14** | **HALLAZGO PRINCIPAL — Bloqueo de aprobación (deadlock).** Ver §3.1. | ver §3.1 | **NUEVO — VERIFICADO** |
| **15** | **Comando fantasma `search`.** `royo-learn --help` anuncia `search` (`cmd/royo-learn/main.go:109`), pero **no existe ningún `case "search"`** en el dispatcher (`cmd/royo-learn/main.go:57-88`) **ni ninguna función `runSearch`**. Ejecutarlo devuelve código `2` con un mensaje de error incorrecto que habla de `version --json` (`cmd/royo-learn/main.go:127-136`). El propio repositorio ya lo sabía: `docs/FINAL-IMPLEMENTATION-REPORT.md:105` admite que "there is no dedicated `royo-learn search` CLI subcommand", y aun así el help lo sigue anunciando. | ver §4.1 | **NUEVO — VERIFICADO** |
| **16** | **`internal/evidence` es un paquete huérfano.** Ningún archivo fuera del propio paquete lo importa (cero importadores que no sean pruebas). `evidence.Redact` (`internal/evidence/redact.go:30`) **nunca se ejecuta en ninguna ruta de producción**. La captura solo acepta un `EvidenceLevel` declarativo (`internal/capture/capture.go:25,86-88`), nunca registros de evidencia. | ver §3.2 | **NUEVO — VERIFICADO** |
| **17** | **`rollback` es asimétrico.** Implementado y alcanzable por CLI (`cmd/royo-learn/main.go:71`; `internal/publish/publish_op.go:476`; `internal/publish/rollback.go:13,34`), pero **ausente del MCP**: no figura entre las 10 tools registradas. El plan lo listaba solo como brecha MCP; es en realidad una asimetría entre interfaces. | `internal/mcpserver/profiles.go:8-119` | **NUEVO — VERIFICADO** |
| **18** | **Las instrucciones MCP mienten sobre el perfil activo.** La respuesta `initialize` incluye un texto estático que enumera las **10 tools** cualquiera sea el perfil. En `minimal` solo hay 3 registradas y en `standard` 9, pero ambas anuncian `publish_learning`. | comparar §4.2 (observado) con `internal/mcpserver/profiles.go` | **NUEVO — VERIFICADO** |

### 3.1 Hallazgo 14 (PRINCIPAL) — Bloqueo de aprobación

La cadena verificada, con `archivo:línea`:

1. **La política marca `requires_approval`.**
   `internal/publish/policy.go:13-14` registra dos políticas:
   `policySharedScopeRequiresApproval` (`policy.go:51`) y
   `policyAgentsRuleRequiresApproval` (`policy.go:72`). Una tercera,
   `policyPreferenceTypeRequiresHuman` (`policy.go:31`), falla de forma
   incondicional cuando el aprendizaje es de tipo `preference` y su destino es
   `shared` o `AGENTS.md`.
   `RequiresHumanApproval` (`policy.go:20-27`) devuelve `true` si **cualquier**
   política falla, y el resultado se graba en el preview
   (`internal/publish/preview.go:138,143`).

2. **La publicación exige la aprobación.**
   `internal/publish/publish_op.go:62-63`:

   ```go
   if preview.RequiresApproval {
       approval, err = s.CheckApproval(ctx, input.PreviewHash)
   ```

   `CheckApproval` (`internal/publish/approval.go:80-95`) falla con
   `ErrApprovalRequired` si no encuentra un registro de aprobación.

3. **Nada puede crear ese registro de aprobación.**
   El único constructor es `publish.Service.Approve`
   (`internal/publish/approval.go:16`). Una búsqueda de `.Approve(` en todo el
   repositorio devuelve **cero llamadores**: ni en `cmd/`, ni en
   `internal/mcpserver/`, ni siquiera en las pruebas. Es código muerto.
   No existe comando `royo-learn approve` (no hay `case "approve"` en el
   dispatcher, `cmd/royo-learn/main.go:57-88`) ni tool `learning_approve`
   (`internal/mcpserver/profiles.go:8-119`).

**Segundo cerrojo, independiente del anterior (descubierto durante esta
verificación).** Para que la política de destino compartido *pase*, la curación
debe llevar la decisión `approve_shared_knowledge`; para `AGENTS.md`,
`approve_agents_rule` (`policy.go:55,75`). Ambas decisiones existen en el
dominio (`internal/domain/types.go:133,136`) y las acepta el servicio de
curación (`internal/curate/curate.go:226-230,284,301`), **pero
`parseCurateAction` del CLI no las mapea**: solo traduce `approve`,
`approve_new_skill`, `approve_skill_update`, `reject` y `needs_evidence`
(`cmd/royo-learn/main.go:756-764`).

Nota de precisión: el handler MCP `curate_learning` **sí** puede alcanzarlas,
porque pasa la cadena de decisión sin lista blanca
(`internal/mcpserver/tools.go:276`: `Decision: domain.CurationDecision(in.Decision)`).
Esto es una asimetría CLI/MCP adicional, no una salvación del defecto.

**Consecuencias exactas:**

- **Desde el CLI**, un aprendizaje con destino `shared` o `AGENTS.md` **nunca**
  puede satisfacer la política (no hay acción que emita la decisión requerida),
  luego `requires_approval` es siempre `true`, luego `CheckApproval` siempre se
  exige, luego la publicación **es imposible**: no existe ruta pública que cree
  la aprobación. Es un bloqueo total.
- **Desde cualquier interfaz**, todo aprendizaje de tipo `preference` con
  destino `shared` o `AGENTS.md` queda **permanentemente impublicable**: la
  política `policyPreferenceTypeRequiresHuman` falla sin escapatoria y la
  aprobación que la desbloquearía no puede crearse.
- En términos generales: **siempre que `requires_approval` sea `true`, publicar
  es imposible por toda interfaz pública, CLI o MCP.**

Este es el defecto central del producto. Precisamente el flujo que el README
describe como gobernado por aprobación humana es el que no se puede completar.

### 3.2 Hallazgo 16 — `internal/evidence` nunca se ejecuta

Ningún archivo fuera de `internal/evidence/` importa
`agent-royo-learn/internal/evidence` (búsqueda de importadores excluyendo
pruebas: sin resultados). El paquete contiene el blob store content-addressed
(`internal/evidence/blob.go:22,49,102,122`) y la redacción de secretos
(`internal/evidence/redact.go:30` `Redact`, `:57` `DetectSecrets`), todo con
pruebas propias que pasan — pero **ninguna ruta pública lo invoca**.

Esto tiene una consecuencia directa sobre el E2E: el comentario de
`cmd/royo-learn/e2e.go:273-275` justifica no comprobar la redacción afirmando
que "Secret redaction happens in the evidence layer (blob store) […] See
internal/evidence/redact.go". **Esa justificación es falsa**: esa capa jamás se
ejecuta. El paso llamado `security-secret-redaction` no verifica ninguna
redacción, y la excusa documentada para no verificarla no se sostiene.

---

## 4. Inventario real de interfaces

### 4.1 Comandos CLI reales

Dispatcher: `cmd/royo-learn/main.go:57-88`. Texto de ayuda: `main.go:95-121`.

| Comando | ¿En el help? | ¿En el dispatcher? | Implementación | ¿En `docs/04`? |
|---------|--------------|--------------------|----------------|-----------------|
| `version` | sí (`:116`) | sí (`:57`) | `main.go:142` | sí (`:15`) |
| `init` | sí (`:101`) | sí (`:59`) | `main.go:190` | sí (`:19`) |
| `doctor` | sí (`:108`) | sí (`:61`) | `main.go:325` | sí (`:43`) |
| `capture` | sí (`:103`) | sí (`:63`) | `main.go:459` | sí (`:72`) |
| `curate` | sí (`:104`) | sí (`:65`) | `main.go:599` | sí (`:133`) |
| `preview` | sí (`:105`) | sí (`:67`) | `main.go:787` | sí (`:146`) |
| `publish` | sí (`:106`) | sí (`:69`) | `main.go:849` | sí (`:170`) |
| `rollback` | sí (`:107`) | sí (`:71`) | `main.go:911` | sí (`:182`) |
| `mcp-serve` | sí (`:102`) | sí (`:73`) | `mcp.go:24` | **no** (docs/04 documenta `mcp`) |
| `engram-health` | sí (`:110`) | sí (`:75`) | `main.go:1053` | **no** |
| `engram-search` | sí (`:111`) | sí (`:77`) | `main.go:1116` | **no** |
| `recurrences` | sí (`:112`) | sí (`:79`) | `main.go:1194` | **no** |
| `metrics` | sí (`:113`) | sí (`:81`) | `main.go:1251` | **no** |
| `e2e` | sí (`:114`) | sí (`:83`) | `e2e.go:31` | sí (`:244`) |
| `setup` | sí (`:115`) | sí (`:85`) | `setup.go:45` | **no** (sí en docs/08:138) |
| `self-update` | sí (`:117`) | sí (`:87`) | `main.go:1370` | sí (`:248`) |
| **`search`** | **sí (`:109`)** | **NO** | **NO EXISTE** | sí (`:114`) |

Subcomandos de `setup`: `install` (`setup.go:62`), `uninstall` (`setup.go:118`),
`status` (`setup.go:159`). **No existe `upgrade-skills`.**

Comprobación ejecutada del comando fantasma:

```text
$ royo-learn search --query test
{"code":"invalid_argument","message":"invalid arguments: expected \"version --json\"", ...}
exit=2
```

El mensaje de error no solo es incorrecto respecto al comando invocado, sino que
menciona `version --json` (`cmd/royo-learn/main.go:127-136` reutiliza el mensaje
de argumentos inválidos de `version`).

### 4.2 Tools MCP reales por perfil — OBSERVADO

Obtenido conduciendo el servidor real por stdio (`royo-learn mcp-serve --profile
<perfil>`), con `initialize` seguido de `tools/list` en JSON-RPC 2.0. **No es
derivado del código: es la respuesta observada.**

`initialize` responde en los tres perfiles:
`protocolVersion: "2024-11-05"`, `serverInfo: {name: "royo-learn", version: "dev"}`,
`capabilities: {logging, tools:{listChanged:true}}`.

| Tool | `minimal` | `standard` (por defecto) | `full` |
|------|:---------:|:------------------------:|:------:|
| `capture_learning` | ✅ | ✅ | ✅ |
| `search_learnings` | ✅ | ✅ | ✅ |
| `doctor` | ✅ | ✅ | ✅ |
| `curate_learning` | — | ✅ | ✅ |
| `preview_publication` | — | ✅ | ✅ |
| `list_learnings` | — | ✅ | ✅ |
| `get_learning` | — | ✅ | ✅ |
| `list_recurrences` | — | ✅ | ✅ |
| `compute_metrics` | — | ✅ | ✅ |
| `publish_learning` | — | — | ✅ |
| **Total `tools/list`** | **3** | **9** | **10** |

Observación adicional (Hallazgo 18): el campo `instructions` de `initialize`
enumera las **10 tools en los tres perfiles**. Un cliente conectado en `minimal`
recibe instrucciones que le prometen `publish_learning`, `curate_learning`,
`get_learning`, `list_learnings`, `list_recurrences` y `compute_metrics`, de las
cuales **ninguna está registrada** en ese perfil.

No existe ninguna tool de aprobación, evidencia posterior, rollback, occurrence
ni status en ningún perfil.

### 4.3 Contraste con el contrato documental

`docs/05-MCP-SPEC.md` especifica 11 tools: `learning_capture` (`:32`),
`learning_search` (`:71`), `learning_get` (`:87`), `learning_list` (`:91`),
`learning_curate` (`:95`), `learning_publication_preview` (`:122`),
`learning_approve` (`:140`), `learning_publish` (`:156`),
`learning_report_occurrence` (`:173`), `learning_status` (`:177`),
`learning_doctor` (`:183`).

**La intersección entre las 11 tools documentadas y las 10 registradas es
vacía.** Ningún nombre coincide.

---

## 5. Skills incompatibles (§0.3.e)

Se recorrieron todos los `skills/**/SKILL.md`. Se extrajeron los nombres de tools
MCP citados y se contrastaron contra el registro real
(`internal/mcpserver/profiles.go`).

| Archivo de Skill | Tool citada | ¿Existe en el registro? |
|------------------|-------------|-------------------------|
| `skills/capture-learning/SKILL.md` | `learning_capture` | **NO** |
| `skills/capture-learning/SKILL.md` | `learning_search` | **NO** |
| `skills/curate-learning/SKILL.md` | `learning_curate` | **NO** |
| `skills/curate-learning/SKILL.md` | `learning_get` | **NO** |
| `skills/publish-learning/SKILL.md` | `learning_publication_preview` | **NO** |
| `skills/publish-learning/SKILL.md` | `learning_approve` | **NO** |
| `skills/publish-learning/SKILL.md` | `learning_publish` | **NO** |
| `skills/royo-learn-onboarding/SKILL.md` | (no cita ninguna tool MCP) | n/a |

**7 de 7 tools citadas por las Skills no existen.** La tasa de aciertos es cero.
Ninguna Skill incluida en el repositorio puede invocar el servidor MCP.

Agravante: `learning_approve` no solo no existe con ese nombre — **no existe
ninguna tool de aprobación bajo ningún nombre** (Hallazgo 14). La Skill
`publish-learning` describe un flujo que el binario no puede ejecutar.

Agravante adicional (Hallazgo 11): `internal/setup/skill.go:49-52` omite toda
Skill ya instalada. Corregir las Skills del repositorio **no repara** las copias
que los usuarios de versiones anteriores ya tienen instaladas.

---

## 6. Falsos positivos del E2E (§0.4)

Archivo: `cmd/royo-learn/e2e.go`. El comando reporta **9/9 pasos correctos y
código de salida 0** sobre una base cuyo flujo principal está bloqueado (§3.1).
A continuación, cada punto donde el E2E acepta como éxito algo que no lo es.
**Ninguno fue corregido.**

### FP-1 — `curate`: un fallo obligatorio se acepta (`e2e.go:150-158`)

```go
// Curate may fail if evidence thresholds aren't met.
// This is acceptable — we verify the command doesn't crash.
if code != exitSuccess {
    errStr := errOut.String()
    if !strings.Contains(errStr, "code") && !strings.Contains(errStr, "evidence") {
        return fmt.Errorf("curate failed with unexpected error: %s", errStr)
    }
    return nil // Soft pass: expected domain guard.
}
```

- `:151` — el comentario declara explícitamente que el fallo "es aceptable".
- `:157` — `return nil` convierte un fallo en un PASS.
- `:154` — el guardián es `strings.Contains(errStr, "code")`. Toda la salida de
  error del binario es un sobre JSON que **siempre** contiene la clave `"code"`
  (`internal/logging`, usado por `writeCurateError`, `main.go:770-780`). Por
  tanto la condición se cumple **para cualquier error posible** y el soft-pass
  es, en la práctica, incondicional.
- **No se verifica ningún cambio de estado**: nunca se comprueba que el
  aprendizaje haya pasado a `approved`.

Este es el punto exacto donde el bloqueo de aprobación (§3.1) queda oculto para CI.

### FP-2 — `preview`: se acepta la ausencia del efecto buscado (`e2e.go:172-182`)

```go
if code != exitSuccess {
    errStr := errOut.String()
    if strings.Contains(errStr, "must be approved") || strings.Contains(errStr, "invalid_transition") {
        return nil // Expected: learning not approved yet.
    }
    ...
}
if !json.Valid(out.Bytes()) {
    return fmt.Errorf("preview output is not valid JSON")
}
```

- `:174-175` — si el aprendizaje no está aprobado (que es justo lo que ocurre
  tras el soft-pass de FP-1), el paso **pasa sin haber generado preview alguno**.
  Es un soft-pass en cascada: FP-1 deja el aprendizaje sin aprobar y FP-2
  convierte esa consecuencia en éxito.
- `:179` — cuando sí hay salida, la única comprobación es `json.Valid`. No se
  verifica `preview_hash`, ni destinos, ni `requires_approval`, ni el diff.

### FP-3 — `recurrences`: solo se valida que el JSON sea JSON (`e2e.go:218-224`)

```go
if code != exitSuccess {
    return fmt.Errorf("recurrences failed: %s", errOut.String())
}
if !json.Valid(out.Bytes()) {
    return fmt.Errorf("recurrences output is not valid JSON")
}
```

- `:221` — una lista vacía `[]` es JSON válido. **La ausencia de datos se acepta
  como éxito.** El paso pasaría igual si el subsistema de recurrencias no
  registrara absolutamente nada.

### FP-4 — `security-path-traversal`: la prueba de seguridad no ejecuta el ataque (`e2e.go:229-250`)

```go
code := run([]string{"capture", ..., "--context", "../../../etc/passwd", ...})
_ = code // Accept any exit code — we care about file safety.
_ = out

entries, _ := os.ReadDir(tempDir)
for _, e := range entries {
    if e.Name() == "etc" || e.Name() == "passwd" { ... }
}
```

- `:240` — **se acepta explícitamente cualquier código de salida.**
- El "ataque" inyecta la ruta en `--context`, que es un campo de texto libre y
  **nunca se usa como ruta del sistema de ficheros**. La superficie que sí
  escribe archivos es la publicación, y el E2E **no la ejercita en ningún paso**.
  El ataque no se dirige contra el componente vulnerable.
- `:244-249` — la comprobación busca entradas llamadas `etc` o `passwd`
  **dentro de `tempDir`**. Un path traversal exitoso escribiría **fuera** de
  `tempDir` por definición. Se está mirando en el lugar equivocado: esta
  aserción no puede detectar el fallo que dice buscar.

### FP-5 — `security-secret-redaction`: no se comprueba ninguna redacción (`e2e.go:255-277`)

```go
// Verify JSON output is valid.
if !json.Valid(out.Bytes()) {
    return fmt.Errorf("capture output is not valid JSON")
}
// NOTE: Records store raw observations by design.
// Secret redaction happens in the evidence layer (blob store),
// not during capture. See internal/evidence/redact.go.
```

- El paso captura la cadena `sk-proj-redactiontest12345` (`:262`) y **nunca
  comprueba que haya sido redactada** en ningún sitio. Las únicas aserciones son
  "no falló" (`:266`) y "es JSON válido" (`:270`).
- `:273-275` — la justificación documentada es **falsa**. Remite a
  `internal/evidence/redact.go`, pero ese paquete es **huérfano**: no lo importa
  nadie (Hallazgo 16). La redacción que el comentario invoca como excusa **nunca
  se ejecuta en ninguna ruta del producto**.
- Un paso llamado `security-secret-redaction` que jamás comprueba una redacción
  es un falso positivo de seguridad.

### FP-6 — El estado no se verifica en ningún paso

Ningún paso del E2E lee el estado del aprendizaje tras una transición. No hay una
sola aserción sobre `status == "approved"`, `status == "published"` ni
`status == "rolled_back"`.

### FP-7 — Operaciones críticas nunca ejercitadas

De las 26 operaciones de la matriz (§7), el E2E ejercita 6: `init`, `capture`,
`curate`, `preview`, `doctor`, `recurrences`. **No ejercita en absoluto**:
`publish`, `approve`, `rollback`, `occurrence`, `get`, `list`, `search`,
`metrics`, ni ninguna operación MCP. No existe ningún E2E de MCP.

Es la razón directa de que el bloqueo de aprobación (§3.1) haya sobrevivido hasta
`v0.1.9` con CI en verde.

### FP-8 — Los pasos no se omiten al fallar el anterior, pero arrastran estado vacío

`executeE2ESteps` (`e2e.go:61-301`) ejecuta los 9 pasos incondicionalmente. Si el
paso 2 (`capture`, `:81`) fallara, `learningID` quedaría como cadena vacía
(`:80`) y los pasos 4, 5 y 7 se ejecutarían con `--learning-id ""`. Sus soft-passes
(FP-1, FP-2, FP-3) podrían absorber los errores resultantes y reportar PASS sobre
un identificador inexistente.

### FP-9 — El paso `capture-idempotent` no prueba idempotencia (`e2e.go:109-136`)

El paso comprueba la deduplicación por hash de contenido (`:127-134`), no la
semántica de `idempotency_key` que el plan define en D5. El nombre del paso
describe una garantía que no verifica.

---

## 7. Matriz obligatoria de operaciones (§0.3.c)

Estados: `FUNCTIONAL` `PARTIAL` `INTERNAL_ONLY` `DOCUMENTED_ONLY`
`BROKEN_INTEGRATION` `MISSING`.

Toda celda está respaldada por un `archivo:línea` leído durante este tramo.
"Prueba real" significa una prueba que verifica un efecto de negocio observable,
no la mera ausencia de crash.

| Operación | Documentada | CLI | MCP | Skill | Servicio interno | Prueba real | Estado |
|-----------|-------------|-----|-----|-------|------------------|-------------|--------|
| **capture** | sí — `docs/04:72` | sí — `main.go:63` → `main.go:459` | sí — `capture_learning`, `profiles.go:9-19` (min/std/full) | cita `learning_capture` — **no existe** (`skills/capture-learning/SKILL.md`) | `internal/capture/capture.go` | parcial — `e2e.go:81-106`; sin evidencia (`capture.go:25,86-88`) | **BROKEN_INTEGRATION** |
| **evidence (add)** | **no** — sin entrada en `docs/04` | **no** | **no** | — | `internal/evidence/blob.go:22,49` + `redact.go:30` — **paquete huérfano, 0 importadores** | solo unitarias internas | **INTERNAL_ONLY** |
| **search** | sí — `docs/04:114` | **FANTASMA** — anunciado en `main.go:109`, sin `case` (`main.go:57-88`), sin implementación | sí — `search_learnings`, `profiles.go:20-30` (min/std/full) | cita `learning_search` — **no existe** | FTS5 en `internal/storage` | no | **BROKEN_INTEGRATION** |
| **get** | sí — `docs/04:91` | **no** | sí — `get_learning`, `profiles.go:75-85` (std/full) | cita `learning_get` — **no existe** | `internal/storage` | no | **BROKEN_INTEGRATION** |
| **list** | sí — `docs/04:100` | **no** | sí — `list_learnings`, `profiles.go:64-74` (std/full) | — | `internal/storage` | no | **PARTIAL** |
| **curate** | sí — `docs/04:133` | sí — `main.go:65` → `main.go:599`; 5 acciones vía `--action` (`main.go:756-764`) | sí — `curate_learning`, `profiles.go:31-41` (std/full); decisión sin lista blanca (`tools.go:276`) | cita `learning_curate` — **no existe** | `internal/curate/curate.go:226-230` | soft-pass — `e2e.go:151,157` (FP-1) | **BROKEN_INTEGRATION** |
| **preview** | sí — `docs/04:146` | sí — `main.go:67` → `main.go:787` | sí — `preview_publication`, `profiles.go:42-52` (std/full) | cita `learning_publication_preview` — **no existe** | `internal/publish/preview.go:19` | soft-pass — `e2e.go:174-175` (FP-2) | **BROKEN_INTEGRATION** |
| **approve** | sí — `docs/04:158`; `docs/05:140` | **no** — sin `case "approve"` (`main.go:57-88`) | **no** — sin tool de aprobación (`profiles.go:8-119`) | cita `learning_approve` — **no existe** | `internal/publish/approval.go:16` — **CERO llamadores (código muerto)** | **ninguna** | **BROKEN_INTEGRATION** (Hallazgo 14) |
| **publish** | sí — `docs/04:170` | sí — `main.go:69` → `main.go:849`; sin `--apply` ni `--approval-id` (`main.go:851-855`) | sí — `publish_learning`, `profiles.go:53-63` (**solo `full`**) | cita `learning_publish` — **no existe** | `internal/publish/publish_op.go:19`; exige aprobación en `:62-63` | **ninguna** — el E2E no lo ejercita | **BROKEN_INTEGRATION** (bloqueado por Hallazgo 14) |
| **rollback** | sí — `docs/04:182` | sí — `main.go:71` → `main.go:911` | **no** — ausente del registro (`profiles.go:8-119`) | — | `internal/publish/publish_op.go:476`; `rollback.go:13,34` | unitarias; no en E2E | **PARTIAL** (Hallazgo 17) |
| **report occurrence** | sí — `docs/04:186`; `docs/05:173` | **no** | **no** | — | `internal/recurrence/recurrence.go:18` `RecordRecurrence` | no | **INTERNAL_ONLY** |
| **recurrences** | **no** — sin entrada en `docs/04` | sí — `main.go:79` → `main.go:1194` | sí — `list_recurrences`, `profiles.go:97-107` (std/full) | — | `internal/recurrence` | falso positivo — `e2e.go:221` (FP-3) | **PARTIAL** |
| **metrics** | **no** — sin entrada en `docs/04` | sí — `main.go:81` → `main.go:1251` | sí — `compute_metrics`, `profiles.go:108-118` (std/full) | — | `internal/recurrence` | no | **PARTIAL** |
| **status** | sí — `docs/05:177`; `docs/15-OPERATIONS.md:11` | **no** | **no** | — | **no existe** | no | **DOCUMENTED_ONLY** |
| **doctor** | sí — `docs/04:43` | sí — `main.go:61` → `main.go:325` | sí — `doctor`, `profiles.go:86-96` (min/std/full) | — | `internal/doctor` | sí — `e2e.go:187-206` verifica `ok=true` | **FUNCTIONAL** |
| **review** | sí — `docs/04:199` | **no** | **no** | — | parcial en `internal/recurrence` (`CheckNeedsReview`) | no | **DOCUMENTED_ONLY** |
| **export** | sí — `docs/04:209` | **no** | **no** | — | **no existe** | no | **DOCUMENTED_ONLY** |
| **import** | sí — `docs/04:217` | **no** | **no** | — | **no existe** | no | **DOCUMENTED_ONLY** |
| **rebuild-index** | sí — `docs/04:225` | **no** | **no** | — | **no existe** | no | **DOCUMENTED_ONLY** |
| **setup** | sí — `docs/08:138-180` (no en `docs/04`) | sí — `main.go:85` → `setup.go:45`; `install`/`uninstall`/`status` (`setup.go:62,118,159`) | **no** | — | `internal/setup` | unitarias | **PARTIAL** |
| **skill upgrade** | **no** | **no** — no existe `setup upgrade-skills` (`setup.go:55`) | **no** | — | `internal/setup/skill.go:49-52` **omite las existentes** | no | **MISSING** (Hallazgo 11) |
| **self-update** | sí — `docs/04:248` | sí — `main.go:87` → `main.go:1370` | **no** | — | `internal/selfupdate` | unitarias | **FUNCTIONAL** |
| **mcp / mcp-serve** | sí — `docs/04:229` documenta `mcp` con `--tools read\|agent\|admin` | implementa **`mcp-serve`** con `--profile minimal\|standard\|full` (`mcp.go:24,26`) | n/a | — | `internal/mcpserver` | unitarias; sin E2E MCP | **BROKEN_INTEGRATION** (nombre, flag y perfiles divergen) |
| **engram-health** | **no** — sin mención en toda la documentación | sí — `main.go:75` → `main.go:1053` | **no** | — | `internal/engram` | unitarias | **PARTIAL** |
| **engram-search** | parcial — solo `docs/FINAL-IMPLEMENTATION-REPORT.md:105` | sí — `main.go:77` → `main.go:1116` | **no** | — | `internal/engram` | unitarias | **PARTIAL** |
| **e2e** | sí — `docs/04:244` | sí — `main.go:83` → `e2e.go:31` | n/a | — | `cmd/royo-learn/e2e.go` | **es la prueba, y es falsa** — 9 pasos, 5 falsos positivos (§6) | **BROKEN_INTEGRATION** |

### Resumen de la matriz

| Estado | Operaciones | Total |
|--------|-------------|-------|
| `FUNCTIONAL` | doctor, self-update | **2** |
| `PARTIAL` | list, rollback, recurrences, metrics, setup, engram-health, engram-search | **7** |
| `INTERNAL_ONLY` | evidence (add), report occurrence | **2** |
| `DOCUMENTED_ONLY` | status, review, export, import, rebuild-index | **5** |
| `BROKEN_INTEGRATION` | capture, search, get, curate, preview, approve, publish, mcp/mcp-serve, e2e | **9** |
| `MISSING` | skill upgrade | **1** |
| | | **26** |

**2 de 26 operaciones están plenamente funcionales.** Ninguna de las dos
(`doctor`, `self-update`) forma parte del recorrido principal del producto.
Todo el recorrido principal —capturar, curar, previsualizar, aprobar,
publicar— está en estado `BROKEN_INTEGRATION`.

---

## 8. Contradicciones con el plan detectadas en este tramo

1. **Hallazgo 6** estaba mal formulado: `publish` sí es alcanzable por CLI en una
   instalación por defecto. La restricción a `full` afecta solo al MCP.
2. **Hallazgo 7** estaba mal formulado: `curate` tiene cinco acciones vía
   `--action`, no dos subcomandos posicionales.
3. **Hallazgo 9** sobredimensionaba la brecha: `setup` y `mcp-serve` sí están
   documentados fuera de `docs/04`; solo `engram-health` carece de toda mención.
4. **Hallazgo 10** se quedaba corto: el E2E sí intenta aprobar y luego enmascara
   el fallo. El soft-pass de `e2e.go:154` es efectivamente incondicional, porque
   todo sobre de error JSON contiene la clave `"code"`.
5. **El plan no contemplaba el bloqueo de aprobación** (Hallazgo 14), que es el
   defecto central: `publish.Service.Approve` es código muerto y `publish` exige
   una aprobación que ninguna ruta pública puede crear.
6. **El plan no contemplaba el comando fantasma `search`** (Hallazgo 15),
   anunciado en el help y sin implementación alguna.
7. **El plan no contemplaba que `internal/evidence` es un paquete huérfano**
   (Hallazgo 16). La regla transversal §1.3 del plan ("reutilizar antes de
   reemplazar") sigue siendo válida, pero debe advertir que este paquete nunca
   ha estado conectado: no es "reconectarlo", es conectarlo por primera vez.
8. **El plan no contemplaba el segundo cerrojo del bloqueo**: el CLI no puede
   emitir las decisiones de curación `approve_shared_knowledge` ni
   `approve_agents_rule` que las políticas exigen (`main.go:756-764` frente a
   `policy.go:55,75`), mientras que el MCP sí puede (`tools.go:276`).
9. **El plan no contemplaba que las instrucciones MCP anuncian las 10 tools en
   todos los perfiles** (Hallazgo 18), incluidas las no registradas.

---

## 9. Conclusión del Tramo 0

La base compila, `go vet` está limpio, `go test -race ./...` pasa al 100 % y el
E2E reporta 9/9. **Ninguna de esas señales refleja el estado real del producto.**

El recorrido principal está roto en tres puntos independientes y acumulativos:

1. Ninguna Skill incluida puede hablar con el servidor MCP (0 de 7 nombres existen).
2. Ningún aprendizaje que requiera aprobación puede publicarse por ninguna
   interfaz, porque la aprobación no tiene entrada pública y su servicio es
   código muerto.
3. El E2E que debería haber detectado ambos problemas está construido de forma
   que no puede fallar.

El tercer punto explica por qué los dos primeros llegaron a `v0.1.9`.
