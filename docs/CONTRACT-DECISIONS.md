# Decisiones de contrato — royo-learn

> Entregable del **Tramo 1** de `docs/PLAN-recuperacion-contrato.md`.
> Redactado el 2026-07-14 sobre la rama `fix/v019-contract-recovery`.
> **No se implementó nada en código antes de este documento.** Cero cambios bajo
> `cmd/`, `internal/`, `skills/`, `go.mod` o `go.sum`.
>
> Base de evidencia: `docs/BASELINE-GAP-REPORT.md` (Tramo 0) y lectura directa del
> código en el HEAD de partida. Cada decisión se justifica contra evidencia
> `archivo:línea`, nunca contra intuición (regla §1.1 del plan).

---

## Índice

| # | Decisión | Origen |
|---|----------|--------|
| D1 | Nombres MCP canónicos y aliases deprecated | Plan §Tramo 1 |
| D2 | Perfiles MCP y flag de selección | Plan §Tramo 1 |
| D3 | Evidencia mínima para aprobar | Plan §Tramo 1 |
| D4 | Aprobación humana obligatoria | Plan §Tramo 1 |
| D5 | Idempotencia y recurrencia | Plan §Tramo 1 |
| D6 | Fuente de verdad de datos | Plan §Tramo 1 |
| D7 | Aplicación de publicaciones (dry-run) | Plan §Tramo 1 |
| D8 | Compatibilidad y versión del Hito 1 | Plan §Tramo 1 |
| D9 | Comandos implementados pero no documentados | Plan §Tramo 1 |
| D10 | `mcp` frente a `mcp-serve` | Plan §Tramo 1 |
| D11 | **El bloqueo de aprobación (defecto central)** | Tramo 0 — Hallazgo 14 |
| D12 | El comando fantasma `search` | Tramo 0 — Hallazgo 15 |
| D13 | `internal/evidence` huérfano | Tramo 0 — Hallazgo 16 |
| D14 | Las instrucciones de `initialize` mienten sobre el perfil | Tramo 0 — Hallazgo 18 |

---

## Advertencia previa: dos correcciones al informe de brechas

Durante la redacción de D11 se releyó la cadena causal completa del bloqueo de
aprobación. **El informe del Tramo 0 llega a la conclusión correcta por un camino
parcialmente equivocado, y omite el cerrojo más profundo.** Las decisiones de este
documento se apoyan en la cadena corregida, verificada línea a línea en D11.

Resumen de las correcciones (detalle y evidencia en D11):

1. **`docs/BASELINE-GAP-REPORT.md:139-148` afirma que las políticas de destino
   compartido y `AGENTS.md` marcan `requires_approval`.** No pueden hacerlo: son
   **tautologías inalcanzables**. El destino de una curación se *deriva de la
   decisión* (`internal/curate/curate.go:259-329`), de modo que
   `destino == shared` ⟺ `decisión == approve_shared_knowledge`. La rama de fallo
   de esas dos políticas es código muerto. La consecuencia real no es un bloqueo:
   es un **agujero de gobernanza** — publicar en `AGENTS.md` **nunca** exige
   aprobación humana.
2. **El informe no detectó el cerrojo raíz.** `curate.checkEvidenceThreshold`
   (`internal/curate/curate.go:190-214`) exige al menos un registro de evidencia
   persistido, y **ninguna interfaz pública puede crear uno**
   (`storage.SaveEvidence` tiene cero llamadores de producción). Por tanto
   **ningún aprendizaje puede alcanzar `approved` por ninguna interfaz pública**,
   ni CLI ni MCP. El bloqueo empieza una etapa antes de donde el informe lo situó.

La conclusión del Hallazgo 14 —«siempre que `requires_approval` sea `true`,
publicar es imposible»— **se mantiene y se confirma**. Lo que cambia es el
mecanismo, su alcance y la existencia de un segundo defecto de signo contrario
(el agujero de gobernanza), que ninguna decisión previa cubría.

---

## D1 — Nombres MCP canónicos y aliases deprecated

### Contexto

- Las Skills incluidas invocan **7 tools** con prefijo `learning_*`
  (`docs/BASELINE-GAP-REPORT.md:314-323`).
- El servidor MCP registra **10 tools** con nombres distintos
  (`internal/mcpserver/profiles.go:8-119`).
- **La intersección entre ambos conjuntos es vacía**
  (`docs/BASELINE-GAP-REPORT.md:325`). Ninguna Skill del repositorio puede invocar
  el servidor MCP. Tasa de aciertos: 0 de 7.
- `docs/05-MCP-SPEC.md` especifica **11 tools** canónicas: `learning_capture`
  (`:32`), `learning_search` (`:71`), `learning_get` (`:87`), `learning_list`
  (`:91`), `learning_curate` (`:95`), `learning_publication_preview` (`:122`),
  `learning_approve` (`:140`), `learning_publish` (`:156`),
  `learning_report_occurrence` (`:173`), `learning_status` (`:177`),
  `learning_doctor` (`:183`).
- Dos de las 10 tools registradas —`list_recurrences` y `compute_metrics`
  (`internal/mcpserver/profiles.go:97-118`)— **no tienen nombre canónico en
  docs/05**. El plan no las contempla en D1. Sin decisión quedarían en limbo, que
  es exactamente lo que D9 prohíbe para el CLI.

### Opciones consideradas

1. **Corregir las Skills hacia los nombres del código.** Barato e inmediato.
2. **Corregir el código hacia el contrato documental, sin aliases.** Rompe a
   cualquier cliente MCP configurado contra `v0.1.9`.
3. **Corregir el código hacia el contrato, conservando los nombres antiguos como
   aliases deprecated que apuntan al mismo handler.**

### Decisión

**Opción 3.** El conjunto canónico del Hito 1 son **13 tools**: las 11 de
`docs/05-MCP-SPEC.md` más dos que los recorridos del Tramo 2 exigen:

```text
learning_capture              learning_publish
learning_add_evidence         learning_rollback
learning_search               learning_report_occurrence
learning_get                  learning_status
learning_list                 learning_doctor
learning_curate               learning_list_recurrences
learning_publication_preview  learning_compute_metrics
learning_approve
```

(Son 15 nombres en total; las dos últimas se justifican abajo.)

- `learning_add_evidence` se añade porque el Recorrido B del plan lo exige
  («Añadir al contrato `learning_add_evidence` (MCP)») y porque D11 lo convierte
  en la pieza que desbloquea todo el recorrido.
- `learning_rollback` se añade porque el escenario MCP del Recorrido E lo ejercita
  explícitamente, y porque hoy `rollback` existe en CLI y falta en MCP
  (Hallazgo 17, `docs/BASELINE-GAP-REPORT.md:132`).
- `learning_list_recurrences` y `learning_compute_metrics` se añaden para que
  **ninguna tool registrada quede sin nombre canónico**. Es una extensión sobre la
  lista del plan, justificada por la misma regla de «nada en limbo» que el plan
  aplica al CLI en D9.

**Aliases deprecated conservados en el Hito 1** (10, uno por cada tool hoy
registrada):

| Alias deprecated (v0.1.9) | Tool canónica |
|---------------------------|---------------|
| `capture_learning` | `learning_capture` |
| `search_learnings` | `learning_search` |
| `get_learning` | `learning_get` |
| `list_learnings` | `learning_list` |
| `curate_learning` | `learning_curate` |
| `preview_publication` | `learning_publication_preview` |
| `publish_learning` | `learning_publish` |
| `doctor` | `learning_doctor` |
| `list_recurrences` | `learning_list_recurrences` |
| `compute_metrics` | `learning_compute_metrics` |

Reglas vinculantes:

- Cada alias invoca **el mismo handler** y produce **exactamente el mismo
  resultado**. Cero duplicación de lógica.
- Cada alias emite un aviso de deprecación (D8).
- Las Skills incluidas en el repositorio usan **solo nombres canónicos**. Está
  prohibido corregir una Skill hacia un alias.
- Ninguna tool nueva (`learning_add_evidence`, `learning_rollback`,
  `learning_approve`, `learning_status`, `learning_report_occurrence`) recibe
  alias: no existía antes, no hay compatibilidad que preservar.

### Justificación

La regla de precedencia §1.1 del plan sitúa `docs/05-MCP-SPEC.md` por encima del
«comportamiento accidental del código». Los nombres registrados en
`profiles.go` son precisamente eso: comportamiento accidental. La Opción 1
consagraría el accidente y dejaría el contrato documental permanentemente falso.

La Opción 2 es correcta en el destino pero prematura: `main == v0.1.9` y existen
instalaciones que ya registraron el servidor. La estrategia de aliases permite
cumplir el contrato **sin romper a nadie**, que es exactamente la condición que
§2 del plan pone para que el Hito 1 sea `v0.1.10` (D8).

### Fecha

2026-07-14

---

## D2 — Perfiles MCP y flag de selección

### Contexto

- El código expone `--profile minimal|standard|full`, con `standard` por defecto
  (`cmd/royo-learn/mcp.go:26`).
