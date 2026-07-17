# Plan de recuperación y cierre de contrato — royo-learn

> Documento operativo para un LLM ejecutor. Redactado el 2026-07-14 tras verificar
> cada afirmación contra el repositorio real en el commit `a00143f` (tag `v0.1.9`).
> Ejecutar los tramos en orden. Cada tramo tiene una puerta de salida explícita:
> no se avanza al siguiente tramo sin cumplirla.

## Mandato

Convertir royo-learn en un sistema cuyo flujo principal funcione de extremo a
extremo y cuya documentación describa exactamente ese comportamiento. Prioridad:

```text
hacer funcionar el recorrido principal
→ demostrarlo con pruebas reales
→ completar las interfaces
→ robustecer la recuperación
→ documentar lo que efectivamente existe
```

Separación conceptual que el producto debe respetar y comunicar:

```text
LLM y Skill: comprenden la conversación y proponen el aprendizaje.
Royo-Learn:  valida, persiste, registra evidencia, controla estados,
             exige aprobaciones, publica, verifica y revierte.
```

---

## 0. Hechos verificados (2026-07-14, HEAD = a00143f = v0.1.9)

Estos hallazgos ya fueron confirmados leyendo el código. El Tramo 0 debe
re-confirmarlos (el repo puede haber cambiado desde esta redacción), pero el
ejecutor no parte de cero: esta tabla es el punto de arranque del gap report.

| # | Hallazgo | Evidencia | Estado |
|---|----------|-----------|--------|
| 1 | `main` y el tag `v0.1.9` apuntan al mismo commit (`a00143f`) | `git describe --tags` → `v0.1.9` | VERIFICADO |
| 2 | Las Skills incluidas invocan tools `learning_*` (`learning_capture`, `learning_search`, `learning_get`, `learning_curate`, `learning_publication_preview`, `learning_approve`, `learning_publish`) | `skills/*/SKILL.md` | VERIFICADO |
| 3 | El servidor MCP registra nombres distintos: `capture_learning`, `search_learnings`, `list_learnings`, `get_learning`, `curate_learning`, `preview_publication`, `publish_learning`, `doctor`, `list_recurrences`, `compute_metrics` | `internal/mcpserver/profiles.go` | VERIFICADO — integración Skills↔MCP rota |
| 4 | No existe ningún tool MCP de aprobación, evidencia posterior, rollback, occurrence ni status | `internal/mcpserver/profiles.go` | VERIFICADO |
| 5 | Perfiles en código: `minimal`/`standard`/`full` (default `standard`); el contrato exige `--tools read\|agent\|admin` | `internal/mcpserver/server.go:51,120`; `docs/04-CLI-SPEC.md:234-242` | VERIFICADO — nota: los perfiles canónicos están en docs/04, no en docs/05 |
| 6 | `publish_learning` solo se expone en el perfil `full`; el default es `standard` → el publish documentado es inalcanzable en una instalación por defecto | `internal/mcpserver/profiles.go:56`, `server.go:51` | VERIFICADO |
| 7 | CLI implementada: `version init doctor capture curate preview publish rollback mcp-serve engram-health engram-search recurrences metrics setup self-update e2e` (y `curate <id> approve\|reject`) | `cmd/royo-learn/main.go:57-87,756-762` | VERIFICADO |
| 8 | CLI documentada pero AUSENTE: `get list search approve occurrence review export import rebuild-index mcp` | `docs/04-CLI-SPEC.md` §§ | VERIFICADO |
| 9 | CLI implementada pero NO documentada: `mcp-serve engram-health engram-search recurrences metrics setup` | comparación docs/04 ↔ dispatcher | VERIFICADO — el plan original no cubría esta dirección |
| 10 | E2E con soft-passes literales: "This is acceptable — we verify the command doesn't crash" y "Soft pass: expected domain guard"; 9 pasos; no ejercita publish, approve, rollback ni occurrence | `cmd/royo-learn/e2e.go:151,157` | VERIFICADO |
| 11 | El instalador de Skills omite cualquier Skill existente ("Existing skills are skipped (never overwritten)") → usuarios con ≤v0.1.9 conservan Skills rotas aunque el repo se corrija | `internal/setup/skill.go:20,49` | VERIFICADO |
| 12 | Existen piezas internas reutilizables: `internal/evidence` (blob store content-addressed, collectors git, redacción de secretos con tests), `internal/publish`, `internal/recurrence`, `internal/curate`, `internal/doctor` | árbol `internal/` | VERIFICADO — la estrategia "reutilizar antes de reemplazar" es viable |
| 13 | `publish` CLI ya exige `--preview-hash`; existe `dry_run_default` en config; el preview expone `requires_approval` | `cmd/royo-learn/main.go:293,825-881` | VERIFICADO — falta approve público y semántica `--apply` |

