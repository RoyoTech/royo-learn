# Prompts de ejecución por sesión — Plan de recuperación de contrato

Este archivo contiene los prompts para ejecutar `docs/PLAN-recuperacion-contrato.md`
con un LLM, **un tramo por sesión**. Cada prompt es autocontenido: asume una
sesión nueva sin memoria de las anteriores.

## Cómo usarlo (instrucciones para el humano)

1. Abrir una sesión nueva del LLM ejecutor por cada prompt.
2. Pegar el **preámbulo común** seguido del **prompt de la sesión** que toca.
3. Al terminar la sesión, auditar la puerta de salida con el checklist del
   final de este archivo ANTES de lanzar la siguiente sesión.
4. Si una sesión muere a medias, usar el **prompt de reanudación**.
5. No lanzar dos sesiones en paralelo: cada tramo depende del anterior.

Orden de sesiones:

| Sesión | Tramo | Contenido |
|--------|-------|-----------|
| 1 | Tramo 0 | Baseline y gap report |
| 2 | Tramo 1 | Decisiones de contrato |
| 3 | Tramo 2 — Recorrido A | Skills ↔ MCP |
| 4 | Tramo 2 — Recorrido B | Captura con evidencia |
| 5 | Tramo 2 — Recorrido C | Aprobación pública |
| 6 | Tramo 2 — Recorrido D | Publicación segura |
| 7 | Tramo 2 — Recorrido E | E2E real (CLI y MCP) |
| 8 | Tramo 2 — Recorrido F | Upgrade de Skills instaladas |
| 9 | Tramo 3 | Puerta de publicación del Hito 1 (`v0.1.10`) |
| 10 | Tramo 4 (parte 1) | CLI completa + MCP completo + errores (§4.1–4.3) |
| 11 | Tramo 4 (parte 2) | Recurrencias + búsqueda/relaciones (§4.4–4.5) |
| 12 | Tramo 4 (parte 3) | Export/import + coherencia + migraciones (§4.6–4.8) |
| 13 | Tramo 5 | Pruebas de contrato permanentes + CI |
| 14 | Tramo 6 | Documentación final e informes (`v0.2.0`) |

---

## Preámbulo común (pegar al inicio de TODAS las sesiones)

```text
Trabajás sobre el repositorio RoyoTech/royo-learn.

Tu fuente de instrucciones es docs/PLAN-recuperacion-contrato.md.
Antes de tocar nada:

1. Leé completo docs/PLAN-recuperacion-contrato.md, en especial las
   reglas transversales del §1 y el tramo que se te asigna abajo.
2. Leé docs/IMPLEMENTATION-LOG.md si existe: ahí está lo que hicieron
   las sesiones anteriores. Si no existe y tu tramo no es el Tramo 0,
   detenete y reportalo: falta trabajo previo.
3. Verificá que la puerta de salida del tramo ANTERIOR está cumplida
   (está definida en el plan). Si no lo está, detenete y reportá
   exactamente qué falta. No intentes completar el tramo anterior.

Reglas no negociables de esta sesión:

- Ejecutás ÚNICAMENTE el tramo asignado abajo. No empieces el siguiente
  aunque te sobre tiempo o te parezca obvio.
- TDD estricto: cada brecha se expone primero con un test rojo
  (commit "test: ..."), después la implementación mínima que lo pone
  verde, después refactor.
- Conventional commits, pequeños, sin atribución de IA. Cada commit deja
  el repo compilable con pruebas verdes, salvo el test rojo identificado.
- Prohibido: rediseñar arquitectura; introducir outbox, embeddings,
  bus de eventos o dependencias nuevas; soft-passes o excepciones
  toleradas en tests; adaptar los criterios de aceptación al código.
- Si encontrás una contradicción real entre el plan, los docs y el
  código, documentala en docs/CONTRACT-DECISIONS.md con una decisión
  explícita antes de seguir. Nunca la resuelvas en silencio.
- Trabajá en la rama fix/v019-contract-recovery (creala desde v0.1.9
  solo si sos la sesión 1; si no existe y no sos la sesión 1, detenete).

Al terminar, hacé dos cosas:

1. Agregá una entrada a docs/IMPLEMENTATION-LOG.md con: fecha, tramo,
   commits creados (hash + mensaje), comandos ejecutados con resultados,
   decisiones tomadas, y estado de cada ítem de la puerta de salida.
2. Reportá en el chat: qué hiciste, qué quedó pendiente, y la puerta de
   salida ítem por ítem con PASS o FAIL. Sin PARTIAL, sin MOSTLY_DONE.
```