- El contrato documenta `royo-learn mcp --tools read|agent|admin`
  (`docs/04-CLI-SPEC.md:229-242`), con la semántica: `read` = búsqueda y get;
  `agent` = ciclo normal; `admin` = import, rollback y reparación.
- **Divergen tres cosas a la vez**: el nombre del comando (D10), el nombre del
  flag y los valores del flag (`docs/BASELINE-GAP-REPORT.md:120`).
- Inventario observado por stdio (`docs/BASELINE-GAP-REPORT.md:272-283`):
  `minimal` = 3 tools, `standard` = 9, `full` = 10.
- `publish_learning` solo existe en `full` (`internal/mcpserver/profiles.go:56`),
  mientras el perfil por defecto es `standard`: **la publicación por MCP es
  inalcanzable en una instalación por defecto.**

### Opciones consideradas

1. Mantener `minimal/standard/full` y reescribir `docs/04` para que coincida.
2. Adoptar `read/agent/admin` como canónicos, con mapeo compatible desde los
   nombres antiguos.
3. Sobre la ubicación de `learning_publish`: **(a)** en `agent`; **(b)** solo en
   `admin`.

### Decisión

**Opción 2**, y **3(a): `learning_publish` vive en el perfil `agent`.**

Contrato del Hito 1:

- Flag canónico: `--tools read|agent|admin`. Flag deprecated: `--profile`, que
  sigue aceptándose y emite aviso de deprecación (D8).
- Valores deprecated aceptados en `--tools` y `--profile`: `minimal → read`,
  `standard → agent`, `full → admin`.
- Perfil por defecto: `agent` (equivalente al `standard` actual — no cambia el
  comportamiento por defecto de ninguna instalación existente).

Distribución de las 15 tools canónicas por perfil:

| Perfil | Tools |
|--------|-------|
| `read` | `learning_search`, `learning_get`, `learning_list`, `learning_status`, `learning_doctor` |
| `agent` | todo `read` + `learning_capture`, `learning_add_evidence`, `learning_curate`, `learning_publication_preview`, `learning_approve`, `learning_publish`, `learning_report_occurrence`, `learning_list_recurrences`, `learning_compute_metrics` |
| `admin` | todo `agent` + `learning_rollback` (y, en el Hito 2, `learning_import` y `learning_rebuild_index`) |

Anotaciones MCP obligatorias por tool: `read` / `write` / `destructive`.
`learning_publish` se anota como **write**, no como destructive: no destruye, y su
protección real es la aprobación (D4, D7, D11). `learning_rollback`,
`learning_import` y `learning_rebuild_index` se anotan **destructive** y quedan
confinadas a `admin`. **Ninguna tool destructive puede aparecer en `read` ni en
`agent`.**

### Justificación

- El plan recomienda `agent` para `learning_publish` y **el informe de brechas no
  aporta ninguna evidencia en contra**; aporta evidencia a favor. `docs/04:241`
  define `agent` como «ciclo normal», y publicar es el paso final del ciclo
  normal. Dejar `publish` fuera del perfil por defecto es precisamente lo que hoy
  produce el Hallazgo 6: una publicación documentada que el MCP no puede ejecutar.
- **La protección no debe ser el perfil, debe ser la aprobación.** El perfil es
  una lista de tools visibles; la política de aprobación es una comprobación de
  negocio verificable y auditable. Confiar la gobernanza al perfil es confiarla a
  la configuración del cliente. Esto entronca directamente con D11: hoy el perfil
  «protege» la publicación mientras la política que debería protegerla es una
  tautología inoperante (D11, cerrojo 2). Se invierte esa relación.
- **Descarte de 3(b) (`publish` solo en `admin`):** obligaría a todo agente que
  siga el ciclo normal a conectarse en perfil administrativo, lo que le concedería
  también `learning_rollback` y, en el Hito 2, `learning_import` y
  `learning_rebuild_index`. El resultado sería *menos* seguro, no más: para
  publicar con gobernanza habría que conceder capacidades destructivas.
- El mapeo de valores antiguos y el `--profile` deprecated garantizan que ninguna
  instalación de `v0.1.9` se rompa, condición de D8 para `v0.1.10`.

**Consistencia con D11 — advertencia vinculante:** situar `learning_publish` en
`agent` es seguro **si y solo si** la puerta de aprobación de D11 es real. Hoy no
lo es: `AGENTS.md` se puede publicar sin aprobación (D11, cerrojo 2). Por tanto
**D2 y D11 deben entrar en la misma versión**. Está prohibido mover
`learning_publish` a `agent` en una entrega que no incluya ya las políticas
corregidas de D11 y la tool `learning_approve`. Si por cualquier razón hubiera que
separarlas, `learning_publish` permanece en `admin` hasta que D11 esté completo.

### Fecha

2026-07-14

---

## D3 — Evidencia mínima para aprobar

### Contexto

- Niveles de evidencia del dominio: `insufficient weak moderate strong reproduced`
  (plan §Tramo 1 D3). El dominio implementa los cuatro primeros
  (`internal/domain/validation.go:49-52`, `isValidEvidenceLevel`).
- **El umbral ya está implementado y es correcto.**
  `curate.checkEvidenceThreshold` (`internal/curate/curate.go:190-214`) exige dos
  condiciones acumulativas para cualquier decisión de aprobación:
  1. `EvidenceLevel` no puede ser `weak` ni `insufficient`
     (`curate.go:192-196` — «minimum: moderate»);
  2. debe existir **al menos un registro de evidencia persistido**
     (`curate.go:209-212` — `len(evidence) == 0` → `ErrEvidenceMissing`).
- **El problema no es el umbral: es que nadie puede satisfacerlo.**
  `storage.SaveEvidence` (`internal/storage/repo_evidence.go:13`) tiene **cero
  llamadores de producción**; solo lo invocan pruebas. Y el CLI `capture` ni
  siquiera expone `--evidence-level` (`cmd/royo-learn/main.go:459-468`), por lo
  que todo aprendizaje capturado por CLI nace `insufficient`
  (`internal/capture/capture.go:86-88`).

### Opciones consideradas

1. Relajar el umbral para que el flujo funcione (por ejemplo, aceptar `weak`, o
   eliminar la exigencia de registro de evidencia).
2. Conservar el umbral tal cual y construir la entrada pública de evidencia que
   permita satisfacerlo.
3. Sustituir la clasificación declarativa por una taxonomía nueva y extensa de
   evidencia.

### Decisión

**Opción 2. El umbral de `internal/curate/curate.go:190-214` se conserva sin
cambios y se declara contrato.**

Política mínima, ya vigente en código y ahora explícita:

| Nivel | Política |
|-------|----------|
| `insufficient` | **Nunca aprobable.** |
| `weak` | **Nunca aprobable** sin evidencia adicional. |
| `moderate` | Suficiente para conocimiento local de bajo impacto. |
| `strong` | Suficiente para Skill o regla operacional. |
| `reproduced` | Fallo o solución reproducida por prueba o comando. |

Reglas vinculantes:

- **La clasificación declarada no sustituye al registro de evidencia.** Declarar
  `evidence_level: "strong"` sin adjuntar ni un solo registro **no aprueba**. Es
  la condición (2) del umbral y no se negocia.
- Toda interfaz pública debe poder aportar evidencia: `learning_capture` acepta un
  array `evidence[]`, y `learning_add_evidence` / `royo-learn evidence add`
  permiten aportarla después (Recorrido B; ver D11 y D13).
- El CLI `capture` debe exponer `--evidence-level` y las banderas de evidencia del
  Recorrido B (`--evidence-file`, `--collect-git-status`, `--collect-git-diff`).
  Sin ellas, el CLI no puede producir un aprendizaje aprobable **por diseño**.
- Colectores iniciales, y solo esos: evidencia entregada directamente,
  `git status`, `git diff`, y el resultado de un comando explícitamente permitido.

### Justificación

La Opción 1 es la tentación evidente y está expresamente prohibida por §1.1 del
plan: «No adaptar silenciosamente los criterios de aceptación al código
incompleto». El umbral no es el defecto — es la única pieza de esta cadena que
está bien construida. Relajarlo convertiría un producto bloqueado en un producto
que publica sin evidencia, que es peor.

La Opción 3 la prohíbe §1.2 («no rediseñar antes de demostrar necesidad») y §1.3
(«reutilizar antes de reemplazar»): `internal/evidence` ya tiene blob store y
redacción con pruebas (D13).

**Prueba de que el umbral se ha estado eludiendo:** las pruebas de integración
verdes del repositorio alcanzan `approved` únicamente escribiendo la evidencia
directamente en SQLite (`internal/integration/learning_flow_test.go:70`,
`internal/integration/p1_procedure_e2e_test.go:100,137`,
`internal/curate/curate_test.go:110`). Eso es exactamente lo que el criterio de
aceptación del Recorrido B prohíbe: «`captured → needs_evidence →
evidence_attached → approved` **sin manipular SQLite a mano**». Esas pruebas no
demuestran que el flujo funcione; demuestran que no funciona sin ayuda ilícita.

