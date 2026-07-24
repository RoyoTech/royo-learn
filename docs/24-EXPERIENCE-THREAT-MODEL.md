# Modelo de amenazas — Capa de descubrimiento de experiencia

- **Estado:** contrato congelado (Hito 0)
- **Alcance:** ingestión de transcripts de agentes, minería de patrones,
  recuperación progresiva y jobs incrementales.
- **Complementa a:** `docs/10-SECURITY.md`.

## 1. Postura de confianza

El contenido de cualquier transcript, DB de harness, path de fuente o
configuración de repositorio es **datos no confiables**, nunca instrucciones. La
única autoridad de configuración sensible es la configuración **de usuario**.

Frontera de confianza (de más a menos):

```text
configuración de usuario   → puede fijar roots, endpoints, proveedores, credenciales por referencia
configuración de proyecto  → solo knobs de bajo riesgo (enable, thresholds, quiet period, límites inferiores)
DB/transcript del harness  → datos leídos en solo lectura; nunca ejecutables
argumentos MCP/CLI         → validados; no pueden inyectar paths arbitrarios
```

## 2. Activos a proteger

- integridad del núcleo de `Learning` (estado, evidencia, aprobaciones);
- secretos del usuario presentes en transcripts (`.env`, tokens, claves);
- razonamiento privado y mensajes de sistema del harness;
- las DB nativas de OpenCode/Claude/Codex/Pi (deben quedar intactas);
- la cadena de auditoría append-only.

## 3. Amenazas y mitigaciones

| # | Amenaza | Mitigación |
|---|---------|------------|
| T1 | Prompt injection en el transcript ("ejecuta…", "aprueba…") | El transcript es dato; ningún adaptador ni el núcleo interpreta contenido como instrucción; no hay ejecución de comandos del transcript. |
| T2 | El transcript pide leer `.env`/secretos | Redacción **antes** de hash/persistencia; ningún sink recibe contenido sin redactar (SQLite, blob, Markdown, audit, respuesta MCP/CLI, logs). |
| T3 | Command string con `;`, `|`, `$()` | Sin shell en ninguna ruta; `spawn` con args separados; `SafeToolCall` no ejecuta, solo describe. |
| T4 | Locator con `../`, symlink escape, UNC/verbatim path | Validación contra roots del usuario; no seguir symlinks fuera de raíz; errores `experience_locator_outside_root`/`symlink_escape`. |
| T5 | Config de proyecto intenta endpoint externo o nueva root | Trust boundary: el proyecto no amplía roots, endpoints, proveedores ni credenciales; solo knobs de bajo riesgo. |
| T6 | Payload comprimido/oversized | `max_turn_bytes` y límite de respuesta MCP ≤ 1 MB; `experience_payload_too_large`. |
| T7 | Secreto repetido en user, assistant y output | La redacción cubre los tres canales antes del fingerprint; el hash se calcula sobre contenido redacted. |
| T8 | DB del harness corrupta / JSONL truncado / output binario | Errores tipados y `degraded`; nunca un `catch-all` que oculte; el núcleo sigue operativo. |
| T9 | Escritura accidental en la DB del harness | Apertura estrictamente read-only; prohibido escribir tablas del harness. |
| T10 | Race de dos ingestors | Lease en SQLite con expiración; `.lock` solo defensa secundaria; `job_lease_conflict`. |
| T11 | Cursor adelantado antes del commit | El cursor se actualiza **solo tras commit**; `experience_cursor_conflict`. |
| T12 | La fuente cambia de ubicación/contenido tras indexar | `SourceHash`/`SourceRevision` detectan sustitución; trace devuelve `trace_source_changed`/`unavailable`. |
| T13 | Markdown/frontmatter malicioso en records | Markdown es proyección, no autoridad; validación de frontmatter y paths existente. |
| T14 | Filtración de razonamiento privado vía trace | Default sin excerpt; excerpt solo con flag y redacción; nunca reasoning privado ni mensajes de sistema secretos. |
| T15 | Promoción automática de una conversación a Skill | Imposible por diseño: promoción → `capture.Service` → curación → aprobación; ningún job publica. |
| T16 | Un adaptador `host_llm` inyecta evidencia `strong` | Salida `host_llm` no confiable; no puede elevar evidencia ni promover. |

## 4. Seguridad de `learning_trace`

- default: referencias y resúmenes, no transcript;
- excerpt requiere solicitud explícita y pasa por redacción;
- respetar `max_bytes`; respuesta ≤ 1 MB;
- una fuente ausente produce `unavailable`, no error global;
- validar que el proyecto de la fuente coincide con el `Learning`;
- no permitir path arbitrario desde argumentos MCP;
- errores: `trace_source_unavailable`, `trace_source_changed`,
  `trace_excerpt_forbidden`, `trace_limit_exceeded`.

## 5. Trust boundary de configuración

La configuración de **usuario** puede fijar: roots de transcripts; endpoints;
proveedores; credenciales por referencia de entorno; activación de adaptadores.

La configuración de **proyecto** solo puede fijar: enable/disable por proyecto;
quiet period dentro de límites; thresholds; máximo de resultados; nombres de
fuentes ya autorizadas.

El repositorio **no** puede: añadir roots; cambiar endpoint; seleccionar archivo
de prompt externo; cambiar proveedor con credenciales; habilitar ejecución de
comandos; ampliar límites superiores.

## 6. Auditoría

Eventos append-only (`experience_session_discovered`, `experience_turn_ingested`,
`experience_pattern_promoted`, `experience_trace_unavailable`, `job_*`, …) con
actor, project, entidad, estado previo/nuevo, hash de payload, versión de
detector/adaptador, resultado y error tipado. **No** se registra el texto
completo del transcript en el audit.

## 7. Pruebas de seguridad obligatorias

Los escenarios T1–T16 se traducen a tests adversariales (ver
`PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` §23.4) y se rastrean en
`docs/25-EXPERIENCE-ACCEPTANCE-MATRIX.md`.
