# PRD — Ingestión de experiencia (capa de descubrimiento)

- **Estado:** contrato congelado (Hito 0)
- **Depende de:** `docs/01-PRD.md`, `docs/02-ARCHITECTURE.md`, `docs/03-DOMAIN-MODEL.md`
- **Complementa a:** `docs/21-EXPERIENCE-DOMAIN.md`, `docs/22-ADAPTER-CONTRACT.md`, `docs/23-PATTERN-MINING.md`, `docs/24-EXPERIENCE-THREAT-MODEL.md`
- **Autoridad de implementación:** `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`

## 1. Problema

El PRD v1 (`docs/01-PRD.md`) resuelve el ciclo desde que un aprendizaje
estructurado **entra** al sistema. La carencia está antes:

- la captura depende de que el agente recuerde llamar a `capture-learning`;
- depende de que el agente interprete correctamente la experiencia;
- no existe ingestión automática de sesiones terminadas;
- no hay índice operacional de sesiones y turnos;
- desde un `Learning` no se puede volver al intercambio exacto que lo originó;
- las recurrencias se registran explícitamente, pero no se descubren solas.

## 2. Objetivo

Ampliar Royo-Learn para **observar automáticamente** experiencia producida por
agentes (OpenCode, Claude Code, Codex, Pi), conservar procedencia hasta la
sesión y el turno, detectar recurrencias reutilizables y permitir recuperación
progresiva, **sin debilitar ninguna garantía existente**.

La cadena completa que esta capa habilita:

```text
experiencia automática → procedencia verificable → evento estructurado
→ patrón recurrente → candidato revisable → Learning gobernado → evidencia
→ curación → aprobación → publicación → verificación → recurrencia medida
```

La primera mitad es nueva. La segunda ya existe y **se reutiliza** (§7 CU-08).

## 3. No objetivos (v1 de la capa)

- almacenar conversaciones completas por defecto (solo referencias + resúmenes
  acotados y opcionales);
- publicar o instalar una Skill automáticamente desde una conversación;
- permitir que un LLM escriba directamente en `AGENTS.md`, Skills o conocimiento
  compartido;
- introducir un daemon obligatorio, red obligatoria o proveedor LLM embebido;
- convertir la búsqueda vectorial en requisito de funcionamiento;
- tratar la experiencia observada como conocimiento aprobado;
- ejecutar comandos o instrucciones extraídos del transcript;
- ampliar trust roots o redirigir credenciales desde configuración de proyecto.

> Reemplaza la afirmación previa "no hay auto-capture de conversaciones" por el
> contrato exacto: **captura estructurada y acotada; transcripts externos por
> defecto; publicación siempre gobernada; semántica opcional y derivada.**

## 4. Principio rector: la experiencia observada no es conocimiento

Una sesión ingerida produce, como mucho, **evidencia preliminar**. Nada de lo
observado cambia el estado de un `Learning` sin pasar por promoción explícita →
`capture.Service` → curación → aprobación. Esta frontera es innegociable y se
audita.

## 5. Casos de uso obligatorios

### CU-E01 — Descubrir experiencia sin intervención

Un adaptador de plataforma localiza la fuente nativa de sesiones (solo lectura),
reconoce sesiones/turnos, determina qué turnos están estables y produce un
`ExperienceEnvelope`. El núcleo valida, redacta, calcula fingerprints, aplica
idempotencia y persiste.

### CU-E02 — Conservar procedencia hasta el turno

Cada turno persistido conserva un `TranscriptLocator` (fuente, sesión, turno,
offset, hash de origen) que permanece **local en SQLite** y nunca se materializa
en records Markdown por defecto.

### CU-E03 — Recuperación progresiva

`learning_search → learning_get → learning_trace`. La traza devuelve por defecto
referencias y resúmenes; un excerpt exige flag explícito y pasa por redacción.

### CU-E04 — Descubrir recurrencias

El sistema agrupa eventos equivalentes por fingerprint determinista y cualifica
un patrón solo bajo criterios conservadores (ver `docs/23-PATTERN-MINING.md`).