### Fecha

2026-07-14

---

## D4 — Aprobación humana obligatoria

### Contexto

- El plan exige aprobación humana **siempre** para: `AGENTS.md`; conocimiento
  compartido; actualización de una Skill existente; reglas globales; archivos
  fuera del proyecto; comandos de verificación de alto impacto; cambios que
  sustituyen una regla anterior.
- El código **no implementa nada de eso**. Las tres políticas de
  `internal/publish/policy.go:12-14` son:
  - `policyPreferenceTypeRequiresHuman` (`policy.go:31-47`) — la única que puede
    fallar realmente.
  - `policySharedScopeRequiresApproval` (`policy.go:51-68`) — **tautología**.
  - `policyAgentsRuleRequiresApproval` (`policy.go:72-88`) — **tautología**.
- Por qué son tautologías: la política pregunta «¿el destino es `shared` y la
  decisión **no** es `approve_shared_knowledge`?» (`policy.go:55`). Pero el destino
  se **deriva** de la decisión en `curate.deriveDestination`
  (`internal/curate/curate.go:276-320`): el destino solo vale `shared` cuando la
  decisión vale `approve_shared_knowledge` (`curate.go:284-291`). La condición de
  fallo es inalcanzable. Idéntico razonamiento para `AGENTS.md`
  (`policy.go:75` frente a `curate.go:301-308`).
- **Consecuencia verificada: publicar en `AGENTS.md` nunca exige aprobación
  humana.** Un aprendizaje de tipo `procedure` capturado por MCP con
  `proposed_destination: "agents_rule"` (`internal/mcpserver/tools.go:85`) y curado
  con `approve_agents_rule` obtiene `requires_approval: false`
  (`internal/publish/preview.go:138,143`) y se escribe sin ninguna aprobación.

### Opciones consideradas

1. Mantener las políticas como están y confiar la protección al perfil MCP.
2. Reescribir las políticas para que dependan del **destino efectivo y del
   impacto**, no de la decisión de curación que ya determinó ese destino.
3. Exigir aprobación humana para toda publicación, sin excepción.

### Decisión

**Opción 2.** Las políticas se reescriben en función del destino efectivo. El
contrato de aprobación humana es:

| Situación | ¿Aprobación humana? |
|-----------|--------------------|
| Destino `agents_rule` (`AGENTS.md`) | **Siempre** |
| Destino `shared` (conocimiento compartido) | **Siempre** |
| Actualización de una Skill **existente** (`approve_skill_update`) | **Siempre** |
| Regla global o archivo fuera de la raíz del proyecto | **Siempre** |
| Publicación que sustituye una regla anterior | **Siempre** |
| Comando de verificación de alto impacto | **Siempre** |
| Skill **nueva** y aislada (`approve_new_skill`) | Política propia: no requiere aprobación si el destino no existe previamente y queda contenido en `skills/<id>/` |
| Destino `project` (conocimiento local, `.royo-learn/knowledge/`) | No requiere aprobación |

Reglas vinculantes:

- `RequiresHumanApproval` deja de ser una función de la decisión de curación y pasa
  a ser una función del **plan de publicación**: destino, operación, existencia
  previa del archivo y alcance.
- **Una política cuya rama de fallo sea inalcanzable es un defecto, no una
  política.** Toda política de `internal/publish/policy.go` debe ir acompañada de
  una prueba que la haga **fallar** con una entrada real. Sin esa prueba, la
  política no se acepta.
- `policyPreferenceTypeRequiresHuman` se conserva, pero deja de ser un callejón sin
  salida: con D11 existirá una aprobación que la desbloquee.

### Justificación

La Opción 1 es la situación actual y ya está refutada: el perfil no protege nada
(D2), y hoy `AGENTS.md` —el archivo que gobierna el comportamiento de todos los
agentes del proyecto— se puede reescribir sin que ningún humano lo autorice. Es el
riesgo más grave del producto y contradice frontalmente el README.

La Opción 3 haría inviable el caso de uso principal (conocimiento local de bajo
impacto, `moderate`, destino `project`) y convertiría la aprobación en un trámite
que los usuarios aprenderían a saltarse. El plan (D4) distingue explícitamente los
casos sensibles de los que no lo son.

El error de diseño de origen es de dirección causal: las políticas comprueban una
propiedad que el constructor de la curación ya garantizó. Comprobar lo que ya se
garantizó es no comprobar nada.

### Fecha

2026-07-14

---

## D5 — Idempotencia y recurrencia

### Contexto

- El paso E2E `capture-idempotent` (`cmd/royo-learn/e2e.go:109-136`) **no prueba
  idempotencia**: prueba deduplicación por hash de contenido
  (`e2e.go:127-134`). El nombre describe una garantía que el paso no verifica
  (`docs/BASELINE-GAP-REPORT.md:477-481`, FP-9).
- No existe `idempotency_key` en la entrada pública de captura: los flags del CLI
  son `--title --context --observation --lesson --type --scope --project-root
  --json` (`cmd/royo-learn/main.go:459-468`) y la entrada MCP tampoco lo expone
  (`internal/mcpserver/tools.go:73-87`).
- `internal/recurrence` existe (`RecordRecurrence`,
  `internal/recurrence/recurrence.go:18`) pero **no tiene entrada pública**:
  `report occurrence` está en estado `INTERNAL_ONLY`
  (`docs/BASELINE-GAP-REPORT.md:506`).

### Opciones consideradas

1. Tratar toda repetición de contenido como una recurrencia (comportamiento
   implícito hoy).
2. Distinguir explícitamente reintento técnico, evento equivalente y deduplicación
   conservadora mediante `idempotency_key`.

### Decisión

**Opción 2.** Semántica vinculante:

```text
misma idempotency_key       → reintento técnico: no crea aprendizaje ni recurrencia
distinta key + mismo hash   → evento equivalente: reutiliza el aprendizaje y registra recurrencia
sin key + mismo hash        → deduplicación conservadora: no registra recurrencia automática
```

- `idempotency_key` se añade a la entrada pública de captura (CLI y MCP).
- El paso E2E `capture-idempotent` se renombra o se corrige para que pruebe lo que
  su nombre afirma. Un paso que no verifica su propia garantía es un falso
  positivo (FP-9).
- El sistema **nunca** infiere una recurrencia por sí solo a partir de un hash
  coincidente sin `idempotency_key`. Registrar una recurrencia es una afirmación
  de negocio, no una coincidencia de cadenas.

### Justificación

La Opción 1 fabrica métricas falsas: un reintento de red se contabilizaría como
una recurrencia real del problema, y toda la sección de métricas (D9,
`royo-learn metrics`) mediría ruido. La distinción entre «el mismo evento se
reintentó» y «el mismo problema volvió a ocurrir» es exactamente el valor que el
producto promete medir; colapsarla lo destruye.

La regla «sin key + mismo hash → no registra recurrencia» es deliberadamente
conservadora: ante la duda, el sistema **no** afirma. Es coherente con la
separación conceptual del plan («el agente propone; royo-learn valida»).

### Fecha

2026-07-14

---

## D6 — Fuente de verdad de datos

### Contexto

- El store vive en `<root>/.royo-learn/`. SQLite es el almacén operacional; los
  records Markdown son la representación portable; existe un audit log.
- `doctor` es hoy una de las dos únicas operaciones `FUNCTIONAL`
  (`docs/BASELINE-GAP-REPORT.md:510`), y es la pieza natural para detectar
  divergencias.
- `rebuild-index` está documentado (`docs/04-CLI-SPEC.md:225`) pero **no existe**
  (`DOCUMENTED_ONLY`, `docs/BASELINE-GAP-REPORT.md:514`).

### Opciones consideradas

1. Declarar SQLite y Markdown transaccionalmente equivalentes.
2. Jerarquía explícita con roles distintos y reconciliación observable.

### Decisión

**Opción 2.** Roles:

| Artefacto | Rol |
|-----------|-----|
| SQLite | **Fuente operacional.** Es la verdad. |
| Markdown (records) | Representación **portable y auditable**. Derivada. |
| Audit log | Historial **append-only**. No se reescribe. |
| Archivos publicados | **Efecto externo controlado**, reversible por `rollback`. |

Reglas vinculantes:

- **Está prohibido declarar que SQLite y Markdown son transaccionalmente
  equivalentes.** No lo son y no se van a forzar a serlo.
- La materialización a Markdown es **reintentable**; su fallo no invalida la
  operación en SQLite, pero **sí debe quedar registrado y ser detectable**.
- `doctor` detecta divergencias entre SQLite y Markdown. `rebuild-index` las
  repara desde los records. Ambas son parte del contrato.
- **No se introduce outbox** en el Hito 1. Solo se considerará si las pruebas de
  corte del Recorrido D demuestran una ventana irrecuperable que journal +
  compensación + `doctor` no cubren (regla §1.2 del plan).

### Justificación

