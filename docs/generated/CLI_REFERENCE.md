<!-- GENERADO AUTOMÁTICAMENTE — NO EDITAR A MANO. -->
<!-- Fuente: el registro `commandRegistry`. Generador: cmd/royo-learn/gendocs_test.go. -->
<!-- Regenerar: go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs -->

# Referencia CLI

Comandos implementados: **25**.

| Comando | Resumen |
|---------|---------|
| `royo-learn version` | Print version information |
| `royo-learn init` | Initialize a new royo-learn project |
| `royo-learn doctor` | Run system diagnostics |
| `royo-learn capture` | Capture a new learning |
| `royo-learn evidence` | Attach or list evidence for a learning |
| `royo-learn get` | Retrieve a single learning by ID |
| `royo-learn list` | List learnings with optional filters |
| `royo-learn search` | Search captured learnings |
| `royo-learn curate` | Curate an existing learning |
| `royo-learn preview` | Preview publication of a learning |
| `royo-learn approve` | Approve a publication preview (human authorization) |
| `royo-learn publish` | Publish a curated learning |
| `royo-learn rollback` | Rollback a published learning |
| `royo-learn occurrence` | Record a recurrence of a learning's pattern |
| `royo-learn recurrences` | List recurrence records for a learning |
| `royo-learn metrics` | Show recurrence metrics for a learning |
| `royo-learn status` | Report the lifecycle status of a learning |
| `royo-learn review` | List candidates, needs-evidence, approved-not-published and recurrences |
| `royo-learn export` | Export a versioned, portable snapshot of the store |
| `royo-learn import` | Validate and import a bundle (dry-run by default) |
| `royo-learn rebuild-index` | Rebuild the search index and re-materialize records from SQLite |
| `royo-learn mcp` | Start the MCP server over stdio |
| `royo-learn e2e` | Run the end-to-end demonstration |
| `royo-learn setup` | Configure the tool for first use |
| `royo-learn self-update` | Update to the latest or a specific version |

## Comandos deprecated

| Deprecated | Canónico | Resumen |
|------------|----------|---------|
| `mcp-serve` | `mcp` | Deprecated alias of mcp |
| `engram-health` | `doctor` | Deprecated: folded under doctor |
| `engram-search` | `search` | Deprecated: folded under search --include-engram |

## Comandos declarados y no construidos

Un comando aquí está documentado pero **no se puede ejecutar todavía**.
La lista se deriva del registro, así que no puede mentir por omisión.

Ninguno: hoy ninguna superficie declara algo que no exista.