### CU-E05 — Promover un patrón revisado

El agente revisa las fuentes originales vía `learning_trace` y llama a
`learning_promote_pattern`. El núcleo re-comprueba el patrón y crea el `Learning`
**mediante `capture.Service`**, enlazando los eventos como evidencia/origen.

### CU-E06 — Ejecución incremental y recuperable

La ingestión, detección y minería corren como jobs con `input_digest`, lease con
expiración y estado `ok/degraded/error`; un reinicio no duplica turnos.

### CU-E07 — Degradación segura

Si una fuente no existe o cambió de esquema, la respuesta es `degraded`/tipada;
las herramientas normales de Royo-Learn no se ven afectadas.

### CU-E08 — Reutilizar el núcleo

La promoción **no** duplica lógica de `Learning`: usa `capture.Service`,
deduplicación, evidencia (`internal/evidence`) y auditoría existentes.

## 6. Requisitos funcionales

| ID | Requisito |
|----|-----------|
| RF-E01 | Contrato neutral `ExperienceEnvelope` entre adaptadores y núcleo. |
| RF-E02 | Orden obligatorio: validar esquema → validar proyecto/locator → límites de bytes → **redacción** → normalización → fingerprints → idempotencia → persistencia → auditoría. |
| RF-E03 | Idempotencia en dos niveles: identidad externa (`source+session+turn`) y fingerprint de revisión. Un reintento técnico nunca aumenta recurrencia. |
| RF-E04 | Estabilidad del turno como lógica de dominio testeable, con `tail_quiet_period` configurable. |
| RF-E05 | `IngestionCursor` reconstruible: si se pierde, releer y deduplicar sin re-resumir. |
| RF-E06 | Procedencia navegable `Learning ↔ ExperienceEvent` vía tabla dedicada. |
| RF-E07 | Minería como pipeline auditable, no una llamada única a un LLM. |
| RF-E08 | Salud especializada: `experience_sources`, `experience_cursors`, `job_leases`, etc., en `doctor`. |
| RF-E09 | Códigos de error estables (ver `docs/17-ERROR-CODES.md`, sección experiencia). |
| RF-E10 | Toda la capa **deshabilitada por defecto** hasta completar setup. |

## 7. Requisitos no funcionales

- local-first; cero red obligatoria; multiplataforma Windows/Linux/macOS;
- sin Python/Bash/`os.system`/shell interpolation;
- arranque MCP sin adaptadores activos: no degradar > 10 %;
- ingestión incremental de cero cambios: < 100 ms esperado;
- `search` p95 < 250 ms sin semántica; `trace` refs < 250 ms;
- respuesta MCP ≤ 1 MB; datos UTC; JSON estable y versionado;
- índices FTS/vectoriales reconstruibles sin perder Learnings/evidencia/auditoría.

## 8. Configuración (deshabilitada por defecto)

```yaml
experience:
  enabled: false
  tail_quiet_period_seconds: 30
  poll_interval_seconds: 10
  max_turn_bytes: 262144
  store_safe_summary: true
  store_raw_transcript: false
patterns:
  enabled: false
  min_distinct_sessions: 3
  min_distinct_days: 2
  min_successful_occurrences: 2
  qualification_mode: conservative
```

Trust boundary (ver `docs/24-EXPERIENCE-THREAT-MODEL.md` §config): la
configuración de proyecto solo ajusta knobs de bajo riesgo; **nunca** roots,
endpoints, proveedores, credenciales ni límites superiores.

## 9. Criterio de éxito de la capa

1. Royo-Learn captura automáticamente experiencia de al menos OpenCode;
2. un reinicio no duplica turnos;
3. cada `Learning` promovido rastrea hasta sesiones y turnos;
4. el transcript permanece externo por defecto;
5. se detectan patrones recurrentes sin publicar automáticamente;
6. la promoción reutiliza `capture.Service`;
7. ninguna falla de adaptador rompe el núcleo;
8. no existe dependencia de MemSearch/Milvus/Python/Bash.

## 10. Trazabilidad

Cada requisito se mapea a hito, acceptance y prueba en
`docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md`.