Declarar una equivalencia transaccional que la implementación no ofrece es la
clase exacta de afirmación que produjo este proyecto de recuperación: contrato que
describe un sistema que no existe. La jerarquía explícita permite decir la verdad
—«SQLite manda, Markdown se deriva y se reconcilia»— y da a `doctor` un trabajo
verificable.

Prohibir el outbox por defecto aplica §1.2: no se introduce una cola sin una
prueba de fallo que la justifique.

### Fecha

2026-07-14

---

## D7 — Aplicación de publicaciones (dry-run)

### Contexto

- `docs/04-CLI-SPEC.md:170-180` documenta `publish` con `--preview-hash
  --approval-id --dry-run=true --apply`, y afirma: «Sin uno de ellos, nunca
  escribe».
- El CLI real solo tiene `--preview-hash` (`cmd/royo-learn/main.go:851-855`).
  **No existe `--apply` ni `--approval-id`** (`docs/BASELINE-GAP-REPORT.md:128`).
- La entrada MCP `publishLearningInput` (`internal/mcpserver/tools.go:115-118`)
  tampoco tiene `approval_id` ni `apply`, pese a que `docs/05-MCP-SPEC.md:162-171`
  los especifica.
- Existe `dry_run_default` en configuración (plan §0, Hallazgo 13).

### Opciones consideradas

1. Considerar que la existencia de un preview equivale a autorización para
   escribir.
2. Dry-run por defecto; la escritura real exige `--apply` explícito.

### Decisión

**Opción 2.**

- `royo-learn publish <id>` **no escribe por defecto**: muestra el preview.
- Escribir exige los tres: `--preview-hash <hash> --approval-id <id> --apply`
  (el `--approval-id` solo cuando el preview indique `requires_approval: true`).
- `--apply` y `--dry-run=false` son equivalentes (`docs/04:180`).
- En MCP: `learning_publish` acepta `apply: bool` (default `false`) y
  `approval_id`, conforme a `docs/05:162-171`.
- **La existencia de un preview no equivale a autorización para modificar
  archivos.** Un preview es una descripción; una aprobación es un acto.

### Justificación

Es el contrato ya escrito (`docs/04:180`) y solo hace falta implementarlo. Por
precedencia §1.1, el contrato gana sobre el código.

El punto sustantivo: sin `--apply`, la única barrera entre un agente automático y
`AGENTS.md` sería la política de aprobación — que hoy es una tautología inoperante
(D4). Dry-run por defecto es la segunda línea de defensa, independiente de la
primera. Las dos deben existir: si una falla, la otra sostiene.

### Fecha

2026-07-14

---

## D8 — Compatibilidad y versión del Hito 1

### Contexto

- `main` y el tag `v0.1.9` apuntan al mismo commit `a00143f`; el diff es vacío,
  verificado por ejecución (`docs/BASELINE-GAP-REPORT.md:27-33`).
- §2 del plan: `v0.1.10` **solo si** todas las interfaces anteriores se conservan
  mediante aliases compatibles (tools MCP, perfiles, comandos CLI). `v0.2.0` si se
  eliminan nombres, se cambian esquemas incompatiblemente o se obliga a migrar.

### Opciones consideradas

1. `v0.1.10` con aliases deprecated (recomendación del plan).
2. `v0.2.0` retirando ya los nombres antiguos.

### Decisión

**Opción 1: el Hito 1 es `v0.1.10`.** El informe de brechas **no aporta ninguna
evidencia en contra**; todas las decisiones D1, D2, D9, D10 y D12 se resuelven con
aliases o con cambios que no rompen ninguna interfaz existente.

Verificación de la condición de §2, decisión por decisión:

| Decisión | ¿Rompe compatibilidad? | Mecanismo |
|----------|------------------------|-----------|
| D1 — nombres MCP | No | 10 aliases deprecated, mismo handler |
| D2 — perfiles y flag | No | `--profile` deprecated; `minimal/standard/full` mapeados; el default sigue siendo el conjunto de tools de `standard` |
| D9 — comandos no documentados | No | Se documentan o se marcan deprecated; ninguno se retira en el Hito 1 |
| D10 — `mcp` | No | `mcp-serve` deprecated, sigue funcionando |
| D12 — `search` | No | Ver D12: hoy `search` **no existe**; nada puede romperse |
| D11 — aprobación | No | Añade tools y flags nuevos; no retira ninguno |

**Aviso de deprecación — obligatorio, nunca silencioso:**

- **CLI:** al invocar un comando o flag deprecated, se emite un aviso en **stderr**
  (nunca en stdout, para no contaminar `--json`), con el nombre canónico y la
  versión de retiro.
- **MCP:** al invocar una tool con nombre deprecated, la respuesta incluye un campo
  `deprecation` con `{ "alias": "...", "canonical": "...", "removed_in": "v0.2.0" }`.

**Calendario de retiro:** los aliases de D1, D2 y D10 se conservan durante toda la
serie `v0.1.x` y **se retiran en `v0.2.0`** (Hito 2), que es el hito que el plan
autoriza a romper compatibilidad («puede retirar aliases si se anuncia», §2).

**Qué sí rompería compatibilidad** y por tanto queda **prohibido en el Hito 1**:
retirar cualquier nombre de tool o comando de `v0.1.9`; cambiar el esquema de una
tool existente de forma incompatible; exigir migración manual del store.

**Lo que el Hito 1 NO anuncia:** que el contrato documental completo está
implementado. `v0.1.10` resuelve los siete problemas del §2 del plan más el
bloqueo de aprobación (D11), y **nada más**. `status`, `review`, `export`,
`import`, `rebuild-index` siguen sin existir y el README debe decirlo.

### Fecha

2026-07-14

---

## D9 — Comandos implementados pero no documentados

### Contexto

Seis comandos existen en el binario y **no tienen entrada en `docs/04-CLI-SPEC.md`**,
que es el contrato del CLI (`docs/BASELINE-GAP-REPORT.md:236-242`). El Tramo 0
matizó el Hallazgo 9 del plan: algunos sí están documentados **fuera** de docs/04.

| Comando | Implementación | Documentación existente |
|---------|----------------|-------------------------|
| `mcp-serve` | `cmd/royo-learn/mcp.go:24` | `docs/FINAL-IMPLEMENTATION-REPORT.md:139` |
| `engram-health` | `cmd/royo-learn/main.go:1053` | **ninguna, en todo el repositorio** |
| `engram-search` | `cmd/royo-learn/main.go:1116` | `docs/FINAL-IMPLEMENTATION-REPORT.md:105` |
| `recurrences` | `cmd/royo-learn/main.go:1194` | ninguna en docs/04 |
| `metrics` | `cmd/royo-learn/main.go:1251` | ninguna en docs/04 |
| `setup` | `cmd/royo-learn/setup.go:45` | `docs/08-GENTLE-AI-CODEX-INTEGRATION.md:138-180` |

### Opciones consideradas

Por comando: **(a)** documentarlo en `docs/04`; **(b)** marcarlo interno/deprecated
con fecha de retiro; **(c)** retirarlo ya.

### Decisión

**Se adopta la recomendación del plan sin desviaciones.** Ningún comando queda en
limbo.

| Comando | Decisión | Destino |
|---------|----------|---------|
| `setup` | **(a) Documentar en `docs/04`** | Entrada propia con subcomandos `install`, `uninstall`, `status` (`setup.go:62,118,159`) **y `upgrade-skills`**, que el Recorrido F añade |
| `recurrences` | **(a) Documentar en `docs/04`** | Entrada propia. Es la lectura de `internal/recurrence`, pieza del contrato (D5) |
| `metrics` | **(a) Documentar en `docs/04`** | Entrada propia. Debe distinguir cero recurrencias / datos insuficientes / recurrencia repetida / recurrencia prevenida (plan §4.4) |
| `mcp-serve` | **(b) Deprecated** | Alias de `mcp` (D10). Retiro en `v0.2.0` |
| `engram-health` | **(b) Deprecated, se pliega bajo `doctor`** | `doctor` incorpora la comprobación de Engram como un check más. Retiro en `v0.2.0` |
| `engram-search` | **(b) Deprecated, se pliega bajo `search`** | `royo-learn search --include-engram` ya está en el contrato (`docs/04-CLI-SPEC.md:120`) y cubre exactamente este caso. Retiro en `v0.2.0` |

Notas:

- `engram-health` es el único comando **sin mención alguna** en toda la
  documentación (`docs/BASELINE-GAP-REPORT.md:124`). Plegarlo bajo `doctor` no
  retira ninguna funcionalidad documentada, porque no había ninguna.
- Los dos comandos `engram-*` siguen ejecutándose durante toda la serie `v0.1.x`
  emitiendo el aviso de deprecación de D8. No se retira nada en el Hito 1.
- `engram-search` depende de que `search` exista. Ver **D12**: si `search` se
  retira del help en lugar de implementarse, esta deprecación no puede ejecutarse.
  Ambas decisiones están acopladas y D12 las resuelve.

### Justificación

