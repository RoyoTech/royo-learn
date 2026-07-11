# Esquema de datos

La migración ejecutable inicial se encuentra en `migrations/001_init.sql`.

## Configuración SQLite

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
```

## Tablas

### `projects`

Identidad estable de repositorios. La ruta puede cambiar; el remote y fingerprint ayudan a reconciliar.

### `learnings`

Estado actual de cada aprendizaje.

### `learning_revisions`

Snapshot inmutable de cada revisión semántica.

### `evidence`

Metadatos y referencias a blobs.

### `learning_relations`

Relaciones entre aprendizajes.

### `curations`

Decisiones de curación.

### `publication_previews`

Plan y hash exacto previo a publicar.

### `approvals`

Aprobaciones ligadas a preview.

### `publications`

Resultado de escritura y rollback.

### `occurrences`

Reapariciones o prevenciones.

### `audit_events`

Append-only.

### FTS5

`learnings_fts` indexa:

- title;
- context;
- observation;
- reusable_lesson;
- retrieval_terms;
- project_key.

Triggers sincronizan insert/update/delete lógico.

## Migraciones

- tabla `schema_migrations`;
- cada migración se ejecuta en transacción;
- checksum embebido;
- si una migración aplicada cambia, `doctor` falla;
- backups antes de migración destructiva;
- no existen migraciones destructivas en v1.

## IDs

Usar UUID v7 o ULID. No exponer IDs autoincrementales como identidad externa.

## Borrado

No borrar aprendizajes en uso normal. Cambiar estado a `archived` o `rejected`.

Evidencias pueden purgarse solo si:

- no son la única evidencia de una publicación;
- el hash y metadatos se conservan;
- la acción se audita.
