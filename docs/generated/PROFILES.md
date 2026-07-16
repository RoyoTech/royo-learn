<!-- GENERADO AUTOMÁTICAMENTE — NO EDITAR A MANO. -->
<!-- Fuente: el registro `allTools`. Generador: internal/mcpserver/gendocs_test.go. -->
<!-- Regenerar: go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs -->

# Perfiles MCP

Cada perfil sirve un subconjunto del registro. Nada destructivo aparece en
`read` ni en `agent` (D2), y esa regla la impone una prueba de contrato
permanente, no esta tabla.

## `read`

Herramientas: **5**.

- `learning_search` — read
- `learning_get` — read
- `learning_list` — read
- `learning_doctor` — read
- `learning_status` — read

## `agent`

Herramientas: **14**.

- `learning_capture` — write
- `learning_search` — read
- `learning_get` — read
- `learning_list` — read
- `learning_doctor` — read
- `learning_curate` — write
- `learning_add_evidence` — write
- `learning_publication_preview` — write
- `learning_approve` — write
- `learning_list_recurrences` — read
- `learning_compute_metrics` — read
- `learning_publish` — write
- `learning_report_occurrence` — write
- `learning_status` — read

## `admin`

Herramientas: **15**.

- `learning_capture` — write
- `learning_search` — read
- `learning_get` — read
- `learning_list` — read
- `learning_doctor` — read
- `learning_curate` — write
- `learning_add_evidence` — write
- `learning_publication_preview` — write
- `learning_approve` — write
- `learning_list_recurrences` — read
- `learning_compute_metrics` — read
- `learning_publish` — write
- `learning_report_occurrence` — write
- `learning_status` — read
- `learning_rollback` — **destructive**

## Nombres de perfil deprecated

| Deprecated | Canónico |
|------------|----------|
| `full` | `admin` |
| `minimal` | `read` |
| `standard` | `agent` |