La recomendación del plan es correcta y el informe de brechas la respalda:
`setup`, `recurrences` y `metrics` son funcionalidad real y útil que el contrato
simplemente omitió; los `engram-*` son superficie accidental que duplica
capacidades que el contrato ya asigna a `doctor` y a `search`.

Un binario que expone comandos que su contrato ignora es un binario cuyo contrato
no sirve para nada. La regla es: **todo comando ejecutable tiene entrada en
`docs/04`, o tiene fecha de retiro.** No hay tercera categoría.

### Fecha

2026-07-14

---

## D10 — `mcp` frente a `mcp-serve`

### Contexto

- `docs/04-CLI-SPEC.md:229` documenta `royo-learn mcp`.
- El código implementa `mcp-serve` (`cmd/royo-learn/main.go:73` →
  `cmd/royo-learn/mcp.go:24`). No existe `case "mcp"`.
- Divergen también el flag y sus valores (D2).

### Opciones consideradas

1. `mcp-serve` canónico; corregir `docs/04`.
2. `mcp` canónico; `mcp-serve` como alias deprecated.

### Decisión

**Opción 2, adoptando la recomendación del plan.** El informe de brechas no aporta
evidencia en contra.

- Nombre canónico: **`royo-learn mcp`**.
- Alias deprecated: **`mcp-serve`**, funcional durante toda la serie `v0.1.x`, con
  aviso de deprecación en stderr (D8), retirado en `v0.2.0`.
- El comando canónico acepta `--tools read|agent|admin` (D2) y `--project-root`.
- El flag `--profile` y los valores `minimal|standard|full` se aceptan como
  deprecated en **ambos** nombres.

### Justificación

Precedencia §1.1: `docs/04-CLI-SPEC.md` está por encima del comportamiento
accidental del código. `mcp` cumple el contrato; `mcp-serve` no lo cumple y no
aporta nada a cambio. El alias evita romper cualquier configuración de cliente MCP
que hoy invoque `mcp-serve` (que es, de hecho, la que instala el propio `setup`).

### Fecha

2026-07-14

---

## D11 — El bloqueo de aprobación *(defecto central)*

### Contexto

Esta es la decisión más importante del documento. La cadena causal fue reverificada
línea a línea y **difiere de la que registra `docs/BASELINE-GAP-REPORT.md:135-200`**.
Hay **cuatro cerrojos independientes**, no uno.

#### Cerrojo 0 — La puerta de evidencia es insatisfacible *(no detectado en el Tramo 0; es el cerrojo raíz)*

`curate.checkEvidenceThreshold` (`internal/curate/curate.go:190-214`) se ejecuta
ante **cualquier** decisión de aprobación (`curate.go:104-107`) y exige dos cosas:

```go
// internal/curate/curate.go:192-196
if learning.EvidenceLevel == domain.EvidenceWeak || learning.EvidenceLevel == domain.EvidenceInsufficient {
    return domain.NewValidationError(domain.ErrEvidenceMissing, ... "minimum: moderate" ...)
}
// internal/curate/curate.go:209-212
if len(evidence) == 0 {
    return domain.NewValidationError(domain.ErrEvidenceMissing, ... "no evidence records attached" ...)
}
```

- La condición (2) exige **al menos un registro de evidencia persistido**.
- `storage.SaveEvidence` (`internal/storage/repo_evidence.go:13`) tiene **cero
  llamadores de producción**. Solo lo invocan pruebas
  (`internal/integration/learning_flow_test.go:70`,
  `internal/integration/p1_procedure_e2e_test.go:100,137`,
  `internal/curate/curate_test.go:110`,
  `internal/mcpserver/server_test.go:715,972`,
  `internal/publish/skill_area_explicit_test.go:118`).
- Ni `learning_capture` (`internal/mcpserver/tools.go:73-87`) ni
  `royo-learn capture` (`cmd/royo-learn/main.go:459-468`) aceptan evidencia.
  **No existe `evidence add` en ninguna interfaz.**

**Conclusión: ningún aprendizaje capturado por una interfaz pública puede alcanzar
`approved`. Nunca. Ni por CLI ni por MCP.** Todo lo que viene después —preview,
approve, publish, rollback— es inalcanzable. Las pruebas de integración están
verdes porque escriben la evidencia con SQL directo, saltándose las interfaces
públicas.

Agravante: el CLI `capture` **ni siquiera expone `--evidence-level`**
(`cmd/royo-learn/main.go:459-468`), de modo que todo aprendizaje capturado por CLI
nace `insufficient` (`internal/capture/capture.go:86-88`) y falla también la
condición (1).

#### Cerrojo 1 — El CLI no puede expresar 3 de los 5 destinos ni 2 de sus 5 acciones

- El CLI `capture` **no tiene flag de destino**. Búsqueda de `destination` en
  `cmd/royo-learn/*.go` de producción: **cero coincidencias** (solo aparece en
  `cmd/royo-learn/main_test.go`). Por tanto `input.Destination == ""` y se aplica
  el default `DestProject` (`internal/capture/capture.go:78-81`). **Todo
  aprendizaje capturado por CLI propone destino `project`.**
- `curate.deriveDestination` (`internal/curate/curate.go:322-326`) exige que
  `learning.ProposedDestination` coincida con el destino que implica la decisión.
- Consecuencia: desde el CLI, `--action approve_new_skill` y
  `--action approve_skill_update` **siempre fallan** con «decision requires
  proposed destination "skill", got "project"». Dos de las cinco acciones que el
  CLI anuncia son estructuralmente inutilizables.
- Además, `parseCurateAction` (`cmd/royo-learn/main.go:754-769`) no mapea
  `approve_shared_knowledge`, `approve_agents_rule`, `approve_test` ni `merge`, que
  el dominio sí define (`internal/domain/types.go:129-138`) y el servicio de
  curación sí acepta (`internal/curate/curate.go:226-231`).
- El MCP, en cambio, pasa la decisión **en crudo, sin lista blanca**
  (`internal/mcpserver/tools.go:276`: `Decision: domain.CurationDecision(in.Decision)`).
  **CLI y MCP no coinciden en qué es una decisión de curación válida.**

#### Cerrojo 2 — Las políticas que deberían exigir aprobación son tautologías

Ya expuesto en D4, con evidencia. `policySharedScopeRequiresApproval`
(`internal/publish/policy.go:51-68`) y `policyAgentsRuleRequiresApproval`
(`policy.go:72-88`) comprueban una propiedad que `curate.deriveDestination`
(`curate.go:284-291,301-308`) ya garantizó. **Su rama de fallo es inalcanzable.**

**Esto invierte el signo del defecto**: no bloquean nada. `AGENTS.md` y el
conocimiento compartido se publican **sin ninguna aprobación humana**.

#### Cerrojo 3 — Cuando la única política viva se dispara, `publish` queda muerto

- La única política que puede fallar es `policyPreferenceTypeRequiresHuman`
  (`internal/publish/policy.go:31-47`): tipo `preference` **y** destino `shared` o
  `agents_rule`. No tiene escapatoria.
- El guardián que debería impedir que ese estado exista,
  `domain.ValidateLearning` (`internal/domain/validation.go:58-65`), es **código
  muerto**: sus únicos llamadores están en `internal/domain/validation_test.go`.
  El estado **sí es alcanzable** por MCP, que expone `proposed_destination`
  (`internal/mcpserver/tools.go:85`).
- Cuando se dispara: `internal/publish/publish_op.go:62-63` exige `CheckApproval`,
  que falla con `ErrApprovalRequired` (`internal/publish/approval.go:89-91`).
- El único constructor de aprobaciones es `publish.Service.Approve`
  (`internal/publish/approval.go:16`): **cero llamadores en todo el repositorio,
  pruebas incluidas.** No hay comando `approve` ni tool `learning_approve`.

**Resultado: un aprendizaje `preference` con destino compartido es permanentemente
impublicable.** La conclusión del Hallazgo 14 se confirma.

#### Síntesis del defecto

El producto sufre **dos fallos de signo opuesto simultáneamente**:

| | Situación | Efecto |
|---|-----------|--------|
| **Agujero** | Destino `AGENTS.md` o `shared`, tipo ≠ `preference` | Se publica **sin aprobación humana**. La gobernanza que el README promete no existe. |
| **Bloqueo** | Destino `AGENTS.md` o `shared`, tipo `preference` | **Impublicable para siempre.** No hay ruta pública que cree la aprobación. |
| **Raíz** | Cualquier aprendizaje, cualquier interfaz | **No puede llegar a `approved`.** No hay ruta pública que registre evidencia. |

### Opciones consideradas

1. **Exponer `Approve` y nada más.** Añadir `royo-learn approve` y
   `learning_approve` sobre `publish.Service.Approve`.
2. **Reparar los cuatro cerrojos como un contrato único**: entrada pública de
   evidencia, lista blanca canónica compartida de decisiones de curación,
   políticas basadas en destino, y aprobación pública ligada al preview hash.
3. **Relajar las políticas** para que nada requiera aprobación y el flujo «pase».

