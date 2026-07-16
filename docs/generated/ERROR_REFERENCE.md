<!-- GENERADO AUTOMÁTICAMENTE — NO EDITAR A MANO. -->
<!-- Fuente: `AllErrorCodes()` y `ErrorCode.ExitCode()`. Generador: internal/domain/gendocs_test.go. -->
<!-- Regenerar: go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs -->

# Referencia de errores

Todo error del producto lleva uno de estos códigos estables. El código
determina el exit code: ninguna superficie elige una constante a mano ni
interpreta un error comparando cadenas.

Códigos: **39**.

| Exit code | Códigos |
|-----------|---------|
| 2 | `evidence_missing`, `evidence_too_large`, `invalid_argument`, `payload_too_large` |
| 3 | `invalid_config` |
| 4 | `ambiguous_project`, `project_not_found`, `unknown_project` |
| 5 | `learning_not_found`, `preview_not_found` |
| 6 | `invalid_transition` |
| 7 | `approval_expired`, `approval_invalid`, `approval_required` |
| 8 | `dirty_target`, `duplicate_learning`, `preview_hash_mismatch`, `publication_conflict`, `rollback_conflict`, `target_ambiguous`, `target_changed` |
| 9 | `verification_failed` |
| 10 | `engram_ambiguous_project`, `engram_unavailable`, `gentle_ai_unavailable`, `skill_registry_failed` |
| 11 | `path_outside_root`, `protected_path`, `secret_detected`, `symlink_escape` |
| 12 | `database_corrupt`, `migration_checksum_mismatch`, `record_hash_mismatch` |
| 13 | `database_locked`, `publication_failed`, `rollback_failed` |
| 14 | `mcp_protocol_error` |
| 15 | `external_command_failed`, `timeout` |

## Sobre la recuperabilidad

Un código no recuperable señala un estado que el usuario no puede corregir
reintentando: exige intervención explícita.
