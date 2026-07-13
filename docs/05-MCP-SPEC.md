# Especificación MCP

## Transporte

- `stdio` obligatorio;
- SDK oficial MCP para Go;
- `stdout` solo protocolo;
- logs a `stderr`;
- instrucciones del servidor con los límites críticos en los primeros 512 caracteres;
- tool names compatibles con `^[a-zA-Z0-9_]{1,64}$`.

## Instrucciones del servidor

El bloque que comienza con `Prerequisite:` debe empezar antes del índice de
carácter 512, medido desde cero en puntos de código Unicode, y debe aparecer
antes de `All tool outputs...`. Ese bloque debe indicar que:

- `royo-learn init --project-root <root>` es obligatorio una vez por cada raíz
  de proyecto independiente;
- el marcador se descubre recorriendo directorios superiores desde un
  subdirectorio;
- `royo-learn setup install` es opcional después de `init` y nunca crea el
  almacén;
- `project_not_found` se corrige ejecutando `init` para la raíz prevista.

Texto base:

> Use royo-learn to turn verified project experience into reusable learning. Search before capturing. Capture does not publish. Curate requires evidence. Preview before writes. AGENTS.md and shared changes always require human approval. Never include secrets or private chain-of-thought.

## Tools v1

### `learning_capture`

Read/write.

Input resumido:

```json
{
  "project_root": "/repo",
  "title": "Validate rendered PDF page numbers",
  "type": "prevention",
  "context": "...",
  "observation": "...",
  "reusable_lesson": "...",
  "recommended_procedure": ["..."],
  "limits": "...",
  "scope_guess": "shared",
  "confidence": "high",
  "evidence_level": "reproduced",
  "proposed_destination": "skill_update",
  "retrieval_terms": ["pdf", "page number"],
  "evidence": [],
  "idempotency_key": "session/task/lesson",
  "actor": {}
}
```

Output:

```json
{
  "learning": {},
  "created": true,
  "similar": [],
  "engram": {"available": true, "references": []},
  "next_action": "curate"
}
```

### `learning_search`

Read-only.

Input:

```json
{
  "query": "database migration",
  "project_root": "/repo",
  "all_projects": false,
  "include_engram": true,
  "limit": 10
}
```

### `learning_get`

Read-only.

### `learning_list`

Read-only.

### `learning_curate`

Write.

Debe incluir decisión completa; no admite campos ambiguos.

```json
{
  "learning_id": "...",
  "decision": "approve_skill_update",
  "rationale": "...",
  "relation": {
    "type": "extends",
    "target_id": "..."
  },
  "destination": {
    "type": "skill",
    "path": "skills/database-migrations/SKILL.md",
    "action": "update"
  },
  "validation": [],
  "acceptance_checks": [],
  "rollback_condition": "...",
  "actor": {}
}
```

### `learning_publication_preview`

Read-only respecto al destino; puede persistir el preview.

```json
{
  "learning_id": "...",
  "publication": {
    "operation": "replace_managed_block",
    "target_path": "AGENTS.md",
    "content": "...",
    "managed_block_id": "learning-..."
  }
}
```

Output incluye `preview_hash`.

### `learning_approve`

Write.

Solo un humano o un agente que represente una aprobación explícita del usuario. El tool schema debe exigir `approval_evidence`:

```json
{
  "learning_id": "...",
  "preview_hash": "...",
  "approved_by": "RoyoTech",
  "reason": "Explicit user approval in current session",
  "approval_evidence": "user-message-reference"
}
```

### `learning_publish`

Destructive/write.

MCP annotations deben marcarlo como escritura. El cliente debe poder solicitar aprobación.

Input:

```json
{
  "learning_id": "...",
  "preview_hash": "...",
  "approval_id": "...",
  "apply": true
}
```

### `learning_report_occurrence`

Write.

### `learning_status`

Read-only.

Devuelve métricas y estado de integraciones.

### `learning_doctor`

Read-only por defecto; `fix_safe` opcional.

## Errores MCP

Siempre usar respuesta estructurada:

```json
{
  "error": {
    "code": "approval_required",
    "message": "Shared publication requires human approval.",
    "details": {
      "learning_id": "...",
      "preview_hash": "..."
    },
    "recoverable": true,
    "next_action": "Call learning_approve after showing the preview to the user."
  }
}
```

## Límites

- request máximo por defecto: 512 KiB;
- response máximo objetivo: 768 KiB;
- evidencia grande se referencia, no se incrusta;
- timeout default de tool: 60 s;
- publicación y verificaciones: configurable hasta 10 min;
- paginación cursor para list/search;
- cancelación por context.

## Tests MCP

- initialize/list tools;
- `Prerequisite:` comienza antes del carácter 512 y precede a `All tool outputs...`;
- esquemas válidos;
- todas las tools invocables;
- no stdout ajeno;
- cancelación;
- payload límite;
- tool annotations;
- error estructurado;
- compatibilidad con Codex;
- prueba con MCP Inspector.