---

## Sesión 1 — Tramo 0: baseline y gap report

```text
Tu tramo asignado es el TRAMO 0 del plan (§ "Tramo 0 — Congelar y
caracterizar la base real").

Tareas, en orden:
1. Creá la rama fix/v019-contract-recovery desde el tag v0.1.9.
2. Ejecutá el baseline completo del §0.2 y registrá cada salida.
3. Creá docs/BASELINE-GAP-REPORT.md con la matriz del §0.3,
   pre-poblada con la tabla "Hechos verificados" del §0 del plan,
   re-confirmando cada hallazgo contra el código actual.
4. Documentá los falsos positivos del E2E (§0.4) con archivo y línea.
   NO corrijas nada todavía: este tramo no modifica comportamiento.
5. Creá docs/IMPLEMENTATION-LOG.md con la primera entrada.

Restricción dura: en este tramo no se modifica ningún archivo de código.
Solo se crean documentos y se ejecutan comandos de diagnóstico.
```

## Sesión 2 — Tramo 1: decisiones de contrato

```text
Tu tramo asignado es el TRAMO 1 del plan (§ "Tramo 1 — Fijar el
contrato público").

Tareas:
1. Creá docs/CONTRACT-DECISIONS.md resolviendo las DIEZ decisiones
   D1–D10 tal como las define el plan. Para cada una: contexto,
   opciones consideradas, decisión, justificación y fecha.
2. Donde el plan da una recomendación (D2 perfil de publish, D8 versión
   v0.1.10, D9 destino de cada comando no documentado, D10 mcp
   canónico), adoptala salvo que docs/BASELINE-GAP-REPORT.md aporte
   evidencia en contra; en ese caso documentá por qué te apartás.
3. No implementes NADA en código en esta sesión. El entregable es
   exclusivamente el documento de decisiones.
```

## Sesión 3 — Recorrido A: Skills y MCP hablan el mismo idioma

```text
Tu tramo asignado es el RECORRIDO A del Tramo 2 del plan.

Prerequisito: docs/CONTRACT-DECISIONS.md existe con D1 y D2 resueltas.

Tareas, en orden TDD:
1. Test rojo: prueba que recorre skills/**/SKILL.md, extrae los nombres
   de tools MCP citados, y verifica contra el registro real que cada
   nombre existe, pertenece al perfil declarado y no es alias
   deprecated. Hoy debe fallar.
2. Test rojo: triple coincidencia docs/05 ↔ registro ↔ Skills.
3. Implementación: nombres learning_* canónicos en el registro MCP,
   aliases antiguos apuntando al MISMO handler, anotaciones
   read/write/destructive, perfiles read/agent/admin con mapeo desde
   minimal/standard/full.
4. Actualizá las Skills incluidas para usar solo nombres canónicos.
5. Ambos tests en verde. go test ./... y go vet ./... limpios.

Criterio de salida: una Skill nunca puede citar una tool inexistente.
```

## Sesión 4 — Recorrido B: captura con evidencia real

```text
Tu tramo asignado es el RECORRIDO B del Tramo 2 del plan.

Prerequisito: Recorrido A cerrado (verificalo en IMPLEMENTATION-LOG).

Seguí el plan al pie de la letra: reutilizá internal/evidence (no
reimplementes); primero actualizá docs/03, docs/04, docs/05 y docs/14
para incorporar learning_add_evidence y royo-learn evidence add al
contrato; después implementá con TDD el flujo completo:
captura con evidencia embebida → needs_evidence → evidence add
posterior → aprobación. Redacción de secretos ANTES de cualquier
persistencia. Idempotency key sin duplicación de evidencia.

Criterio de salida: captured → needs_evidence → evidence_attached →
approved sin tocar SQLite a mano, demostrado por tests.
```

## Sesión 5 — Recorrido C: aprobación pública y verificable

```text
Tu tramo asignado es el RECORRIDO C del Tramo 2 del plan.

Prerequisito: Recorrido B cerrado.

Implementá con TDD la aprobación de publicación ligada al preview:
tool MCP learning_approve y comando royo-learn approve con los campos
exactos del plan. La aprobación queda vinculada a learning ID, preview
hash, destinos, actor, razón, evidencia de consentimiento, fecha y
expiración, y se invalida en los seis casos que lista el plan.
learning_publish exige approval_id cuando requires_approval es true.

Tests obligatorios (los cinco del plan): sin aprobación → bloqueada;
hash distinto → rechazada; expirada → rechazada; válida → aceptada;
reutilizada para otro preview → rechazada.
```

