# Operación

## Rutina diaria

El usuario no debería ejecutar comandos normalmente. Las Skills llaman al MCP.

Comandos útiles:

```bash
royo-learn review
royo-learn status
royo-learn doctor
royo-learn search "tema"
```

## Backup

- DB: snapshot SQLite consistente;
- records y knowledge: Git;
- evidence blobs: opcional;
- shared library: Git.

## Restore

1. restaurar records;
2. `royo-learn rebuild-index`;
3. verificar hashes;
4. importar audit backup si corresponde.

## Mantenimiento

```bash
royo-learn doctor
royo-learn db vacuum
royo-learn evidence prune --dry-run
royo-learn review --stale
```

## Observabilidad

Logs JSON opcionales a stderr:

```json
{
  "time": "...",
  "level": "info",
  "component": "publish",
  "operation": "apply",
  "learning_id": "...",
  "duration_ms": 42
}
```

Nunca loguear contenido completo por defecto.

## Políticas de ineficacia

Un aprendizaje puede marcarse `needs_review` cuando:

- tiene dos reincidencias después de publicación;
- fue recuperado pero no evitó el error;
- la Skill no se activó repetidamente;
- una verificación dejó de ser válida;
- el target fue eliminado.

No debe despublicarse automáticamente.

## Compatibilidad

`royo-learn version --json` expone:

- schema version;
- MCP SDK version;
- DB migration level;
- record format version.
