# Catálogo de errores

Cada error debe tener:

```json
{
  "code": "string",
  "message": "human readable",
  "recoverable": true,
  "details": {},
  "next_action": "..."
}
```

Códigos mínimos:

```text
invalid_argument
invalid_config
project_not_found
ambiguous_project
unknown_project
learning_not_found
invalid_transition
duplicate_learning
evidence_missing
evidence_too_large
secret_detected
path_outside_root
symlink_escape
protected_path
target_ambiguous
target_changed
dirty_target
approval_required
approval_invalid
approval_expired
preview_not_found
preview_hash_mismatch
publication_conflict
verification_failed
rollback_conflict
database_locked
database_corrupt
migration_checksum_mismatch
record_hash_mismatch
engram_unavailable
engram_ambiguous_project
gentle_ai_unavailable
skill_registry_failed
mcp_protocol_error
payload_too_large
external_command_failed
timeout
experience_commit_unknown
```

Todos deben mapear a CLI exit code y MCP envelope.