---

## 1. Reglas transversales (aplican a todos los tramos)

### 1.1 Fuente de verdad — precedencia

1. `docs/14-ACCEPTANCE-CRITERIA.md`
2. `docs/01-PRD.md`
3. `docs/02-ARCHITECTURE.md`
4. `docs/03-DOMAIN-MODEL.md`
5. `docs/04-CLI-SPEC.md`
6. `docs/05-MCP-SPEC.md`
7. `docs/06` a `docs/10`
8. `TASKS.md`
9. `AGENTS.md`
10. README
11. comportamiento accidental del código

No adaptar silenciosamente los criterios de aceptación al código incompleto.
Ante una inconsistencia real del contrato documental: identificarla,
documentarla en `docs/CONTRACT-DECISIONS.md`, resolverla con una decisión
explícita, actualizar contrato y código juntos, y añadir una prueba que impida
su reaparición.

### 1.2 No rediseñar antes de demostrar necesidad

Prohibido introducir sin una prueba de fallo que lo justifique: nueva
arquitectura completa, bus de eventos, cola outbox, otro motor de
almacenamiento, generación automática masiva de código, embeddings, base
vectorial, proveedor LLM, servicio adicional obligatorio, reestructuración
masiva de paquetes.

### 1.3 Reutilizar antes de reemplazar

Los paquetes `internal/evidence`, `internal/publish`, `internal/recurrence`,
`internal/curate` y `internal/doctor` ya existen y tienen tests. Exponer y
conectar esas capacidades; no reimplementarlas dentro de handlers CLI o MCP.

### 1.4 Desarrollo por recorrido vertical + TDD estricto

Cada bloque termina con un recorrido ejecutable:

```text
entrada pública → servicio de aplicación → dominio → almacenamiento → respuesta → prueba de negocio
```

Orden de trabajo dentro de cada recorrido: primero un test rojo que expone la
brecha (commit `test:` identificado como prueba roja), luego la implementación
mínima que lo pone verde, luego refactor. Una función no está terminada porque
exista una estructura o una función interna que nadie puede invocar.

### 1.5 Disciplina de commits

Conventional commits, pequeños y verificables, sin trailers de atribución.
No mezclar en un mismo commit: migraciones, refactor masivo, cambio de
contrato, nueva función, actualización documental, reescritura de pruebas.
Cada commit deja el repo compilable y con pruebas verdes, salvo el commit de
prueba roja claramente identificado. Secuencia orientativa:

```text
test: expose current MCP contract mismatch
fix: add canonical MCP tool names with deprecated aliases
test: require persisted evidence before approval
feat: accept evidence during capture
feat: expose preview approval through CLI and MCP
fix: make publication failure compensating
test: replace permissive CLI e2e
test: add real MCP end-to-end flow
feat: safely upgrade managed skills
docs: align README with verified behavior
```

### 1.6 Idioma de artefactos

Código, identificadores, comentarios, mensajes de error y salidas JSON en
inglés. Los documentos de `docs/` siguen el idioma ya usado por cada documento
que se edita.

---

## 2. Estrategia de versiones

| Hito | Versión | Condición |
|------|---------|-----------|
| Hito 1 — Estabilización funcional | `v0.1.10` | Solo si TODAS las interfaces anteriores se conservan mediante aliases compatibles (tools MCP, perfiles, comandos CLI) |
| Hito 2 — Contrato completo | `v0.2.0` | Completa docs/04, docs/05 y docs/14; puede retirar aliases si se anuncia |

