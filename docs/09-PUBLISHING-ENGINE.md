# Motor de publicación

## Objetivo

Convertir una decisión aprobada en un cambio mínimo, reversible y verificable.

## Destinos

### Project knowledge

```text
.royo-learn/knowledge/<topic>.md
```

### Shared knowledge

Raíz configurada explícitamente:

```text
<shared_root>/knowledge/<topic>.md
```

### Skills

```text
<skills_root>/<skill-name>/SKILL.md
```

### AGENTS.md

Solo bloque administrado:

```markdown
<!-- royo-learn:<learning-id>:start -->
- Regla aprobada.
<!-- royo-learn:<learning-id>:end -->
```

### Test

Archivo o patch aprobado bajo el proyecto.

## Resolución del destino

Orden:

1. path exacto de curación;
2. registro de Skills;
3. búsqueda exacta por `name` de frontmatter;
4. error por ambigüedad.

Nunca crear una segunda Skill con el mismo nombre para evitar decidir.

## Operaciones

### `create`

Falla si el archivo existe.

### `replace`

Solo permitido para artifacts completamente administrados por royo-learn.

### `replace_managed_block`

Solo modifica el bloque con ID exacto.

### `apply_unified_patch`

- validar patch;
- aplicar en worktree temporal o buffer;
- rechazar offsets ambiguos;
- mostrar resultado.

## Preview

Debe incluir:

```json
{
  "learning_id": "...",
  "targets": [{
    "path": "...",
    "operation": "...",
    "before_sha256": "...",
    "after_sha256": "...",
    "diff": "..."
  }],
  "verification": [],
  "requires_approval": true,
  "risk": "high",
  "preview_hash": "..."
}
```

Hash calculado sobre representación canónica que incluye:

- learning ID;
- target roots;
- paths;
- operaciones;
- before/after hashes;
- comandos de verificación;
- política.

## Políticas de aprobación

Siempre requieren humano:

- `AGENTS.md`;
- shared root;
- archivos fuera de `.royo-learn/`;
- actualización de Skill existente;
- ejecución de comandos de verificación no allowlisted;
- más de un archivo;
- eliminación;
- target con cambios no committeados.

Config puede permitir sin aprobación:

- nuevo knowledge file local;
- record materialization;
- índice local.

## Escritura atómica

Por archivo:

1. leer y hash;
2. crear backup;
3. escribir temp en mismo filesystem;
4. fsync;
5. rename atómico;
6. verificar hash;
7. continuar.

Para múltiples archivos:

- staging directory;
- preflight completo;
- aplicar;
- si uno falla, rollback de todos;
- auditar cada paso.

## Verificación

Comandos como arrays:

```yaml
command: ["go", "test", "./..."]
timeout_seconds: 300
working_directory: "."
allow_failure: false
```

No shell.

Allowlist configurable:

```text
go test
go vet
npm test
pnpm test
pytest
gentle-ai skill-registry refresh
```

Una Skill requiere además:

- YAML válido;
- `name` único;
- `description` no vacía;
- ruta correcta;
- sin secretos;
- registro actualizado cuando disponible.

## Rollback

Guardar:

- before bytes o blob hash;
- after hash;
- timestamp;
- target path.

Rollback automático solo si el target conserva `after_sha256`. Si divergió, producir patch de reversión y requerir intervención humana.

## Concurrencia

- lock por proyecto;
- lock por target;
- timeout;
- PID y timestamp;
- recuperación de lock huérfano segura;
- SQLite transaction + filesystem journal.

## Archivos sucios

Si el target tiene cambios Git no committeados:

- preview permitido;
- publish bloqueado por defecto;
- override humano explícito;
- backup obligatorio;
- auditoría con `dirty_override=true`.
