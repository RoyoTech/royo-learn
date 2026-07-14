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
--file <json|yaml>
--stdin
--title
--type
--lesson
--idempotency-key
--collect-git-diff
--collect-git-status
--evidence-command <JSON array>
--no-engram
```

Debe rechazar capturas sin lección reusable.

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
--json
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
