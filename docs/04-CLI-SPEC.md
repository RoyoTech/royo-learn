# Especificación CLI

## Convenciones

- salida humana por defecto;
- `--json` produce JSON estable;
- errores a `stderr`;
- exit codes documentados;
- soporte de `--project-root`;
- timestamps UTC RFC3339;
- no prompts interactivos en `--json` o CI.

## Comandos

### `royo-learn version`

Muestra versión, commit, fecha y Go version.

### `royo-learn init`

Inicializa el proyecto:

```text
.royo-learn/
├── config.yaml
├── records/
├── evidence/
├── backups/
└── .gitignore
```

No pisa archivos.

Flags:

```text
--project-root
--shared-root
--force        solo recrea archivos generados, nunca records
--json
```

### `royo-learn doctor`

Checks:

```text
config
database
migrations
project
git
filesystem
engram
gentle_ai
skill_registry
codex_mcp
shared_library
record_integrity
```

Flags:

```text
--check <name>
--fix-safe
--json
```

`--fix-safe` solo puede crear directorios, reparar permisos de archivos propios, reconstruir índices y refrescar el skill registry. No edita configuración global de agentes.

### `royo-learn capture`

Entradas:

```text
--file <json>
--stdin
--title
--context
--observation
--lesson
--type
--scope
--destination <none|project|shared|skill|agents_rule>
--confidence <low|medium|high>
--evidence-level <insufficient|weak|moderate|strong>
--idempotency-key
--evidence-file <path>
--collect-git-status
--collect-git-diff
--evidence-command <JSON array>
--project-root
--json
```

Debe rechazar capturas sin lección reusable.

`--file` y `--stdin` leen una petición de captura completa en JSON (ver
`schemas/capture-request.schema.json`), incluido el array `evidence[]`. Las
banderas explícitas tienen prioridad sobre los campos del archivo.

**Destino y nivel de evidencia son obligatorios para poder aprobar.** Sin
`--destination`, toda captura del CLI propone `project`, lo que hace
estructuralmente inalcanzables las decisiones de curación `approve_new_skill` y
`approve_skill_update`. Sin `--evidence-level`, toda captura nace `insufficient`
y el umbral de D3 la rechaza. Ambas banderas existen precisamente para que el
recorrido `capture → curate → approved` sea alcanzable desde el CLI.

Colectores (D3): evidencia entregada directamente (`--evidence-file`),
`--collect-git-status`, `--collect-git-diff`, y `--evidence-command` con un
comando explícitamente permitido. No hay más colectores.

La redacción de secretos ocurre **antes** de cualquier escritura: SQLite, blob
store, Markdown, audit log, salida JSON y logs.

### `royo-learn evidence add <learning-id>`

Adjunta evidencia a un aprendizaje ya capturado. Es la operación que hace
utilizable el estado `needs_evidence`: sin ella, un aprendizaje devuelto a
`needs_evidence` no tiene forma de volver a `approved`.

```text
royo-learn evidence add <learning-id> \
  --kind <file|git_diff|git_commit|command|test|engram_observation|issue|pull_request|text|external_reference> \
  --summary <text> \
  --source <text> \
  --content <text>
```

Flags:

```text
--kind <kind>              (default: text)
--summary <text>
--source <text>
--content <text>
--evidence-file <path>     lee un array JSON de registros de evidencia
--collect-git-status
--collect-git-diff
--evidence-command <JSON array>
--evidence-level <insufficient|weak|moderate|strong>
--project-root
--json
```

`--evidence-level` actualiza el nivel declarado del aprendizaje en la misma
operación. Sin él, un aprendizaje capturado con el nivel por defecto
(`insufficient`) seguiría siendo inaprobable aunque se le adjunte evidencia
real, porque el umbral de D3 exige las dos condiciones.

Salida (`--json`):

```json
{
  "learning_id": "...",
  "evidence_ids": ["..."],
  "evidence_count": 1,
  "evidence_level": "moderate",
  "redacted": false
}
```

Debe rechazar una llamada sin ningún registro de evidencia.

### `royo-learn get <id>`

Flags:

```text
--include-evidence
--json
```

### `royo-learn list`

Filtros:

```text
--status
--type
--scope
--project
--limit
--offset
--json
```

### `royo-learn search <query>`

Flags:

```text
--all-projects
--include-engram
--status
--limit
--json
```

Combina resultados, pero identifica la fuente:

```text
royo_learn
engram
```

### `royo-learn curate <id>`

Solo acepta un archivo de decisión o flags completos. No “piensa” la curación.

```text
--file <curation.json>
--decision
--rationale
--destination
--target
--json
```