## Sesión 6 — Recorrido D: publicación segura y estados verdaderos

```text
Tu tramo asignado es el RECORRIDO D del Tramo 2 del plan.

Prerequisito: Recorrido C cerrado.

Implementá con TDD: preview persistido con el contenido completo que
exige el plan (hash que depende del plan completo, no solo del diff);
flujo de aplicación en el orden exacto del plan (validaciones → lock →
backups → journal → escritura → verificación → published); dry-run por
defecto y escritura solo con --preview-hash + --approval-id + --apply;
compensación ante fallos post-escritura sin marcar published.

Pruebas de inyección de fallos: las siete del plan (primer archivo,
segundo archivo, verificación, journal, SQLite final, rollback, destino
modificado tras el preview).

Restricción: NO introduzcas outbox. Si creés que el journal +
compensación no alcanza, documentá la prueba de fallo que lo demuestra
en docs/CONTRACT-DECISIONS.md y detenete para revisión humana.
```

## Sesión 7 — Recorrido E: E2E que demuestre el producto

```text
Tu tramo asignado es el RECORRIDO E del Tramo 2 del plan.

Prerequisito: Recorridos A–D cerrados.

Reemplazá el E2E permisivo actual (cmd/royo-learn/e2e.go) por:
1. Escenario CLI de 19 pasos exactos del plan, sin soft-passes, sin
   "failure is acceptable", sin aceptar exit codes arbitrarios, con
   verificación de efectos de negocio (archivo escrito, estado
   published, restauración byte a byte).
2. Escenario MCP con cliente real por stdio ejecutando la secuencia
   completa del plan, verificando schemas, perfiles, anotaciones,
   códigos de error y archivos resultantes.
3. Dos políticas separadas: publicación de bajo impacto y publicación
   sensible con aprobación humana.

Antes de reescribir, confirmá contra docs/BASELINE-GAP-REPORT.md que
cada falso positivo documentado en el Tramo 0 queda eliminado.
```

## Sesión 8 — Recorrido F: actualización segura de Skills instaladas

```text
Tu tramo asignado es el RECORRIDO F del Tramo 2 del plan.

Prerequisito: Recorrido A cerrado (B–E no son prerequisito).

Implementá con TDD el manifiesto de Skills gestionadas y los comandos
royo-learn setup status / setup upgrade-skills --dry-run / --apply,
con la política exacta del plan: hash intacto → backup y actualizar;
modificada → versión candidata + diff + conflicto, sin sobrescribir;
no gestionada → no tocar.

Las siete pruebas del plan son obligatorias: instalación nueva, upgrade
sin modificaciones, upgrade con personalización, backup, dry-run,
idempotencia, recuperación tras fallo.
```

## Sesión 9 — Tramo 3: puerta de publicación del Hito 1

```text
Tu tramo asignado es el TRAMO 3 del plan.

No escribas funcionalidad nueva. Tu trabajo es auditar y cerrar:
1. Verificá uno por uno los 12 ítems del checklist del Tramo 3,
   ejecutando las pruebas que los demuestran. Reportá PASS/FAIL por ítem.
2. Ejecutá la batería completa de verificación final del plan.
3. Buscá activamente soft-passes o excepciones toleradas en los tests
   (grep de "acceptable", "soft", "skip" en *_test.go y e2e.go).
4. Actualizá el README para que describa ÚNICAMENTE lo demostrado.
5. Si TODO está en PASS: preparé el release v0.1.10 según el proceso
   del repo (.goreleaser.yml) pero NO publiques el tag: dejá el
   comando exacto listo y detenete para aprobación humana.
6. Si hay UN solo FAIL: no prepares release; reportá el FAIL con
   evidencia y qué recorrido debe reabrirse.
```

## Sesión 10 — Tramo 4 parte 1: CLI completa, MCP completo, errores (§4.1–4.3)

```text
Tu tramo asignado son las secciones 4.1, 4.2 y 4.3 del Tramo 4 del plan.

Prerequisito: v0.1.10 publicado o explícitamente aprobado por el humano.

Con TDD: registro declarativo único de comandos CLI del que derivan
help y dispatcher, con la prueba contractual de cinco condiciones del
plan; tools MCP restantes (learning_add_evidence ya existe desde el
Recorrido B; completá status, rollback, y las administrativas solo si
tienen utilidad real por MCP); modelo común de errores con el envelope
JSON del plan y exit codes contractuales de docs/17, con prueba por
clase de error. Nada destructivo en perfiles read ni agent.
```