`v0.2.0` se adelanta al Hito 1 únicamente si se eliminan nombres anteriores,
se cambian esquemas incompatiblemente o se obliga a migrar manualmente. La
decisión final se justifica por escrito en `docs/CONTRACT-DECISIONS.md`, nunca
por intuición. Dado que hoy `main == v0.1.9` y la estrategia es de aliases,
la recomendación por defecto es `v0.1.10` para el Hito 1.

El Hito 1 NO anuncia que el contrato documental completo está implementado:
resuelve exactamente los siete problemas siguientes y nada más — Skills↔MCP
incompatibles; sin entrada pública de evidencia; sin aprobación pública;
publicación insegura/incompleta; E2E falso; Skills instaladas no actualizables;
README engañoso.

---

## Tramo 0 — Congelar y caracterizar la base real

No modificar comportamiento durante este tramo.

### 0.1 Rama y registro de partida

```bash
git checkout -b fix/v019-contract-recovery v0.1.9
```

Registrar en `docs/BASELINE-GAP-REPORT.md`: commit de partida, tag `v0.1.9`,
commit actual de `main`, diff entre ambos (hoy es vacío — verificarlo, no
asumirlo), versión de Go, sistema operativo, estado limpio/sucio del repo.

### 0.2 Baseline ejecutable

```bash
go version
git status
git rev-parse HEAD
git describe --tags --always
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/royo-learn
```

Con el binario construido:

```bash
royo-learn version
royo-learn --help
royo-learn e2e --temp
```

Levantar el servidor MCP por stdio y capturar la respuesta real de
`initialize` y `tools/list` en los tres perfiles actuales
(`minimal`, `standard`, `full`).

### 0.3 Informe de brechas — `docs/BASELINE-GAP-REPORT.md`

Matriz obligatoria (pre-poblar con la tabla de Hechos Verificados §0):

| Operación | Documentada | CLI | MCP | Skill | Servicio interno | Prueba real | Estado |
|-----------|------------:|----:|----:|------:|-----------------:|------------:|--------|

Filas mínimas: capture, evidence (add), search, get, list, curate, preview,
approve, publish, rollback, report occurrence, recurrences, metrics, status,
doctor, review, export, import, rebuild-index, setup, skill upgrade,
self-update, mcp/mcp-serve, engram-health, engram-search, e2e.

Estados permitidos: `FUNCTIONAL PARTIAL INTERNAL_ONLY DOCUMENTED_ONLY BROKEN_INTEGRATION MISSING`.

### 0.4 Caracterizar las falsas pruebas

Documentar (sin corregir todavía) cada punto del E2E donde: un fallo
obligatorio se acepta; solo se verifica JSON válido; no se comprueba un cambio
de estado; una prueba de seguridad no ejecuta el ataque; una etapa se omite si
la anterior falla; una ausencia de datos se acepta como éxito.
Puntos ya conocidos: `cmd/royo-learn/e2e.go:151` y `:157`.

### Puerta de salida del Tramo 0

- [ ] commit de baseline registrado
- [ ] `docs/BASELINE-GAP-REPORT.md` completo con la matriz
- [ ] comandos CLI reales inventariados
- [ ] tools MCP reales inventariadas por perfil
- [ ] Skills incompatibles listadas con los tools que citan
- [ ] falsos positivos del E2E documentados con línea exacta

---

## Tramo 1 — Fijar el contrato público (`docs/CONTRACT-DECISIONS.md`)

Antes de tocar handlers, crear `docs/CONTRACT-DECISIONS.md` resolviendo DIEZ
decisiones (las ocho del plan original más dos que la verificación reveló).
Cada decisión lleva: contexto, opciones, decisión, justificación, fecha.

### D1 — Nombres MCP canónicos

Canónicos (según docs/05): `learning_capture, learning_search, learning_get,
learning_list, learning_curate, learning_publication_preview,
learning_approve, learning_publish, learning_report_occurrence,
learning_status, learning_doctor`.

Aliases deprecated que se conservan en el Hito 1: `capture_learning,
search_learnings, get_learning, list_learnings, curate_learning,
preview_publication, publish_learning, doctor`. Cada alias invoca el MISMO
handler y produce exactamente el mismo resultado. Cero duplicación de lógica.

### D2 — Perfiles MCP