### Decisión

**Opción 2. Los cuatro cerrojos son un solo contrato y se reparan juntos.**
Reparar solo el cerrojo 3 (Opción 1) dejaría un producto que sigue sin poder
aprobar nada (cerrojo 0) y que sigue escribiendo `AGENTS.md` sin permiso
(cerrojo 2).

#### 11.1 — El contrato de aprobación

Existen **dos aprobaciones distintas** y el contrato debe nombrarlas por separado,
porque hoy se confunden:

| | **Aprobación de curación** | **Aprobación de publicación** |
|---|---|---|
| Qué decide | Que el aprendizaje es válido y a qué destino va | Que un humano autoriza escribir **este** plan concreto |
| Quién la emite | `royo-learn curate --decision <d>` / `learning_curate` | `royo-learn approve` / `learning_approve` |
| Qué produce | Una `Curation` con destino y estado `approved` | Un `Approval` con `approval_id` |
| Requisito previo | Umbral de evidencia (D3) | Un preview válido y no invalidado |
| Estado hoy | Existe, pero es insatisfacible (cerrojo 0) | **No existe entrada pública** (cerrojo 3) |

Contrato de la **aprobación de publicación**:

- **MCP:** `learning_approve`, entrada mínima
  `{learning_id, preview_hash, approved_by, reason, approval_evidence, expires_at}`.
  `approval_evidence` es **obligatorio** en el schema (`docs/05-MCP-SPEC.md:144`).
- **CLI:**

  ```bash
  royo-learn approve <learning-id> \
    --preview-hash <hash> --approved-by <identity> \
    --reason <reason> --approval-evidence <reference>
  ```

  En modo `--json`: sin preguntas interactivas, todos los campos obligatorios,
  respuesta con `approval_id`.
- La aprobación queda **ligada** a: learning ID, preview hash, destinos, actor,
  razón, evidencia de consentimiento, fecha y expiración.
- Se **invalida** cuando: cambia el preview; cambia un destino; cambia el contenido
  previo del archivo; expira; se revoca; cambia la política aplicable.
- `learning_publish` exige el `approval_id` cuando el preview indica
  `requires_approval: true`. **No basta con encontrar «alguna aprobación»
  compatible**: debe ser la aprobación de ese `preview_hash`.

#### 11.2 — CLI y MCP comparten una única lista blanca canónica de decisiones

**Sí, deben compartirla. Es obligatorio.** Se define **un único registro
declarativo en el dominio** (`internal/domain`) con las decisiones de curación
válidas, y **tanto `parseCurateAction` (CLI) como el handler
`curate_learning` (MCP) validan contra ese registro**. Ninguna interfaz define su
propia lista.

Decisiones canónicas (las que ya existen en `internal/domain/types.go:129-138`):

```text
reject  needs_evidence  merge
approve_project_knowledge  approve_shared_knowledge  approve_new_skill
approve_skill_update  approve_agents_rule  approve_test
```

- El CLI unifica su flag: `--decision <d>` (nombre canónico, conforme a
  `docs/04-CLI-SPEC.md:140`), con `--action` como alias deprecated (D8). Los
  valores son los canónicos del dominio; el atajo histórico `approve` se conserva
  como alias deprecated de `approve_project_knowledge`.
- El MCP **deja de pasar la decisión en crudo** (`internal/mcpserver/tools.go:276`)
  y valida contra el mismo registro, devolviendo un error estructurado ante un
  valor desconocido.
- Prueba de contrato permanente: el conjunto de decisiones aceptadas por el CLI, el
  aceptado por el MCP y el definido en el dominio son **idénticos**. Una divergencia
  rompe la build.

**Justificación de la obligatoriedad.** Hoy la asimetría es explotable en ambas
direcciones y no es un detalle cosmético:

- Por **CLI** hay decisiones del dominio que **no se pueden alcanzar**
  (`approve_shared_knowledge`, `approve_agents_rule`, `approve_test`, `merge`).
- Por **MCP** se puede enviar **cualquier cadena** sin lista blanca
  (`tools.go:276`), incluidas las que el CLI prohíbe.
- El resultado es que **la superficie con menos supervisión humana (el agente por
  MCP) tiene más poder que la superficie con más supervisión (el humano por CLI)**.
  Es exactamente la inversión de privilegios que un sistema de gobernanza no puede
  permitirse. Una única lista blanca en el dominio la elimina por construcción.

#### 11.3 — La escapatoria para `preference` + destino `shared`/`AGENTS.md`

**La escapatoria es la aprobación humana explícita, y ninguna otra.**

- `policyPreferenceTypeRequiresHuman` **se conserva** y sigue marcando
  `requires_approval: true` para tipo `preference` con destino `shared` o
  `agents_rule`. **No se elimina y no se relaja.**
- Deja de ser un callejón sin salida porque `learning_approve` existirá: el humano
  aprueba el preview concreto, obtiene un `approval_id`, y `publish --apply
  --approval-id <id>` procede.
- Se **elimina** el guardián duplicado y contradictorio de
  `domain.ValidateLearning` (`internal/domain/validation.go:58-65`), que prohíbe en
  *captura* lo que la política gobierna en *publicación*. Prohibir capturar una
  preferencia compartida es negar el caso de uso; la decisión correcta es
  capturarla y exigir que un humano autorice su publicación. Hoy ese guardián es
  código muerto (cero llamadores de producción), de modo que eliminarlo **no
  cambia ningún comportamiento observable** — solo retira una contradicción del
  contrato.
- El nombre de la política se corrige a `preference_shared_requires_human_approval`
  para que describa lo que hace: exigir aprobación, no prohibir.

#### 11.4 — Las políticas se reescriben en función del destino

Conforme a D4. `RequiresHumanApproval` pasa a depender del plan de publicación
(destino efectivo, operación, existencia previa del archivo, alcance), no de la
decisión de curación que ya determinó ese destino.

**Regla vinculante: toda política debe tener una prueba que la haga fallar con una
entrada real.** Una política cuya rama de fallo es inalcanzable no es una política.
Esta regla, aplicada al código actual, habría detectado el cerrojo 2 en el momento
de escribirlo.

#### 11.5 — Orden de reparación (dependencias reales)

El Recorrido C del plan («aprobación») **no puede ser el primero**: sin el
Recorrido B (evidencia) no hay nada que aprobar. El orden `A → B → C → D → E` del
plan es correcto y la razón es más fuerte de lo que el plan sabía: **el cerrojo 0
está aguas arriba de todo.**

```text
B (evidencia pública)   → desbloquea el cerrojo 0 → permite llegar a `approved`
C (lista blanca única)  → desbloquea el cerrojo 1 → permite alcanzar todo destino
C (políticas por destino) → cierra el cerrojo 2  → AGENTS.md exige aprobación
C (learning_approve)    → desbloquea el cerrojo 3 → lo exigido se puede conceder
```

#### 11.6 — Pruebas obligatorias (todas en rojo primero)

1. Captura sin evidencia → intento de aprobación **bloqueado** con
   `evidence_missing`.
2. Incorporación posterior de evidencia (`evidence add`) → aprobación **exitosa**.
   **Sin escribir en SQLite a mano.**
3. Publicación a `AGENTS.md` sin aprobación → **bloqueada** con
   `approval_required`. *(Esta prueba falla hoy: la publicación tiene éxito.)*
4. Aprobación de otro `preview_hash` → **rechazada**.
5. Aprobación expirada → **rechazada**.
6. Aprobación revocada → **rechazada**.
7. Aprobación válida → publicación **aceptada**.
8. Aprendizaje `preference` + destino `shared` → `requires_approval: true` →
   aprobado por humano → **publicable**.
9. Toda decisión de curación válida por CLI lo es también por MCP, y viceversa
   (prueba de contrato de la lista blanca única).
10. Toda política de `internal/publish/policy.go` tiene al menos una entrada que la
    hace **fallar**.

### Justificación

La Opción 3 (relajar) convertiría un producto bloqueado en un producto peligroso, y
está prohibida por §1.1 del plan.

La Opción 1 (exponer solo `Approve`) es la reparación intuitiva y es insuficiente
por evidencia: aunque `royo-learn approve` existiera hoy mismo, **ningún
aprendizaje podría llegar a `approved`** (cerrojo 0), luego no habría curación,
luego no habría preview, luego no habría nada que aprobar. Y aunque se resolviera,
`AGENTS.md` seguiría escribiéndose sin aprobación (cerrojo 2), porque la política
que debería exigirla no puede fallar. Reparar el cerrojo 3 aislado produce un
sistema que sigue sin funcionar y sigue sin gobernar.

Los cuatro cerrojos comparten una misma causa de fondo: **se comprueba en un punto
lo que otro punto ya garantizó, y no se comprueba en ningún punto lo que nadie
garantiza.** La reparación tiene que ser el contrato completo, o no es reparación.

### Fecha

2026-07-14

---

## D12 — El comando fantasma `search`

### Contexto