## Sesión 11 — Tramo 4 parte 2: recurrencias y búsqueda (§4.4–4.5)

```text
Tu tramo asignado son las secciones 4.4 y 4.5 del Tramo 4 del plan.

Con TDD: conectá internal/recurrence con learning_report_occurrence y
royo-learn occurrence, registrando los nueve campos del plan y
aplicando la semántica de idempotencia de D5; métricas que distinguen
los cuatro estados del plan. Relaciones explícitas (duplicate_of,
extends, supersedes, contradicts, narrows, related) donde el agente
propone y la curación confirma. Sin embeddings.
```

## Sesión 12 — Tramo 4 parte 3: export/import, coherencia, migraciones (§4.6–4.8)

```text
Tu tramo asignado son las secciones 4.6, 4.7 y 4.8 del Tramo 4 del plan.

Con TDD: export/import/rebuild-index/review con formatos versionados,
dry-run, backup, detección de conflictos y prueba round-trip completa;
coherencia SQLite–Markdown con doctor detectando divergencias y
rebuild-index reparándolas (outbox solo con prueba de corte que
demuestre ventana irrecuperable, documentada antes en
CONTRACT-DECISIONS); migraciones versionadas, idempotentes,
respaldadas, probadas desde una base v0.1.9 REAL, e incapaces de
autoaprobar registros antiguos.
```

## Sesión 13 — Tramo 5: pruebas de contrato permanentes y CI

```text
Tu tramo asignado es el TRAMO 5 del plan.

Implementá las siete familias de pruebas de contrato permanentes del
plan y actualizá la CI a la matriz completa: Linux/Windows/macOS × Go
mínimo de go.mod y Go estable, con -race obligatorio en Linux, y los
doce tipos de job que lista el plan (incluye upgrade desde v0.1.9,
skill upgrade, fault injection e instalación limpia).
```

## Sesión 14 — Tramo 6: documentación final e informes

```text
Tu tramo asignado es el TRAMO 6 del plan.

1. Generá o validá docs/generated/ (CLI_REFERENCE, MCP_REFERENCE,
   ERROR_REFERENCE, PROFILES) desde los registros reales, no a mano.
2. Actualizá el README con las tres secciones del plan (qué hace el
   LLM, qué hace Royo-Learn, qué NO hace). Los README traducidos se
   actualizan o se marcan desactualizados.
3. Redactá docs/FINAL-IMPLEMENTATION-REPORT.md con los 17 puntos y la
   tabla Requisito/Estado/Prueba/Evidencia. Solo PASS, FAIL o
   NOT_APPLICABLE. Con un FAIL el proyecto no se declara terminado.
4. Si todo está en PASS: dejá preparado el release v0.2.0 y detenete
   para aprobación humana. No publiques el tag.
```

---

## Prompt de reanudación (si una sesión murió a medias)

```text
[Preámbulo común]

Tu tramo asignado es el mismo que la sesión anterior interrumpida:
[TRAMO / RECORRIDO X].

La sesión anterior quedó incompleta. Antes de continuar:
1. Leé la última entrada de docs/IMPLEMENTATION-LOG.md y el git log de
   la rama fix/v019-contract-recovery.
2. Ejecutá go build ./... y go test ./... para conocer el estado real.
3. Reportá qué encontrás: commits hechos, tests en rojo/verde, y qué
   ítems de la puerta de salida ya están cumplidos.
4. Continuá desde ahí. No repitas trabajo ya commiteado; no descartes
   commits existentes sin explicar por qué.
```

---

## Checklist del auditor humano (entre sesión y sesión)

Antes de lanzar la sesión siguiente, verificar en 5 minutos:

- [ ] La entrada nueva existe en `docs/IMPLEMENTATION-LOG.md` y lista commits reales (`git log` los confirma).
- [ ] `go build ./... && go test ./...` pasan en la rama.
- [ ] La puerta de salida del tramo está reportada ítem por ítem, todo en PASS.
- [ ] No aparecieron palabras sospechosas nuevas en tests: `rg -i "acceptable|soft.pass|skip.*expected" --glob "*_test.go" --glob "e2e.go"`.
- [ ] El diff no toca nada fuera del alcance del tramo (`git diff --stat` contra el commit de inicio de la sesión).
- [ ] Si el ejecutor se apartó del plan, hay una decisión escrita en `docs/CONTRACT-DECISIONS.md` que lo justifica.

Si algo falla: usar el prompt de reanudación sobre el mismo tramo. Nunca
avanzar de tramo "a cuenta de" arreglarlo después.
