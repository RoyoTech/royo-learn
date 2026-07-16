<!-- GENERADO AUTOMÁTICAMENTE — NO EDITAR A MANO. -->
<!-- Fuente: el registro `allTools`. Generador: internal/mcpserver/gendocs_test.go. -->
<!-- Regenerar: go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs -->

# Referencia MCP

Herramientas canónicas registradas: **15**.

| Herramienta | Acceso | Perfiles | Aliases deprecated | Descripción |
|-------------|--------|----------|--------------------|-------------|
| `learning_capture` | write | admin, agent | `capture_learning` | Capture a new learning or return an existing one by content hash. Deduplication is automatic. |
| `learning_search` | read | admin, agent, read | `search_learnings` | Search learnings by full-text query. Returns ranked results from the FTS5 index. |
| `learning_get` | read | admin, agent, read | `get_learning` | Retrieve a single learning by its unique identifier. |
| `learning_list` | read | admin, agent, read | `list_learnings` | List learnings for the current project. Filter by status, type, or scope. |
| `learning_doctor` | read | admin, agent, read | `doctor` | Run health checks on the server: database connectivity, project resolution, version info. |
| `learning_curate` | write | admin, agent | `curate_learning` | Curate a learning: approve, reject, or request more evidence. Approval enforces evidence thresholds. |
| `learning_add_evidence` | write | admin, agent | `add_evidence` | Attach evidence records to an existing learning so it can satisfy the approval threshold. Redaction runs before persistence. |
| `learning_publication_preview` | write | admin, agent | `preview_publication` | Generate a publication preview showing what files would be created or modified. Persists the preview and returns its hash. |
| `learning_approve` | write | admin, agent | — | Record explicit human approval bound to a publication preview hash. Required before publishing to AGENTS.md, shared scope, or an existing Skill. |
| `learning_list_recurrences` | read | admin, agent | `list_recurrences` | List recurrence records for a learning, tracking when the same pattern appears across captures. |
| `learning_compute_metrics` | read | admin, agent | `compute_metrics` | Compute recurrence metrics (frequency, interval, trend) for a learning's recurrence pattern. |
| `learning_publish` | write | admin, agent | `publish_learning` | Publish an approved learning. Requires a preview hash, and an approval_id when the preview reports requires_approval=true. |
| `learning_report_occurrence` | write | admin, agent | — | Record an explicit occurrence of a learning's pattern, with outcome, retrieval and skill-activation detail. The same idempotency_key on a retry does not create a second record (D5). |
| `learning_status` | read | admin, agent, read | — | Report the current status of a learning: its lifecycle state, type, revision and last update. |
| `learning_rollback` | **destructive** | admin | — | Roll back a publication, restoring every file it changed from backups. Destructive: confined to the admin profile. |

## Aliases deprecated

Los aliases siguen siendo invocables y comparten el handler de su nombre
canónico, pero **no se anuncian** en las instrucciones del servidor (D14, D16).