- `royo-learn --help` anuncia `search` (`cmd/royo-learn/main.go:109`).
- **No existe `case "search"` en el dispatcher** (`cmd/royo-learn/main.go:57-88`)
  ni ninguna función `runSearch`.
- Ejecutarlo devuelve código `2` con un mensaje de error que habla de
  `version --json` (`cmd/royo-learn/main.go:127-136`):

  ```text
  $ royo-learn search --query test
  {"code":"invalid_argument","message":"invalid arguments: expected \"version --json\"", ...}
  exit=2
  ```

- `docs/04-CLI-SPEC.md:114` lo documenta con `--all-projects --include-engram
  --status --limit --json`.
- **El repositorio ya lo sabía**: `docs/FINAL-IMPLEMENTATION-REPORT.md:105` admite
  que «there is no dedicated `royo-learn search` CLI subcommand», y el help lo
  siguió anunciando de todos modos.
- La capacidad **sí existe** por debajo: FTS5 en `internal/storage`, y el MCP la
  expone como `search_learnings` (`internal/mcpserver/profiles.go:20-30`), presente
  incluso en el perfil `minimal`.

### Opciones consideradas

1. **Retirar `search` del help ahora** (cambio solo de ayuda/documentación).
2. **Implementarlo en el Hito 1**, aunque el plan lo asigna al Tramo 4 §4.1.
3. Dejarlo anunciado y no implementado. *(Estado actual.)*

### Decisión

**Opción 2: se implementa `royo-learn search` en el Hito 1.** Se adelanta
deliberadamente respecto del Tramo 4 §4.1 del plan.

- Conforme a `docs/04-CLI-SPEC.md:114-131`: flags `--all-projects
  --include-engram --status --limit --json`, y la salida **identifica la fuente**
  de cada resultado (`royo_learn` / `engram`).
- Se apoya en el FTS5 ya existente en `internal/storage` y en el mismo servicio que
  usa la tool MCP `learning_search`. **No se reimplementa la búsqueda** (§1.3).
- Prueba de contrato permanente (plan §Tramo 5): **todo comando anunciado en
  `--help` se puede ejecutar; todo comando implementado aparece en el help.** Un
  comando fantasma rompe la build.

**La Opción 3 queda expresamente prohibida:** un comando no puede estar anunciado y
ausente. Es la regla que se incumple hoy.

### Justificación

La Opción 1 (retirarlo del help) es legítima y barata —el prompt del tramo señala
correctamente que sería un cambio solo de ayuda, no una ruptura de contrato de
código—, pero se descarta por tres razones de evidencia:

1. **El coste de implementarlo es bajo y está acotado.** El motor de búsqueda ya
   existe y ya está expuesto por MCP (`profiles.go:20-30`). El trabajo es un
   dispatcher, un parser de flags y un formateador. No es funcionalidad nueva: es
   una entrada pública a una capacidad que ya funciona. §1.2 del plan prohíbe
   rediseñar, no prohíbe conectar lo que existe.
2. **D9 lo necesita.** La deprecación de `engram-search` se pliega bajo
   `royo-learn search --include-engram` (`docs/04:120`). Si `search` no existe,
   `engram-search` no tiene dónde plegarse y D9 no se puede ejecutar. Retirar
   `search` del help obligaría a mantener `engram-search` como comando de primera
   clase, ampliando la superficie accidental en lugar de reducirla.
3. **Asimetría injustificable.** `search` es la única operación del recorrido
   principal que el MCP ofrece incluso en su perfil más restringido (`minimal`,
   3 tools) y que el CLI no ofrece en absoluto. El humano tiene menos capacidad de
   consulta que el agente. Es coherente con la inversión de privilegios que D11
   corrige.

Retirarlo del help sería honesto, pero dejaría el contrato `docs/04:114` sin
cumplir y a `docs/FINAL-IMPLEMENTATION-REPORT.md:105` como testimonio permanente de
una brecha conocida y no cerrada. Implementarlo cierra la brecha y habilita D9.

**Corrección adicional obligatoria:** el mensaje de error de
`cmd/royo-learn/main.go:127-136` menciona `version --json` para *cualquier* comando
desconocido. Debe indicar el comando realmente invocado y listar los válidos.

### Fecha

2026-07-14

---

## D13 — `internal/evidence` es un paquete huérfano

### Contexto

- **Cero importadores** de `agent-royo-learn/internal/evidence` fuera del propio
  paquete (`docs/BASELINE-GAP-REPORT.md:131,204-208`).
- El paquete contiene un blob store content-addressed
  (`internal/evidence/blob.go:22,49,102,122`) y redacción de secretos
  (`internal/evidence/redact.go:30` `Redact`, `:57` `DetectSecrets`), **con pruebas
  propias que pasan**.
- **`evidence.Redact` nunca se ejecuta en ninguna ruta de producción.**
- `cmd/royo-learn/e2e.go:273-275` justifica no comprobar la redacción de secretos
  apuntando a este paquete: «Secret redaction happens in the evidence layer (blob
  store) […] See internal/evidence/redact.go». **La justificación es falsa**: esa
  capa jamás se ejecuta. El paso `security-secret-redaction`
  (`cmd/royo-learn/e2e.go:255-277`) captura la cadena `sk-proj-redactiontest12345`
  y **nunca comprueba que haya sido redactada** (FP-5,
  `docs/BASELINE-GAP-REPORT.md:431-451`).

### Opciones consideradas

1. Reimplementar la evidencia dentro de los handlers de CLI y MCP.
2. Conectar `internal/evidence` a las rutas públicas.
3. Eliminar el paquete por no usarse.

### Decisión

**Opción 2, y se confirma expresamente que la regla §1.3 del plan («reutilizar
antes de reemplazar») sigue vigente y aplica a este paquete.**

Precisiones vinculantes:

1. **El Recorrido B conecta este paquete por PRIMERA VEZ. No lo «reconecta».**
   `internal/evidence` nunca ha estado enchufado a ninguna ruta de producción. El
   plan §1.3 lo lista entre las «piezas internas reutilizables» junto a
   `internal/publish`, `internal/recurrence`, `internal/curate` y
   `internal/doctor`, pero **su situación no es la misma**: los otros cuatro sí se
   invocan desde handlers. Este no. Quien ejecute el Recorrido B debe partir de que
   **no hay ningún punto de integración preexistente que replicar**: hay que
   diseñarlo. Sus pruebas verdes prueban que el paquete funciona **aislado**; no
   prueban absolutamente nada sobre el producto.
2. **La redacción de secretos ocurre ANTES de escribir**, en todas las superficies:
   SQLite, Markdown, audit log, respuestas MCP y logs (plan, Recorrido B). No es un
   filtro de salida: es una condición de escritura.
3. **La aserción de redacción del E2E debe volverse real, no diferirse.** El paso
   `security-secret-redaction` (`cmd/royo-learn/e2e.go:255-277`) debe:
   - capturar un aprendizaje con un secreto **en un registro de evidencia**;
   - leer el registro persistido de vuelta por una interfaz pública;
   - **afirmar que el secreto no aparece** en el aprendizaje, ni en la evidencia,
     ni en el record Markdown, ni en el audit log, ni en la respuesta MCP;
   - fallar si aparece.
   El comentario de `e2e.go:273-275` se elimina. **Está prohibido sustituir la
   aserción por una nota explicativa.** Un paso llamado
   `security-secret-redaction` que no comprueba ninguna redacción es un falso
   positivo de seguridad, y es peor que no tener el paso: afirma una garantía que
   no existe.

**La Opción 3 (eliminar) se descarta** aunque sea la lectura literal de «cero
importadores»: el paquete es exactamente lo que D3 y D11 necesitan, ya tiene el
blob store content-addressed y la redacción probados. Borrarlo obligaría a
reescribirlo en el Recorrido B, violando §1.3.

**La Opción 1 (reimplementar en los handlers) está prohibida** por §1.3: «Exponer y
conectar esas capacidades; no reimplementarlas dentro de handlers CLI o MCP».

### Justificación

El caso de `internal/evidence` es la lección más útil del Tramo 0: **un paquete con
pruebas verdes al 100 % que no aporta ninguna garantía al producto, y una prueba de
seguridad que se ampara en él para no comprobar nada.** Las pruebas de un paquete
huérfano miden la corrección de un artefacto que nadie ejecuta.

Esto no es una anomalía aislada. Es el mismo patrón que `publish.Service.Approve`
(D11, cerrojo 3), que `domain.ValidateLearning` (D11, cerrojo 3) y que
`storage.SaveEvidence` (D11, cerrojo 0): **código correcto, probado y muerto**. La
regla que se extrae, y que el Tramo 5 debe convertir en prueba permanente:

> Ninguna capacidad se considera existente hasta que una interfaz pública la
> invoque y una prueba de negocio observe su efecto.

### Fecha

2026-07-14

---

## D14 — Las instrucciones de `initialize` mienten sobre el perfil

### Contexto

