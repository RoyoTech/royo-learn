# Matriz de aceptación trazable — Capa de descubrimiento

- **Estado:** contrato congelado (Hito 0)
- **Propósito:** enlazar cada invariante y requisito con su hito, criterio de
  aceptación y prueba, de modo que ningún hito avance con gates pendientes.
- **Fuentes:** `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` §22–§25,
  `docs/20-EXPERIENCE-INGESTION-PRD.md`, `docs/14-ACCEPTANCE-CRITERIA.md`.

## 1. Invariantes → dónde se garantizan

| ID | Invariante | Documento que lo fija | Hito que lo prueba |
|----|------------|-----------------------|--------------------|
| INV-01 | Go + SQLite son el núcleo | ADR-0001, 02-ARCH | todos |
| INV-02 | Cero red obligatoria | 20-PRD §7 | 1, 2 |
| INV-03 | Sin Python/Bash/`os.system`/shell | ADR-0001 §5, 24-TM T3 | 2, 3 |
| INV-04 | Sin daemon obligatorio | 20-PRD §3 | 3 |
| INV-05 | No transcript completo por defecto | 20-PRD §3, 21-DOM §4 | 1, 4 |
| INV-06 | Redacción antes de hash y persistencia | 21-DOM §5, 24-TM T2/T7 | 1 |
| INV-07 | El repo no amplía trust roots ni redirige credenciales | 24-TM §5 | 1, 2 |
| INV-08 | Experiencia observada ≠ conocimiento aprobado | 20-PRD §4 | 6, 7 |
| INV-09 | Un patrón nunca se publica/instala automáticamente | 23-PM §6, 24-TM T15 | 6, 7 |
| INV-10 | Toda promoción reutiliza `capture.Service` | 23-PM §6 | 7 |
| INV-11 | Ningún job aprueba o publica | 23-PM §9, 13-jobs | 7, 8 |
| INV-12 | SQLite = fuente operacional; Markdown/índices = derivados | 02-ARCH §8, ADR-0001 | todos |
| INV-13 | Windows/Linux/macOS | 14-ACC A | todos |
| INV-14 | CLI/MCP actuales y JSON estable preservados | 20-PRD §7, 05-MCP | todos |
| INV-15 | Preview hash, aprobación, publicación atómica, verificación, rollback intactos | 01-PRD RF-006..008, 03-DOM | todos |
| INV-16 | El cursor no se adelanta a un commit | 21-DOM §7, 24-TM T11 | 1, 8 |
| INV-17 | Lease en SQLite; `.lock` solo secundario | 21-DOM §8, 24-TM T10 | 8 |

## 2. Hitos → tareas → aceptación → prueba

### Hito 0 — Contrato y ADR (este entregable)

| Entregable | Aceptación | Verificación |
|------------|------------|--------------|
| docs 20–25 + ADR-0001 | términos definidos; sin contradicción con `main`; seguridad explícita | revisión documental; baseline sin cambios de código |
| Actualización 01/02/17 | reframe de no-objetivos y "auto-capture"; errores nuevos listados | diff documental |

**Gate de salida Hito 0:** ningún `.go`/`.sql` modificado; `go build ./...` y
`go vet ./...` siguen verdes (baseline); revisión aprueba antes de migración 004.

### Hito 1 — Dominio y almacenamiento (migración 004)

| Tarea | Aceptación | Prueba |
|-------|------------|--------|
| tipos de dominio + validación | un envelope válido crea sesión y turno | unit |
| migración `004_experience_ingestion.sql` | tablas + índices creados; idempotente | migration test |
| idempotencia | reintento exacto no duplica; revisión actualiza seguro | unit/integration |
| redacción | secreto no llega a ningún sink | security test |
| cursor | se actualiza solo tras commit | integration |
| gates globales | `go test -race ./...` y cross-build pasan | CI |

### Hito 2 — OpenCode `--once`

| Tarea | Aceptación | Prueba |
|-------|------------|--------|
| fixture SQLite anonimizada | lee fixture; ignora incompletos; captura cerrados | integration |
| discovery seguro + read-only | cero side effects sobre DB de OpenCode | integration |
| reinicio | no duplica | integration |
| seguridad de path | path fuera de raíz bloqueado | security test |
| portabilidad | build Windows/Linux/macOS; sin Python/Bash | CI |

### Hito 4 — Trace progresivo

| Tarea | Aceptación | Prueba |
|-------|------------|--------|
| tabla Learning↔Event + resolver | Learning promovido muestra sesiones origen | integration |
| bounds y redacción | excerpt solo con flag; tool calls acotados; sin reasoning privado | unit/security |
| fuente mutada/ausente | detectada; respuesta parcial < 1 MB | integration |

### Hito 5 — Detectores deterministas

| Aceptación | Prueba |
|------------|--------|
| precisión > recall; cero eventos en conversación rutinaria | unit/fixtures |
| mismo input + versión = mismo output | unit |

### Hito 6 — Patrones y recurrencia (migración 005)

| Aceptación | Prueba |
|------------|--------|
| 3 sesiones similares cualifican; 3 reintentos de una sesión no | integration |
| patrón ya cubierto no se duplica; contradicción bloquea; false cluster descartable | integration |

### Hito 7 — Promoción gobernada

| Aceptación | Prueba |
|------------|--------|
| promoción no publica; usa `capture.Service`; fuentes enlazadas | e2e |
| patrón → `promoted`; error deja estado consistente; idempotente | integration |

### Hito 8 — Motor de jobs (migración 006)

| Aceptación | Prueba |
|------------|--------|
| dos procesos no ejecutan el mismo job; lease expira | integration/race |
| input sin cambios se omite; degraded conserva último éxito; crash no bloquea | integration |

### Hito 9 — Recuperación lexical

| Aceptación | Prueba |
|------------|--------|
| contratos previos siguen; ranking determinista; sin FTS injection | unit/benchmark |
| búsquedas ES/EN; p95 local en presupuesto | benchmark |

## 3. Amenazas de seguridad → prueba

| Amenaza (24-TM) | Escenario adversarial | Hito |
|-----------------|-----------------------|------|
| T1 prompt injection | transcript con instrucciones | 2 |
| T2 leer `.env` | secreto en transcript | 1 |
| T3 shell metachars | `;`, `|`, `$()` en command | 2 |
| T4 path traversal | `../`, symlink, UNC | 2 |
| T5 config de repo hostil | endpoint/root desde proyecto | 1 |
| T7 secreto multicanal | user+assistant+output | 1 |
| T10 race de ingestors | dos procesos | 8 |
| T11 cursor adelantado | fallo antes de commit | 1/8 |
| T12 fuente mutada | cambio tras indexar | 4 |
| T15 auto-publicación | intento de Skill sin curación | 7 |

## 4. Cobertura objetivo (nuevos paquetes)

```text
internal/experience >= 90% · internal/patterns >= 90% · internal/jobs >= 90%
internal/retrieval  >= 85% · internal/adapters >= 85%
```

## 5. Regla de parada

Detener y abrir ADR ante: necesidad de transcript completo; endpoint remoto
obligatorio; credenciales no previstas; cambio de formato upstream; semántica que
exige CGO/runtime pesado; config de proyecto que necesita ampliar trust roots; un
job que podría publicar; modificar estado de `Learning` sin sus servicios; o
contradicción con una garantía de publicación existente.