Canónicos (según `docs/04-CLI-SPEC.md:234-242`): `read` (search y get),
`agent` (ciclo normal), `admin` (import, rollback y reparación).
Mapeo compatible: `minimal → read`, `standard → agent`, `full → admin`.
Decisión pendiente de fijar por escrito: en qué perfil vive
`learning_publish`. Recomendación: en `agent`, porque docs/04 define agent
como "ciclo normal" y la protección real es la aprobación humana ligada al
preview (D4/D7), no el perfil; dejar en `admin` lo destructivo/administrativo
(rollback, import, rebuild). Registrar la alternativa (publish solo en admin)
y el porqué del descarte.

### D3 — Evidencia mínima

Niveles: `insufficient weak moderate strong reproduced`. La clasificación
declarada no reemplaza un registro real de evidencia. Política mínima:
`insufficient` nunca aprobable; `weak` normalmente requiere más evidencia;
`moderate` conocimiento local de bajo impacto; `strong` Skill o regla
operacional; `reproduced` fallo o solución reproducida por prueba o comando.

### D4 — Aprobación humana obligatoria

Siempre requieren aprobación humana: `AGENTS.md`; conocimiento compartido;
actualización de una Skill existente; reglas globales; archivos fuera del
proyecto; comandos de verificación de alto impacto; cambios que sustituyen una
regla anterior. Una Skill nueva y aislada puede usar otra política, documentada.

### D5 — Idempotencia y recurrencia

```text
misma idempotency_key            → reintento técnico: no crea aprendizaje ni recurrencia
distinta key + mismo hash        → evento equivalente: reutiliza aprendizaje y registra recurrencia
sin key + mismo hash             → deduplicación conservadora: no registra recurrencia automática
```

### D6 — Fuente de verdad de datos

SQLite: fuente operacional. Markdown: representación portable y auditable.
Audit log: historial append-only. Archivos publicados: efecto externo
controlado. NO declarar que SQLite y Markdown son transaccionalmente
equivalentes.

### D7 — Aplicación de publicaciones

Default: dry-run. La escritura real exige `--apply` explícito. La existencia
de un preview no equivale a autorización para modificar archivos.

### D8 — Compatibilidad y versión

Fijar: qué aliases se conservan y hasta cuándo; qué cambios rompen
compatibilidad; si el Hito 1 es `v0.1.10` o `v0.2.0` (recomendado: `v0.1.10`,
ver §2); qué aviso de deprecación recibe el usuario (campo en la respuesta o
warning en stderr, no silencio).

### D9 — Comandos implementados pero no documentados (NUEVA)

`mcp-serve engram-health engram-search recurrences metrics setup` existen en
el binario y no aparecen en docs/04. Decidir por comando: documentarlo en
docs/04 (recomendado para `setup`, `recurrences`, `metrics`) o marcarlo
interno/deprecated con fecha de retiro (`engram-health`, `engram-search`
pueden plegarse bajo `doctor`). Ningún comando puede quedar en el limbo.

### D10 — `mcp` vs `mcp-serve` (NUEVA)

docs/04 documenta `royo-learn mcp`; el código implementa `mcp-serve`.
Decisión recomendada: `mcp` como nombre canónico (cumple contrato) y
`mcp-serve` como alias deprecated del Hito 1.

### Puerta de salida del Tramo 1

- [ ] `docs/CONTRACT-DECISIONS.md` con las 10 decisiones resueltas y fechadas
- [ ] ninguna decisión queda implícita; no se implementa nada antes de esto

---

## Tramo 2 — Hito 1: seis recorridos verticales

Orden y dependencias:

```text
A (Skills↔MCP)  ──→  B (captura+evidencia) ──→ C (aprobación) ──→ D (publicación segura) ──→ E (E2E real)
      │
      └──→  F (upgrade de Skills instaladas)   [independiente de B–D; requiere A]
```

### Recorrido A — Skills y MCP hablan el mismo idioma

**Objetivo**: una Skill incluida puede llamar realmente al servidor MCP.