- `buildInstructions` (`internal/mcpserver/server.go:127-158`) recibe el perfil
  **solo para imprimirlo** en la línea `Profile: %s` (`server.go:131`). La lista de
  tools que sigue (`server.go:144-153`) es una **cadena estática con las 10 tools**,
  idéntica en los tres perfiles.
- Inventario real observado (`docs/BASELINE-GAP-REPORT.md:272-283`): `minimal`
  registra **3** tools, `standard` **9**, `full` **10**.
- Consecuencia: un cliente conectado en `minimal` recibe instrucciones que le
  prometen `publish_learning`, `curate_learning`, `get_learning`, `list_learnings`,
  `list_recurrences` y `compute_metrics`. **Ninguna de las seis está registrada en
  ese perfil** (`docs/BASELINE-GAP-REPORT.md:285-289`).
- Las pruebas existentes **consagran el defecto**:
  `internal/mcpserver/conformance_test.go:440` exige que las instrucciones del
  perfil `full` contengan `publish_learning`, y `:446-464` comprueba una lista fija
  de nombres. Ninguna prueba comprueba que las instrucciones **coincidan con lo que
  `tools/list` devuelve**.

### Opciones consideradas

1. Mantener el texto estático y documentar la discrepancia.
2. Derivar la cadena de instrucciones del **registro real de tools del perfil
   activo**.

### Decisión

**Opción 2. La cadena de instrucciones se deriva del registro del perfil activo.
Nunca se codifica a mano.**

Reglas vinculantes:

- `buildInstructions` toma como entrada el **conjunto de tools efectivamente
  registradas** para el perfil, y genera la lista a partir de él. Una tool no
  registrada no puede aparecer en las instrucciones; una tool registrada no puede
  faltar.
- Las instrucciones enumeran **solo nombres canónicos** (D1). Los aliases
  deprecated funcionan pero **no se anuncian**: anunciarlos perpetuaría su uso.
- Se conserva el bloque `Prerequisite:` de onboarding, cuya posición ya está
  protegida por pruebas (`internal/mcpserver/server_test.go:859-875`).
- **Prueba de contrato permanente:** para **cada** perfil, el conjunto de tools
  mencionadas en `instructions` es **exactamente igual** al conjunto que devuelve
  `tools/list`. Divergencia ⇒ build rota. Esta prueba sustituye a la lista fija de
  `conformance_test.go:446-464`, que hoy verifica la cadena literal en lugar del
  registro.

### Justificación

Es la misma clase de defecto que D1 (Skills que citan tools inexistentes) y que
D12 (el help anuncia un comando ausente): **una superficie declarativa mantenida a
mano que se desincroniza de la realidad ejecutable.** El plan ya prescribe el
remedio general en el Tramo 6 —«No copiar la misma lista a mano en cinco
documentos»— y en el Tramo 5 —pruebas de contrato permanentes—. D14 aplica esa
regla a la primera cadena que cualquier cliente MCP lee.

El agravante que decide el caso: las instrucciones son lo primero que un LLM recibe
al conectarse. Un modelo en perfil `minimal` al que se le promete
`publish_learning` intentará invocarla, fallará, y no tendrá forma de saber que la
instrucción era falsa. **Es una superficie que induce activamente al error al
consumidor principal del producto.**

### Fecha

2026-07-14

---

## Coherencia entre decisiones

Verificación explícita exigida por el Tramo 1.

### D1 ↔ D2

Las 15 tools canónicas de D1 están todas asignadas a un perfil en D2, y ningún
perfil contiene una tool que D1 no declare canónica. Los 10 aliases deprecated de
D1 heredan el perfil de su tool canónica. `--profile` y los valores
`minimal/standard/full` sobreviven como deprecated (D2), con el mismo mecanismo de
aviso que los aliases de tools (D8).

### D2 ↔ D11 — el punto de tensión, resuelto

D2 mueve `learning_publish` al perfil `agent` **sobre el argumento explícito de que
la protección real es la aprobación humana, no el perfil**. Ese argumento es válido
**solo si D11 está implementado**: hoy la aprobación no protege nada (cerrojo 2:
`AGENTS.md` se publica sin aprobación) y bloquea todo lo que sí toca (cerrojo 3).

Resolución vinculante, ya incorporada en D2:

> **D2 y D11 entran en la misma versión.** Está prohibido mover `learning_publish`
> al perfil `agent` en una entrega que no incluya ya las políticas por destino
> (D11 §11.4) y la tool `learning_approve` (D11 §11.1). Si hubiera que separarlas,
> `learning_publish` permanece en `admin`.

Sin esta cláusula, D2 y D11 se contradirían: D2 retiraría la única barrera que hoy
existe (el perfil `full`) confiando en una barrera (la aprobación) que aún no
funciona. Con ella, ambas decisiones son consistentes.

### D3 ↔ D11

D3 conserva el umbral de evidencia de `internal/curate/curate.go:190-214` y lo
declara contrato. D11 (cerrojo 0) demuestra que ese umbral es **hoy
insatisfacible** por toda interfaz pública. Ambas se sostienen: el umbral es
correcto y lo que falta es la entrada pública de evidencia. **D3 sin D11 §11.5
produce un producto que no puede aprobar nada.** El Recorrido B es la dependencia
raíz de todo el Hito 1.

### D4 ↔ D11

D4 fija **qué** exige aprobación humana (destino, impacto). D11 §11.4 fija **cómo**
se implementa (políticas basadas en el plan de publicación, no en la decisión de
curación) y añade la regla de que toda política necesita una prueba que la haga
fallar. No hay conflicto: D4 es el contrato, D11 §11.4 es el mecanismo.

### D9 ↔ D12

D9 depreca `engram-search` plegándolo bajo `royo-learn search --include-engram`.
Esa deprecación **exige que `search` exista**, lo que D12 resuelve implementándolo
en el Hito 1. Si D12 hubiera optado por retirar `search` del help, D9 habría tenido
que conservar `engram-search` como comando de primera clase. Las dos decisiones
están acopladas y son consistentes en la forma resuelta.

### D8 ↔ todas

Ninguna decisión de este documento retira un nombre, comando, tool o flag que
exista en `v0.1.9`. Todas las divergencias se resuelven con aliases deprecated y
avisos explícitos. Por tanto la condición de §2 del plan se cumple y **el Hito 1 es
`v0.1.10`**.

---

## Registro de desviaciones respecto del plan

| Decisión | Recomendación del plan | ¿Adoptada? | Motivo de la desviación |
|----------|------------------------|------------|-------------------------|
| D2 | `learning_publish` en `agent` | **Sí** | — (se añade la cláusula de acoplamiento con D11) |
| D8 | Hito 1 = `v0.1.10` | **Sí** | — |
| D9 | Documentar `setup`/`recurrences`/`metrics`; plegar `engram-*` | **Sí** | — |
| D10 | `mcp` canónico, `mcp-serve` alias | **Sí** | — |
| D1 | 11 tools canónicas | **Ampliada a 15** | `learning_add_evidence` y `learning_rollback` los exigen los Recorridos B y E. `learning_list_recurrences` y `learning_compute_metrics` evitan dejar 2 tools registradas sin nombre canónico (limbo, prohibido por D9). |
| D12 | Tramo 4 §4.1 (Hito 2) | **Adelantada al Hito 1** | El motor existe y ya está expuesto por MCP; D9 depende de que `search` exista. Ver D12 §Justificación. |
| — | Hallazgo 14: las políticas de `shared`/`AGENTS.md` marcan `requires_approval` | **Corregido** | Son tautologías inalcanzables. El defecto real es de signo contrario: `AGENTS.md` se publica **sin** aprobación. Ver D11, cerrojo 2. |
| — | Hallazgo 14: el bloqueo empieza en `Approve` | **Corregido** | El cerrojo raíz está una etapa antes: la puerta de evidencia es insatisfacible porque `storage.SaveEvidence` no tiene llamadores de producción. Ver D11, cerrojo 0. |

---

## Puerta de salida del Tramo 1

| # | Ítem | Estado |
|---|------|--------|
| 1 | `docs/CONTRACT-DECISIONS.md` con todas las decisiones resueltas y fechadas | **PASS** — D1 a D14, las 10 del plan más 4 que el Tramo 0 forzó. Cinco secciones cada una (Contexto, Opciones, Decisión, Justificación, Fecha). |
| 2 | Ninguna decisión queda implícita; no se implementa nada antes de esto | **PASS** — `git diff v0.1.9 HEAD -- cmd/ internal/ skills/ go.mod go.sum` vacío. |

**Resultado del Tramo 1: PASS en los 2 ítems. Sin FAIL.**

### Siguiente paso

Tramo 2, Recorrido A (Skills ↔ MCP). Advertencia derivada de D11: el orden de
dependencias del plan (`A → B → C → D → E`) es correcto, pero **el Recorrido B
(evidencia) es la dependencia raíz de todo el Hito 1**, no una etapa más. Sin
entrada pública de evidencia, ningún aprendizaje alcanza `approved` y los
Recorridos C, D y E no tienen sobre qué operar.
