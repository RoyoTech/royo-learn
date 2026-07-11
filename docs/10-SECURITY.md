# Seguridad y privacidad

## Threat model

Amenazas:

- prompt injection que intenta publicar reglas;
- path traversal;
- symlink escape;
- ejecución arbitraria;
- exfiltración por Engram;
- secretos dentro de diff/log;
- aprobación reutilizada con otro contenido;
- carrera entre preview y apply;
- corrupción de Skills;
- edición de archivos administrados por terceros;
- stdout contaminado en MCP;
- payloads enormes;
- DB corrupta;
- dependencia comprometida.

## Controles

### Path security

- canonicalizar ruta;
- comprobar raíz permitida;
- resolver symlinks existentes;
- validar padre de paths nuevos;
- bloquear `..`;
- bloquear UNC/remotos por defecto;
- bloquear device files;
- bloquear `.git/`, `.ssh/`, `.env`, credentials y claves;
- allowlist para shared root.

### Redacción

Detectar al menos:

- OpenAI/Anthropic/GitHub/AWS tokens;
- private keys;
- bearer tokens;
- passwords en URL;
- `.env` assignment;
- cookies;
- connection strings;
- tags `<private>`.

Reemplazar por `[REDACTED:<type>]`, conservar hash no reversible del fragmento para deduplicación.

### Command execution

- `exec.CommandContext`;
- argumentos separados;
- cwd validado;
- environment mínimo;
- timeout;
- stdout/stderr limitado;
- no shell;
- allowlist;
- registrar exit code y hash;
- bloquear comandos destructivos.

### MCP

- sin logs stdout;
- límites de tamaño;
- validación estricta;
- tool annotations;
- errores no filtran paths sensibles salvo modo debug;
- debug nunca activo por defecto.

### Approval integrity

- preview hash;
- expiración;
- actor;
- reason;
- audit;
- invalidación ante cualquier cambio;
- publicación shared y AGENTS siempre prompt.

### Files managed by others

Detectar marcadores o manifests de Gentle-AI. Si no se puede demostrar propiedad, bloquear reemplazo completo.

### Dependency security

CI:

```bash
go mod verify
go vet ./...
govulncheck ./...
```

Pin de GitHub Actions por SHA en implementación final.

### Data retention

Defaults:

- records: permanente;
- audit: permanente;
- evidence blobs: 180 días si no respaldan publicación;
- command raw output: 30 días;
- secrets redacted: nunca persistir original.

### Telemetría

Ninguna telemetría remota en v1.

## Pruebas de seguridad

- traversal Windows/Linux;
- symlink attack;
- changed-target after approval;
- command injection;
- secret fixtures;
- malicious YAML;
- oversized payload;
- database locking;
- tampered migration;
- tampered record hash;
- MCP stdout pollution;
- AGENTS block collision.