**Cambios**: en el registro MCP (`internal/mcpserver/profiles.go` + registro),
añadir los nombres `learning_*` canónicos; conservar los aliases antiguos
apuntando al mismo handler; agregar anotaciones read/write/destructive por
tool; aplicar perfiles `read/agent/admin` con mapeo desde
`minimal/standard/full`. Actualizar las Skills incluidas para usar SOLO
nombres canónicos. No corregir la Skill hacia los nombres accidentales del
código: corregir el código hacia el contrato.

**Pruebas obligatorias** (primero en rojo):
1. Test que recorre `skills/**/SKILL.md`, extrae los nombres de tools MCP,
   consulta el registro real, y verifica que cada nombre existe, pertenece al
   perfil declarado, y NO es un alias deprecated.
2. Test de triple coincidencia: tools documentadas (docs/05) ↔ tools
   registradas ↔ tools usadas por Skills.

**Criterio de aceptación**: una Skill nunca puede citar una tool inexistente.

### Recorrido B — Captura con evidencia real

**Objetivo**: un aprendizaje puede llegar legítimamente a curación.

**Estrategia**: reutilizar `internal/evidence` (blob store, collectors git,
redacción). No diseñar una taxonomía nueva y extensa.

Extender la captura para aceptar:

```json
{
  "title": "...", "context": "...", "observation": "...",
  "reusable_lesson": "...", "confidence": "high",
  "evidence_level": "strong", "idempotency_key": "...",
  "evidence": [
    { "type": "test_result", "summary": "...", "source": "...", "content": "..." }
  ]
}
```

CLI mínima: `--file --stdin --idempotency-key --collect-git-status
--collect-git-diff --evidence-file --json`. Collectors iniciales: evidencia
entregada directamente, `git status`, `git diff`, resultado de un comando
explícitamente permitido. NO exponer veinte tipos de collector.

**Persistencia**: aprendizaje, evidencia y evento de auditoría en una
operación coherente. La redacción de secretos ocurre ANTES de escribir en
SQLite, Markdown, audit log, respuestas MCP y logs.

**Evidencia posterior**: el estado `needs_evidence` exige poder agregar
evidencia después. Añadir al contrato `learning_add_evidence` (MCP) y
`royo-learn evidence add` (CLI). Documentar PRIMERO en docs/03, docs/04,
docs/05 y docs/14; después implementar. No esconder una operación nueva solo
en el código.

**Pruebas**: captura sin evidencia → intento de aprobación bloqueado →
incorporación posterior de evidencia → aprobación exitosa; secreto redactado;
reintento con la misma idempotency key sin duplicar evidencia.

**Criterio de aceptación**: `captured → needs_evidence → evidence_attached →
approved` sin manipular SQLite a mano.

### Recorrido C — Aprobación pública y verificable

**Objetivo**: exponer la aprobación que ya existe internamente (el CLI ya
tiene `curate <id> approve`, pero NO existe la aprobación de publicación
ligada al preview).

**MCP**: `learning_approve` con entrada mínima
`{learning_id, preview_hash, approved_by, reason, approval_evidence, expires_at}`.

**CLI**:

```bash
royo-learn approve <learning-id> \
  --preview-hash <hash> --approved-by <identity> \
  --reason <reason> --approval-evidence <reference>
```

En modo JSON: sin preguntas interactivas, todos los campos obligatorios,
respuesta con `approval_id`.

**Reglas**: la aprobación queda vinculada a learning ID, preview hash,
destinos, actor, razón, evidencia de consentimiento, fecha y expiración.
Se invalida cuando: cambia el preview; cambia un destino; cambia el contenido
previo del archivo; expira; se revoca; cambia la política relevante.
`learning_publish` exige el `approval_id` cuando el preview indica
`requires_approval: true`. No basta con encontrar "alguna aprobación" compatible.

**Pruebas**: publicación sensible sin aprobación → bloqueada; aprobación de
otro hash → rechazada; expirada → rechazada; válida → aceptada; reutilizada
para otro preview → rechazada.

### Recorrido D — Publicación segura y estados verdaderos

**Objetivo**: el sistema nunca informa éxito parcial ni deja archivos
modificados tras un fallo tardío.

**Preview persistido** con: learning ID, proyecto, destinos, operación por
destino, hash anterior, hash posterior, diff, verificaciones, política
aplicada, necesidad de aprobación, preview hash. El preview hash depende del
plan completo, no solo del texto combinado del diff.