### `royo-learn preview <id>`

Genera diff y preview hash.

```text
--publication-file
--output <path>
--json
```

No escribe destinos.

### `royo-learn approve <id>`

```text
--preview-hash
--approved-by
--reason
--expires
--json
```

En modo interactivo muestra diff y requiere confirmación exacta. En modo no interactivo necesita todos los flags.

### `royo-learn publish <id>`

```text
--preview-hash
--approval-id
--dry-run=true
--apply
--json
```

`--apply` y `--dry-run=false` son equivalentes. Sin uno de ellos, nunca escribe.

### `royo-learn rollback <publication-id>`

Valida que el destino no haya cambiado desde la publicación. Si cambió, bloquea rollback automático y genera un patch de reversión.

### `royo-learn occurrence`

```text
--learning-id
--fingerprint
--summary
--outcome
--retrieved <true|false>
--skill-activated <true|false>
--evidence-file
--idempotency-key
--json
```

`--idempotency-key` aplica la semántica D5: la misma clave en un reintento
devuelve el registro existente y no crea una segunda recurrencia. La respuesta
incluye `new` (`true` si se creó, `false` si fue un reintento técnico).

### `royo-learn status <id>`

Informa el estado de ciclo de vida de un aprendizaje: `status`, tipo, revisión
y última actualización. Es el equivalente CLI de la tool MCP `learning_status`.

```text
--project-root
--json
```

### `royo-learn recurrences`

Lista los registros de recurrencia de un aprendizaje (lectura de
`internal/recurrence`, pieza del contrato de D5).

```text
--learning-id   (obligatorio)
--limit
--project-root
--json
```

### `royo-learn metrics`

Calcula las métricas de recurrencia de un aprendizaje: frecuencia, intervalo
medio, tendencia y si necesita revisión. Distingue cero recurrencias, datos
insuficientes, recurrencia repetida y recurrencia prevenida.

```text
--learning-id   (obligatorio)
--project-root
--json
```

### `royo-learn setup`

Configura la herramienta para su primer uso. Subcomandos:

```text
install          registra el servidor MCP e instala las Skills incluidas
uninstall        revierte la instalación
status           muestra el estado de las Skills gestionadas
upgrade-skills   actualiza de forma segura las Skills ya instaladas (--dry-run|--apply)
```

### `royo-learn review`

Lista:

- candidatos;
- needs_evidence;
- aprobados no publicados;
- recurrencias;
- memories de Engram potencialmente convertibles, solo si se solicita.

### `royo-learn export`

```text
--format jsonl|markdown
--project
--output
```

### `royo-learn import`

```text
--file
--dry-run
--apply
```

### `royo-learn rebuild-index`

Reconstruye DB/index desde records sin perder audit log salvo flag explícito de recuperación.

### `royo-learn mcp`

Inicia servidor MCP stdio.

```text
--tools read|agent|admin
--project-root
```

Profiles:

- `read`: búsqueda y get;
- `agent`: ciclo normal;
- `admin`: import, rollback y reparación.

### `royo-learn e2e --temp`

Ejecuta una demostración aislada y devuelve no-cero ante cualquier fallo.

### `royo-learn self-update`

Replaces the running binary with an official GitHub release after verifying its SHA-256 checksum against the published `checksums.txt`.

```text
--check     report whether an update is available, without downloading
--version   install a specific release (explicit downgrades allowed)
--json
```

Behavior:

- development builds (`version` = `dev`) refuse implicit updates; pass `--version` to install a release explicitly;
- implicit downgrades are refused; installing an older release requires `--version`;
- `--check` cannot be combined with `--version` (`invalid_argument`);
- release download URLs must use HTTPS (redirects included) and downloads are size-capped before extraction;
- when `GITHUB_TOKEN` is set it is sent as an `Authorization: Bearer` header to raise GitHub API rate limits;
- on Windows the previous binary is parked as `<binary>.old` and removed on the next run; a `<binary>.update-lock` file blocks concurrent updates.

Error codes (JSON envelope on `stderr`): `invalid_argument`, `development_build`, `self_update_failed`.

Exit codes: `0` on success (including "already up to date"); `1` on any error envelope.

## Exit codes

```text
0  éxito
2  argumentos inválidos
3  configuración inválida
4  proyecto ambiguo/no encontrado
5  entidad no encontrada
6  transición inválida
7  aprobación requerida/inválida
8  conflicto de destino
9  verificación fallida
10 integración opcional no disponible cuando fue requerida
11 seguridad/ruta bloqueada
12 corrupción/integridad
13 error de almacenamiento
14 error MCP
15 error externo
```
