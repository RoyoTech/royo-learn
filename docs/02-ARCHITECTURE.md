# Arquitectura

## 1. Contexto del sistema

```text
┌──────────────────────────────────────────────────────┐
│ Codex / OpenCode / Claude / Pi / otro cliente MCP   │
│                                                      │
│ Skills: capture · curate · publish                  │
└─────────────────────┬────────────────────────────────┘
                      │ MCP stdio
                      ▼
┌──────────────────────────────────────────────────────┐
│ royo-learn                                           │
│                                                      │
│ CLI · MCP · dominio · aprobación · publicación      │
│ evidencia · recurrencia · auditoría · búsqueda      │
└──────────────┬──────────────────────┬────────────────┘
               │                      │
               ▼                      ▼
       SQLite + FTS5             Filesystem/Git
       estado operacional        records y destinos
               │
               └──────────┬───────────┘
                          ▼
                 Engram HTTP opcional
                 127.0.0.1:7437
```

## 2. Responsabilidades

### Agente + Skills

- interpretar la experiencia;
- redactar la lección;
- decidir si es generalizable;
- aportar relaciones semánticas;
- redactar contenido de una Skill;
- proponer criterios de verificación;
- pedir aprobación cuando corresponda.

### Binario

- validar contratos;
- resolver proyecto;
- recolectar evidencia determinista;
- redactar secretos;
- persistir y versionar;
- buscar lexicalmente;
- impedir transiciones inválidas;
- generar previews;
- verificar aprobación;
- publicar atómicamente;
- ejecutar verificaciones permitidas;
- revertir;
- auditar;
- medir reincidencias.

## 3. Componentes internos

```text
cmd/royo-learn
    bootstrap CLI/MCP y composición de dependencias

internal/domain
    entidades, enums, transiciones y errores

internal/config
    config global/proyecto, defaults y validación

internal/project
    resolución segura de raíz y project key

internal/storage
    SQLite, migraciones, repositorios y transacciones

internal/search
    FTS5, normalización y ranking lexical

internal/evidence
    recolección, hashing, límites y redacción

internal/gitx
    git root, status, diff, commit y branch

internal/engram
    cliente HTTP local opcional, health, search, save

internal/capture
    creación idempotente y relaciones candidatas

internal/approval
    previews, tokens, expiración y políticas

internal/publish
    planning, patch, atomic write, backup, verify, rollback

internal/recurrence
    fingerprint y métricas

internal/audit
    eventos append-only y export

internal/mcpserver
    tools, schemas, middleware y stdio

internal/validate
    Skill frontmatter, Markdown, paths y config
```

## 4. Flujo de captura

```text
learning_capture
   │
   ├─ resolver proyecto
   ├─ validar payload
   ├─ redactar
   ├─ normalizar y hash
   ├─ comprobar idempotencia
   ├─ buscar lexicalmente
   ├─ recolectar evidencia permitida
   ├─ persistir transacción
   ├─ materializar record Markdown
   ├─ opcional: guardar referencia breve en Engram
   └─ responder candidato + posibles similares
```

## 5. Flujo de publicación

```text
approved learning
   ↓
publication_preview
   ↓
resolver destino canónico
   ↓
validar operación y ruta
   ↓
crear diff + verification plan + preview hash
   ↓
approval policy
   ├─ no requiere → publish
   └─ requiere → learning_approve
                    ↓
              learning_publish
                    ↓
          revalidar preview hash
                    ↓
          backup + atomic write
                    ↓
             verification commands
                    ↓
          success → published
          failure → rollback + blocked
```

## 6. Integración con Engram

Usar la API HTTP local, nunca la base SQLite.

Operaciones v1:

- `GET /health`
- `GET /search`
- `GET /context`
- `POST /observations` opcional
- `GET /doctor` opcional

El sistema no depende de que Engram esté ejecutándose. Si no está disponible:

```json
{
  "engram": {
    "available": false,
    "degraded": true,
    "reason": "connection_refused"
  }
}
```

## 7. Integración con Gentle-AI

No hay enlace binario interno. La integración es por convenciones:

- Skills bajo raíces que el registro pueda descubrir;
- ejecutar `gentle-ai skill-registry refresh --force` después de cambios de Skills;
- consultar `.atl/skill-registry.md` para resolver una Skill canónica;
- nunca modificar archivos administrados por el instalador;
- `gentle-ai doctor` como verificación opcional.

## 8. Almacenamiento dual

### SQLite

Fuente operacional:

- estado;
- consultas;
- relaciones;
- auditoría;
- aprobaciones;
- publicaciones;
- recurrencias.

### Records Markdown

Fuente auditable y portable:

```text
.royo-learn/records/<learning-id>.md
```

Cada materialización incluye `record_hash`. La DB puede reconstruirse desde records mediante `royo-learn rebuild-index`.

No almacenar diffs enormes dentro del Markdown. Evidencias grandes se guardan por hash en:

```text
.royo-learn/evidence/sha256/<prefix>/<hash>
```

## 9. Configuración

Precedencia:

1. flags CLI / argumentos MCP;
2. `.royo-learn/config.yaml` del proyecto;
3. configuración de usuario;
4. defaults compilados.

Ninguna configuración de proyecto puede habilitar escritura fuera de raíces confiables sin consentimiento del usuario.

## 10. Límites de arquitectura

- No hay daemon obligatorio.
- MCP `stdio` abre DB por proceso.
- SQLite en WAL.
- No hay auto-capture de conversaciones.
- No hay publicación por “confianza” del modelo.
- No hay acceso a memoria privada de otros agentes.
- Se comparten únicamente artefactos estructurados.