**Flujo de aplicación**:

```text
validar aprendizaje → validar preview → validar aprobación
→ validar hashes actuales → adquirir lock → crear backups
→ registrar intento → escribir → verificar → persistir resultado
→ marcar published → cerrar journal
```

**Fallos**: si hay error después de modificar archivos: intentar rollback,
registrar el resultado, NO marcar published, devolver error estructurado,
dejar instrucción de recuperación si el rollback falla. `published` solo
después de que todas las escrituras terminen, las verificaciones pasen, el
registro de publicación persista y la auditoría quede registrada.

**Dry-run**: `royo-learn publish <id>` no escribe por defecto; muestra el
preview. Escribir exige `--preview-hash <hash> --approval-id <id> --apply`.
(El CLI ya exige `--preview-hash`; añadir `--approval-id` y `--apply`.)

**Pruebas de fallos** (inyección): escritura del primer archivo, del segundo,
verificación, journal, actualización final de SQLite, rollback, y destino
modificado después del preview. NO introducir outbox automáticamente: primero
comprobar si journal previo + compensación + `doctor` satisfacen las
garantías; outbox solo si las pruebas demuestran una ventana irrecuperable.

### Recorrido E — E2E que demuestre el producto

**Objetivo**: reemplazar el E2E permisivo por pruebas que fallen ante
cualquier ruptura real.

**Escenario CLI** (19 pasos, todos obligatorios): crear repo Git temporal →
`init` → `doctor` → capturar con evidencia → `get` → `search` → curar →
preview → publicar sin aprobación (verificar `approval_required`) → aprobar →
publicar con `--apply` → comprobar el archivo → comprobar estado `published`
→ registrar ocurrencia → comprobar métricas → rollback → comprobar
restauración byte a byte → `doctor` final.

Prohibido: "failure is acceptable", saltar etapas, JSON vacío como evidencia,
aceptar cualquier exit code, PASS sin efecto de negocio.

**Escenario MCP**: cliente MCP real por stdio ejecutando `initialize`,
`tools/list`, `learning_capture`, `learning_get`, `learning_search`,
`learning_curate`, `learning_publication_preview`, `learning_publish` (falla
por aprobación), `learning_approve`, `learning_publish` (éxito),
`learning_report_occurrence`, `learning_status`, `learning_rollback`.
Comprobar schemas, nombres, perfiles, anotaciones, respuestas, códigos de
error, cambios de estado y archivos resultantes.

**Dos políticas separadas**: (1) publicación de bajo impacto; (2) publicación
sensible que exige aprobación humana. No usar un único caso que evite
accidentalmente la política de aprobación.

### Recorrido F — Actualización segura de Skills instaladas

**Problema verificado**: `internal/setup/skill.go` omite cualquier Skill
existente; corregir las Skills del repo no repara las copias instaladas por
usuarios de versiones anteriores.

**Implementación mínima**: manifiesto por Skill instalada:

```json
{ "name": "capture-learning", "version": "3.0.0",
  "source_sha256": "...", "installed_sha256": "...", "managed_by": "royo-learn" }
```

Comandos: `royo-learn setup status`, `royo-learn setup upgrade-skills
--dry-run`, `royo-learn setup upgrade-skills --apply`.

**Política**: hash intacto → backup, actualizar, registrar versión.
Modificada por el usuario → no sobrescribir, crear versión candidata, mostrar
diff, registrar conflicto. No instalada por royo-learn → no tocarla.

**Pruebas**: instalación nueva; upgrade sin modificaciones; upgrade con
personalización; backup; dry-run; repetición idempotente; recuperación tras
fallo.

**Criterio de aceptación**: actualizar el binario ofrece una ruta segura para
actualizar las Skills incompatibles ya instaladas.

---

## Tramo 3 — Puerta de publicación del Hito 1

Publicar `v0.1.10` solo cuando se cumpla TODO simultáneamente:

```text
[ ] Skills y MCP coinciden (test de contrato en verde)
[ ] captura acepta evidencia
[ ] evidence add funciona (CLI y MCP)
[ ] curación puede aprobar realmente por interfaces públicas
[ ] aprobación pública funciona y queda ligada al preview hash
[ ] publish exige approval_id cuando requires_approval
[ ] publish requiere --apply para escribir
[ ] rollback compensa fallos posteriores a escritura
[ ] E2E CLI completo pasa (19 pasos, sin soft-passes)
[ ] E2E MCP completo pasa
[ ] Skills instaladas pueden actualizarse (Recorrido F)
[ ] README describe únicamente lo demostrado
```

Verificación final:

```bash
go fmt ./...
go mod tidy
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/royo-learn
```

No publicar si una prueba crítica contiene excepciones toleradas o soft-passes.

---

## Tramo 4 — Hito 2: completar todo el contrato

### 4.1 CLI completa

Implementar todos los comandos de docs/04 más las decisiones D9/D10:
`version init doctor capture evidence get list search curate preview approve
publish rollback occurrence recurrences metrics status review export import
rebuild-index mcp e2e setup self-update`.

Help y dispatcher derivados o validados contra un único registro declarativo
(una tabla compartida + pruebas contractuales basta; no introducir generación
compleja de código). Prueba contractual: todo comando de `--help` existe; todo
comando implementado aparece en help; todo comando acepta `--help`; todo
comando documentado está implementado; no hay comandos fantasma.

### 4.2 MCP completo

Implementar y probar: `learning_capture learning_add_evidence learning_search
learning_get learning_list learning_curate learning_publication_preview
learning_approve learning_publish learning_report_occurrence learning_status
learning_doctor learning_rollback`. Añadir `learning_export learning_import
learning_rebuild_index learning_review` solo si tienen utilidad real por MCP.
Ninguna operación destructiva en `read` ni en `agent`.

### 4.3 Errores y exit codes

Modelo común de errores (mismo dominio traducido por handlers CLI y MCP; nunca
interpretar errores por string):

```json
{ "error": { "code": "approval_required",
  "message": "The publication requires human approval.",
  "recoverable": true, "details": {},
  "next_action": "Approve the current preview and retry with its approval ID." } }
```

Implementar los exit codes contractuales (docs/17) y probar cada clase.

### 4.4 Recurrencias e idempotencia

Conectar `internal/recurrence` con `learning_report_occurrence` y
`royo-learn occurrence`. Registrar: learning ID, fingerprint, evento, fecha,
resultado, si se recuperó el aprendizaje, si se activó la Skill, evidencia,
actor. Aplicar la semántica D5. Las métricas distinguen: cero recurrencias,
datos insuficientes, recurrencia repetida, recurrencia prevenida.

### 4.5 Búsqueda y relaciones

FTS5 (y Engram opcional) para candidatos similares en la captura; el sistema
NUNCA decide autónomamente que dos aprendizajes son equivalentes. Relaciones
explícitas: `duplicate_of extends supersedes contradicts narrows related`.
El agente propone; la curación confirma. Sin embeddings en esta etapa.

### 4.6 Importación, exportación y reconstrucción

`export import rebuild-index review` con: formatos versionados, validación
antes de importar, dry-run, backup, detección de conflicto, sin sobrescritura
silenciosa, reconstrucción determinista, prueba round-trip
(exportar → borrar base temporal → importar/reconstruir → comparar
aprendizajes, evidencias, relaciones y estados).

### 4.7 Coherencia SQLite–Markdown

Primero: audit dentro de la transacción principal; estado anterior capturado
antes de mutar; materialización reintentable; hashes; `doctor` detecta
divergencias; `rebuild-index` las repara. Outbox solo si las pruebas de corte
demuestran que la materialización no puede recuperarse de otra forma.

### 4.8 Migraciones

Inspeccionar el esquema antes de crear tablas. Campos probables: idempotency
key, aprobación, evidencia de aprobación, versión de política, estado de
publicación, resultado de ocurrencia, manifiesto de Skills, materialización
pendiente (si se demuestra necesaria). Migraciones versionadas, idempotentes,
respaldadas, probadas desde una base v0.1.9 real, reversibles cuando sea
razonable, e incapaces de autoaprobar registros antiguos.

---

## Tramo 5 — Pruebas de contrato permanentes y CI

### Pruebas de contrato (quedan para siempre en el repo)

- Skills ↔ MCP: toda tool citada por una Skill existe y no es alias deprecated.
- Documentación ↔ MCP: toda tool obligatoria registrada con schema correcto.
- Help ↔ CLI: todo comando anunciado se puede ejecutar.
- README ↔ binario: los comandos del Quick Start se ejecutan en CI.
- Perfil ↔ permisos: nada destructivo en `read` ni accidentalmente en `agent`.
- Versiones: sin versiones escritas a mano en múltiples archivos.
- JSON: snapshots versionados de learning, evidence, preview, approval,
  publication, occurrence, error y status.

### CI

Matriz: Linux, Windows, macOS × Go mínimo declarado en `go.mod` y Go estable
más reciente compatible. En Linux, `go test -race ./...` obligatorio.
Incluir: unit, integration, CLI E2E, MCP E2E, contract tests, upgrade desde
v0.1.9, Skill upgrade, rollback, fault injection, cross-build, instalación
limpia, self-update smoke test.

---

## Tramo 6 — Documentación final e informes

Actualizar el README solo después de cerrar cada hito. Debe explicar:

- **Qué hace el LLM**: interpreta, identifica, estructura, propone, recomienda
  relaciones.
- **Qué hace Royo-Learn**: valida, persiste, registra evidencia, garantiza
  estados, exige aprobación, publica, verifica, audita, revierte, mide
  recurrencias.
- **Qué NO hace**: no comprende conversaciones por sí solo; no decide
  automáticamente que una observación es una regla; no aprueba en nombre del
  usuario; no sustituye Engram; no necesita proveedor LLM interno; no usa
  embeddings; no garantiza funciones no demostradas.

Generar o validar automáticamente: `docs/generated/CLI_REFERENCE.md`,
`docs/generated/MCP_REFERENCE.md`, `docs/generated/ERROR_REFERENCE.md`,
`docs/generated/PROFILES.md`. No copiar la misma lista a mano en cinco
documentos. Los README traducidos (`docs/README.*.md`) se actualizan o se
marcan como desactualizados; no se dejan contradiciendo al README principal.

### Entregables documentales del proyecto

```text
docs/BASELINE-GAP-REPORT.md        (Tramo 0)
docs/CONTRACT-DECISIONS.md         (Tramo 1)
docs/IMPLEMENTATION-LOG.md         (continuo, por recorrido)
docs/FINAL-IMPLEMENTATION-REPORT.md (cierre)
```

El informe final incluye: commit inicial y final, versión, decisiones
contractuales, aliases conservados, migraciones, recorridos E2E, pruebas MCP,
CLI, de fallo y de rollback, actualización de Skills, compatibilidad v0.1.9,
comandos ejecutados con resultados, riesgos residuales y funciones fuera de
alcance. Tabla obligatoria:

| Requisito | Estado | Prueba | Evidencia |
|-----------|--------|--------|-----------|

Estados permitidos: `PASS FAIL NOT_APPLICABLE`. Prohibidos: `PARTIAL
MOSTLY_DONE EXPECTED_FAILURE SOFT_PASS`. Con un `FAIL`, el proyecto no puede
declararse terminado.

---

## Definición de terminado (global)

Royo-Learn está reparado únicamente cuando: una Skill puede invocar todas las
tools que menciona; un aprendizaje puede capturarse con evidencia y agregarla
después; la curación funciona por interfaces públicas; la aprobación humana
está expuesta y ligada al preview; una publicación sensible se bloquea sin
aprobación; publicar exige aplicación explícita; un fallo posterior a
escritura no deja un falso `published`; rollback restaura el contenido exacto;
una recurrencia puede registrarse y medirse; la idempotencia no crea
recurrencias falsas; CLI, MCP, Skills y documentación coinciden; las Skills
antiguas se actualizan sin destruir personalizaciones; el E2E prueba efectos
de negocio; una base v0.1.9 migra sin perder datos; Windows, Linux y macOS
pasan sus pruebas; y el README describe exactamente lo demostrado.

Solo entonces es válida la frase final del producto:

> Royo-Learn es un motor local de aprendizaje operacional para agentes. El
> agente identifica y estructura la experiencia; Royo-Learn conserva
> evidencia, controla su gobernanza y convierte aprendizajes aprobados en
> cambios verificables, auditables y reversibles.
