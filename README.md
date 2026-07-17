# Agent Royo Learn

> **Royo-Learn es un motor local de aprendizaje operacional para agentes.** El
> agente identifica y estructura la experiencia; Royo-Learn conserva evidencia,
> controla su gobernanza y convierte aprendizajes aprobados en cambios
> verificables, auditables y reversibles.
>
> — Frase final del producto (`docs/PLAN-recuperacion-contrato.md`).

Binario único: `royo-learn` (`royo-learn.exe` en Windows). Servidor MCP
principal por `stdio`. Sin proveedor LLM embebido. Sin base vectorial. Sin red
obligatoria. Local-first, multiplataforma.

## Qué problema resuelve

Los agentes resuelven problemas en sesiones cortas y luego olvidan. El usuario
corrige el mismo comportamiento tres veces. Una solución queda enterrada en una
conversación que ya nadie lee. `AGENTS.md` se satura porque no hay
clasificación. Una memoria puntual se aplica después como si fuera una regla
universal, sin que nadie la aprobara.

Royo-Learn parte de una idea simple: **recordar no basta**. Para que una
experiencia cambie el comportamiento futuro tiene que pasar por un ciclo
auditable —capturar, evidenciar, aprobar, publicar, verificar, revertir— y
quedar registrada en un lugar al que el agente pueda volver con contexto.

No reemplaza a Gentle-AI ni a Engram:

| Sistema | Qué hace | Qué **no** hace |
|---|---|---|
| **Gentle-AI** | Configura agentes, Skills, workflows, MCP. | No persiste memoria de sesiones; no audita publicaciones. |
| **Engram** | Memoria persistente de sesiones, decisiones, descubrimientos, errores. | No valida contratos; no aplica cambios al sistema del usuario. |
| **Royo-Learn** | Convierte experiencias verificadas en cambios auditables del comportamiento (conocimiento, Skills, reglas, tests, alertas de recurrencia). | No entiende conversaciones por sí solo; no decide por su cuenta que una observación es una regla; no aprueba en nombre del usuario. |

Royo-Learn funciona aunque Gentle-AI o Engram estén ausentes: la integración
con ambos es opcional y degradable de forma observable (`degraded: true` en el
`doctor`).

## Qué hace cada parte

### El LLM y la Skill (el lado semántico)

La responsabilidad del modelo y de la Skill es **proponer**, no decidir:

- interpretar la experiencia;
- identificar el patrón reusable;
- estructurar el candidato (título, contexto, observación, lección reusable,
  procedimiento, límites, evidencia, alcance, destino propuesto, término de
  búsqueda);
- proponer relaciones semánticas con aprendizajes existentes
  (`duplicate_of`, `extends`, `supersedes`, `contradicts`, `narrows`,
  `related`);
- redactar el contenido completo o el parche de una Skill;
- proponer criterios de verificación.

El LLM **no aprueba** y **no publica**. Solo propone.

### Royo-Learn (el lado operacional)

El binario **valida, persiste y gobierna**:

- valida el contrato del payload (esquemas, tipos, alcance);
- redacta secretos **antes** de cualquier persistencia (DB, blob store,
  Markdown, audit log, respuesta CLI/MCP);
- normaliza y hashea para deduplicación;
- busca lexicalmente en FTS5;
- recolecta evidencia permitida (archivo, commit, diff Git, comando y
  resultado, prueba, Engram observation ID, issue/PR, texto breve);
- persiste de forma transaccional en SQLite;
- materializa un record Markdown auditable en
  `.royo-learn/records/<learning-id>.md`;
- garantiza la máquina de estados (`captured → needs_evidence → approved →
  published`, etc., sin saltos);
- exige aprobación humana cuando el destino lo requiere;
- aplica publicaciones de forma atómica con backup y rollback;
- ejecuta verificaciones post-aplicación;
- audita cada acción relevante en log append-only;
- mide reincidencias.

### Royo-Learn — qué NO hace

- No comprende conversaciones por sí solo.
- No decide automáticamente que una observación es una regla.
- No aprueba en nombre del usuario.
- No sustituye a Engram (es un complemento opcional).
- No necesita un proveedor LLM interno.
- No usa embeddings ni base vectorial en v1.
- No garantiza funciones no demostradas (la frase final del producto es la
  verdad verificable; el resto del README apunta a pruebas que la sostienen).

## Ciclo de un aprendizaje

```text
experiencia
    ↓
learning_capture        (Skill redacta candidato idempotente)
    ↓
búsqueda previa + FTS5
    ↓
learning_add_evidence   (si el sistema lo requiere)
    ↓
learning_curate         (approve | reject | needs_evidence | merge)
    ↓
learning_publication_preview   (diff + hash + riesgos + verificación)
    ↓
learning_approve        (humano, ligado al preview hash)
    ↓
learning_publish        (backup → write atómico → verificación → published)
    ↓
learning_report_occurrence     (cuando el patrón reaparece)
    ↓
learning_compute_metrics       (tasa de recuperación, reincidencias)
```

Estados válidos (`docs/03-DOMAIN-MODEL.md`, `docs/01-PRD.md` RF-005):

```text
captured → needs_evidence → approved → published → superseded | archived
                    │           │
                    ↓           ↓
                 rejected   superseded
                    │
                    ↓
                 merged
```

Fuera de esta máquina, no hay transición.

Destinos posibles de publicación (`RF-008`):

- conocimiento de proyecto;
- conocimiento compartido;
- Skill nueva o actualización de Skill;
- regla administrada en `AGENTS.md` (exige aprobación humana);
- archivo de prueba o regresión;
- ninguno (candidato informativo sin publicación).

## Comandos CLI

Todos los comandos aceptan `--json` con salida estable. Códigos de salida
documentados en `docs/04-CLI-SPEC.md`:

| Código | Significado |
|---|---|
| `0` | éxito |
| `1` | fallo de dominio |
| `2` | argumentos inválidos |
| `4` | proyecto no encontrado |
| `5` | proyecto ambiguo |

Comandos disponibles hoy (verificados contra `cmd/royo-learn/main.go`):

```text
version           imprime versión
init              inicializa un proyecto (.royo-learn/)
doctor            diagnóstico de salud (DB, Git, Engram, rutas, registros)
capture           captura un aprendizaje (idempotente)
evidence add      adjunta evidencia a un aprendizaje
curate            aprobar / rechazar / pedir evidencia / fusionar
get               obtiene un aprendizaje por ID
search            búsqueda léxica (FTS5)
occurrence        registra una reincidencia
recurrences       lista reincidencias
metrics           métricas de un aprendizaje
preview           previsualiza una publicación (diff + hash + plan)
approve           aprueba un preview (humano)
publish           publica (dry-run por defecto, --apply para escribir)
rollback          revierte una publicación
mcp-serve         arranca el servidor MCP por stdio (--tools read|agent|admin)
engram-health     comprueba el estado de Engram
engram-search     busca en Engram (degradación observable si no está)
setup             install | uninstall | status | upgrade-skills
self-update       actualiza el binario (--check | --version vX.Y.Z)
e2e               ejecuta el escenario end-to-end (--temp)
```

Perfiles MCP (`--tools read|agent|admin`, default `agent`):

- `read` — solo lectura: `learning_search`, `learning_get`, `learning_list`,
  `learning_status`, `learning_doctor`.
- `agent` (default) — ciclo completo: añade `learning_capture`,
  `learning_add_evidence`, `learning_curate`,
  `learning_publication_preview`, `learning_approve`, `learning_publish`,
  `learning_report_occurrence`, `learning_list_recurrences`,
  `learning_compute_metrics`.
- `admin` — añade `learning_rollback` (destructivo).

Los nombres `--profile minimal|standard|full` de v0.1.9 siguen funcionando
como alias deprecated; se eliminan en v0.2.0.

## Servidor MCP

Una vez inicializado el proyecto, el binario expone el servidor por `stdio`:

```bash
# Codex CLI
codex mcp add royo-learn -- royo-learn mcp-serve

# Claude Code / OpenCode — agregar a la config MCP del cliente
{
  "mcpServers": {
    "royo-learn": {
      "command": "royo-learn",
      "args": ["mcp-serve"],
      "env": {}
    }
  }
}
```

Convenciones:

- `stdout` queda reservado para mensajes MCP; los logs van a `stderr`.
- Las herramientas de escritura (`learning_publish`, `learning_rollback`)
  están marcadas como `write`/`destructive`.
- Cada respuesta conserva `code`, `recoverable`, `details`, `next_action` y
  la ruta del artefacto de recuperación cuando aplica.
- Sin telemetría. Sin red obligatoria.

Las Skills que coordinan el ciclo viven en `skills/` y se distribuyen
opcionalmente con `royo-learn setup install --agent <claude-code|codex|opencode|all>`.

## Instalación

### Requisitos

- Windows, Linux o macOS (incluye WSL).
- Sin dependencias externas en runtime: el binario es estático, sin CGO.
- Acceso a `https://github.com/RoyoTech/royo-learn/releases` para descargar.

### Windows (PowerShell 5.1+)

```powershell
irm https://raw.githubusercontent.com/RoyoTech/royo-learn/main/install.ps1 | iex
```

El binario queda en `%LOCALAPPDATA%\royo-learn\bin\royo-learn.exe` y se
agrega al `PATH` de usuario.

Versión específica o desinstalación:

```powershell
.\install.ps1 -Version v0.1.10
.\install.ps1 -Uninstall
```

### Linux, macOS, WSL (bash)

```bash
curl -fsSL https://raw.githubusercontent.com/RoyoTech/royo-learn/main/install.sh | bash
```

El binario queda en `~/.local/bin/royo-learn`. Para sistemas donde
`~/.local/bin` no esté en el `PATH`, agregarlo manualmente.

Versión específica o desinstalación:

```bash
./install.sh --version v0.1.10
./install.sh --uninstall
```

> **Importante:** el script de PowerShell **no** corre en Git Bash, MSYS ni
> Cygwin. En WSL usá la versión bash. El script de bash detecta esos entornos
> y aborta con instrucciones para PowerShell.

### Auto-actualización

Una vez instalado:

```bash
royo-learn self-update --check           # consulta sin descargar
royo-learn self-update                   # actualiza al último release
royo-learn self-update --version vX.Y.Z  # actualiza a una versión específica
```

`--check` y `--version` son mutuamente excluyentes. Si `GITHUB_TOKEN` está
definido, `self-update` lo envía como Bearer para evitar el rate limit de la
API.

### Compilar desde el código

Requisitos: Go 1.25+.

```bash
git clone https://github.com/RoyoTech/royo-learn.git
cd royo-learn
make build         # binario local
make build-all     # cross-compile windows/linux/darwin × amd64/arm64
make quality       # fmt + vet + test -race + build
make test          # go test -race ./...
```

El target `build-all` produce en `dist/`:

```text
royo-learn-windows-amd64.exe
royo-learn-linux-amd64
royo-learn-linux-arm64
royo-learn-darwin-amd64
royo-learn-darwin-arm64
```

## Inicio rápido

```bash
# 1. Inicializar el proyecto (una vez por raíz)
royo-learn init --project-root /ruta/al/proyecto

# 2. Diagnóstico
royo-learn doctor --project-root /ruta/al/proyecto --json

# 3. Capturar un aprendizaje
royo-learn capture \
  --project-root /ruta/al/proyecto \
  --title "Connection pool exhaustion" \
  --context "production deploy" \
  --observation "pool exhausted at 100 concurrent" \
  --lesson "set max_connections based on memory budget" \
  --type procedure \
  --scope project \
  --json

# 4. Adjuntar evidencia si el sistema lo requiere
royo-learn evidence add <learning-id> \
  --summary "load test reproduces the fix" \
  --content "..." \
  --json

# 5. Buscar antes de repetir
royo-learn search "connection pool" --json

# 6. Curar
royo-learn curate \
  --learning-id <learning-id> \
  --action approve \
  --rationale "validated with load testing" \
  --json

# 7. Preview (devuelve hash y si requiere aprobación humana)
royo-learn preview \
  --learning-id <learning-id> \
  --json

# 8. Aprobar (humano, ligado al hash del preview)
royo-learn approve <learning-id> \
  --preview-hash <preview-hash> \
  --approved-by "<identidad>" \
  --reason "revisado y validado" \
  --json

# 9. Publicar (dry-run por defecto; --apply para escribir)
royo-learn publish \
  --learning-id <learning-id> \
  --preview-hash <preview-hash> \
  --approval-id <approval-id> \
  --apply \
  --json

# 10. Revertir si algo salió mal
royo-learn rollback \
  --journal-id <id-de-publicacion> \
  --json

# 11. Registrar reincidencia y medir
royo-learn occurrence --learning-id <learning-id> --outcome prevented --json
royo-learn recurrences --learning-id <learning-id> --json
royo-learn metrics --learning-id <learning-id> --json
```

## El ciclo completo, de principio a fin

El inicio rápido lista los comandos. Esta sección los encadena en un caso
real para mostrar **qué garantiza el binario en cada paso**. Los nombres de
campo y los valores de retorno son los que devuelve el código (verificados
contra `cmd/royo-learn/main.go` y `internal/mcpserver/profiles.go`). Donde
una salida pueda variar entre ejecuciones, se marca con `<…>`.

### Escenario

Maria aplica la migración `0047_add_user_prefs.sql` en producción. El
framework la aplica, pero el chequeo de idempotencia del ORM falla porque
la columna ya existía de un intento previo abortado. La base queda en un
estado inconsistente. Media mañana perdida en revertir a mano. Maria
decide que esto no puede pasar de nuevo, ni a ella en seis meses ni a
nadie del equipo.

Royo-Learn existe para que una corrección explicada una vez se convierta
en un cambio **verificable, auditable y reversible** del comportamiento
del proyecto.

### Acto 1 — Inicialización (una vez por raíz)

El sistema no existe hasta que el proyecto decide crearlo. Cada raíz de
proyecto tiene su propio `.royo-learn/` con DB, records y backups.

```bash
$ royo-learn init --project-root ~/code/mi-app --json
{
  "status": "initialized",
  "project_root": "~/code/mi-app",
  "store": "~/code/mi-app/.royo-learn/store.db",
  "config": "~/code/mi-app/.royo-learn/config.yaml",
  "next_action": "run \"royo-learn doctor --project-root ~/code/mi-app --json\""
}

$ royo-learn doctor --project-root ~/code/mi-app --json
{
  "status": "ok",
  "components": {
    "database":          { "ok": true,  "migrations": "v3" },
    "git":               { "ok": true },
    "engram":            { "ok": false, "available": false, "degraded": true, "reason": "connection_refused" },
    "skill_registry":    { "ok": false, "available": false }
  }
}
```

> Nota: `engram.available = false` no es un fallo. El sistema reporta
> degradación explícita y sigue operativo.

### Acto 2 — Captura (la LLM redacta, el binario valida)

Maria le dice a su agente: *"Guardá esto: nunca aplicar una migración sin
verificar primero su estado real en la base"*. El agente redacta el
candidato, llama al binario, y el binario valida, redacta secretos, hashea,
busca en FTS5 y persiste. **Una sola llamada, una sola garantía.**

```bash
$ royo-learn capture \
    --project-root ~/code/mi-app \
    --title "Migrations must be idempotent and state-checked" \
    --context "Production incident: migration 0047 re-ran on a half-applied DB" \
    --observation "ORM idempotency check failed mid-transaction; user_prefs table left inconsistent; manual rollback took half a day." \
    --lesson "Always query schema_migrations before running make migrate; refuse to apply if state diverges from expected." \
    --type procedure \
    --scope project \
    --destination agents_rule \
    --idempotency-key "maria-2026-07-17-migration-state" \
    --json
{
  "learning_id": "<ULID>",
  "status": "needs_evidence",
  "fingerprint": "<sha256>",
  "similar": [
    {
      "learning_id": "<ULID previo>",
      "title": "Check migration state before applying",
      "status": "rejected",
      "match": "lexical"
    }
  ],
  "next_action": "run \"royo-learn evidence add <learning-id> --summary ...\""
}
```

Cuatro cosas pasaron en esa sola llamada, sin que la LLM tuviera que
recordarlas:

1. **Validación de contrato** (campos requeridos, tipos, alcance).
2. **Redacción de secretos** — si Maria hubiera pegado una contraseña en
   `--context`, se reemplaza por `[REDACTED]` **antes** de tocar SQLite,
   el blob store, el Markdown, el audit log y esta misma respuesta.
3. **Hash normalizado** + búsqueda en FTS5 — encontró un candidato
   rechazado previamente. No se duplica.
4. **Estado inicial**: `needs_evidence`, porque la regla propuesta
   (`agents_rule`) exige evidencia antes de poder aprobarse.

### Acto 3 — Evidencia

Maria adjunta el diff del fallo, el log del ORM y el fix manual. El
estado **se mantiene** en `needs_evidence` hasta que la curación decida
que es suficiente.

```bash
$ royo-learn evidence add <learning-id> \
    --summary "Failed migration run + manual fix diff" \
    --content "$(cat incident-0047.diff)" \
    --json
{
  "learning_id": "<ULID>",
  "evidence_id": "<ULID>",
  "evidence_count": 1,
  "status": "needs_evidence",
  "next_action": "run \"royo-learn evidence add <learning-id> --summary ...\" or run \"royo-learn curate --learning-id <learning-id> --action approve\""
}
```

### Acto 4 — Curación

Maria revisa el candidato, ve que la evidencia alcanza, y aprueba con
fundamento. Recién acá el estado pasa a `approved`.

```bash
$ royo-learn curate \
    --learning-id <learning-id> \
    --action approve \
    --rationale "Validated against incident-0047; same pattern would prevent the next half-applied migration." \
    --json
{
  "learning_id": "<ULID>",
  "status": "approved",
  "previous_status": "needs_evidence",
  "actor": { "kind": "human", "name": "cli-user" },
  "next_action": "run \"royo-learn preview --learning-id <learning-id>\""
}
```

`--action` también acepta `reject`, `needs_evidence` (volver a pedir más),
`relate` (vincular con otro existente), `merge` (fusionar con duplicado) y
`approve_new_skill` / `approve_skill_update` para destinos de Skill.

### Acto 5 — Preview de publicación

**El sistema nunca escribe sin preview.** Antes de tocar un archivo del
proyecto, muestra exactamente qué va a cambiar, dónde, y qué verificaciones
correrá después. El preview lleva un hash SHA-256: cualquier cambio
invalida aprobaciones previas.

```bash
$ royo-learn preview --learning-id <learning-id> --json
{
  "learning_id": "<ULID>",
  "destination": "agents_rule",
  "targets": [
    {
      "path": "AGENTS.md",
      "operation": "append_section",
      "preview_sha256": "<sha256 del diff>",
      "diff_excerpt": "+## Migrations\n+- Always query schema_migrations before running make migrate.\n+- Refuse to apply if state diverges from expected.\n"
    }
  ],
  "requires_approval": true,
  "verification_commands": [
    "git diff -- AGENTS.md",
    "git status --porcelain"
  ],
  "rollback_plan": {
    "strategy": "restore_backup",
    "backup_path": ".royo-learn/backups/<id>.bak"
  },
  "next_action": "run \"royo-learn approve --learning-id <learning-id> --preview-hash <hash> --approved-by maria\""
}
```

`requires_approval: true` aparece porque el destino `agents_rule` (escribir
una regla en `AGENTS.md` compartido) está marcado como sensible. Maria no
puede saltarse este paso.

### Acto 6 — Aprobación humana

La aprobación está **ligada al hash exacto del preview**. Si Maria
recalcula el preview mañana porque cambió una palabra, el hash cambia y
la aprobación vieja queda inválida.

```bash
$ royo-learn approve \
    --learning-id <learning-id> \
    --preview-hash <preview_sha256> \
    --approved-by maria \
    --reason "Reviewed the incident diff and the proposed rule; ship it." \
    --json
{
  "approval_id": "<ULID>",
  "learning_id": "<ULID>",
  "preview_hash": "<sha256>",
  "approved_by": "maria",
  "approved_at": "<RFC3339 UTC>",
  "next_action": "run \"royo-learn publish --learning-id <learning-id> --preview-hash <hash> --approval-id <id> --apply\""
}
```

### Acto 7 — Publicación atómica

**Dry-run por defecto.** Sin `--apply`, el binario solo reporta el plan.
Con `--apply`: backup → escritura atómica → verificación → `published`.
Si la verificación falla, revierte el contenido previo byte por byte y el
estado NO queda como `published`.

```bash
$ royo-learn publish \
    --learning-id <learning-id> \
    --preview-hash <preview_sha256> \
    --approval-id <approval_id> \
    --apply \
    --json
{
  "publication_id": "<ULID>",
  "learning_id": "<ULID>",
  "status": "published",
  "journal_id": "<ULID>"
}
```

`journal_id` es la llave para revertir. Si Maria descubre dentro de un
mes que la regla estaba mal redactada:

```bash
royo-learn rollback --journal-id <journal_id> --json
```

El binario restaura el contenido anterior desde el backup, marca la
publicación como `superseded`, y deja el aprendizaje en `approved` (no en
`published`). Listo para corregir y volver a publicar.

### Acto 8 — La regla vive

`AGENTS.md` ahora contiene, en una sección identificada:

<!-- prettier-ignore -->
```markdown
## Migrations

- Always query schema_migrations before running make migrate.
- Refuse to apply if state diverges from expected.
```

La próxima vez que cualquier agente de cualquier sesión lea
`AGENTS.md` antes de empezar a trabajar (que es la convención del
proyecto), va a leer esta regla.

### Acto 9 — Tres semanas después: la reincidencia

Un agente nuevo arranca una tarea sobre el mismo proyecto. Lee
`AGENTS.md`, ve la regla, y antes de correr `make migrate` ejecuta la
verificación. Detecta que el estado del ORM no coincide con la migración
esperada y **aborta con un mensaje claro** en vez de avanzar y romper la
base. Maria registra el incidente como reincidencia **prevenida**.

```bash
$ royo-learn occurrence \
    --learning-id <learning-id> \
    --outcome prevented \
    --json
{
  "learning_id": "<ULID>",
  "occurrence_id": "<ULID>",
  "outcome": "prevented",
  "rule_retrieved": true,
  "next_action": "run \"royo-learn metrics --learning-id <learning-id>\""
}

$ royo-learn metrics --learning-id <learning-id> --json
{
  "learning_id": "<ULID>",
  "occurrences": 1,
  "prevented": 1,
  "recurred": 0,
  "retrieval_rate": 1.0
}
```

**El ciclo cierra.** Una corrección explicada una vez se convirtió en un
cambio verificable del comportamiento, con auditoría completa y métrica
que demuestra que la regla sirvió.

### El mismo ciclo, vía MCP

Todos los pasos anteriores están disponibles como tools MCP con el perfil
`agent` (default) o `admin` (suma `learning_rollback`):

```text
learning_capture                  learning_publication_preview
learning_add_evidence             learning_approve
learning_curate                   learning_publish
learning_search                   learning_report_occurrence
learning_get                      learning_compute_metrics
learning_list                     learning_list_recurrences
learning_status                   learning_doctor
```

Mismas garantías, mismo formato de respuesta JSON, mismo contrato de
errores (`code`, `recoverable`, `details`, `next_action`). El servidor
corre por `stdio` con `royo-learn mcp-serve`.

## Integración opcional con Engram

Si Engram está corriendo en `http://127.0.0.1:7437`, Royo-Learn lo usa para:

- consultar contexto previo de sesiones (`GET /context`, `GET /search`);
- opcionalmente guardar una referencia breve al candidato (`POST /observations`).

Royo-Learn **nunca** accede directamente a `~/.engram/engram.db`. Si Engram
no está disponible, sigue funcionando: el `doctor` reporta
`engram.available: false, degraded: true, reason: <motivo>` y el resto del
ciclo no se interrumpe.

## Lo que Royo-Learn no hace en v1

Tomado literal de `docs/01-PRD.md` §3 (no-objetivos):

- entrenar modelos o modificar pesos;
- evolución automática estilo GEA;
- almacenar conversaciones completas;
- sustituir Engram o sustituir Gentle-AI;
- sincronización cloud propia;
- base vectorial o embeddings;
- panel web;
- proveedor LLM embebido;
- auto-edición irrestricta del sistema del usuario.

Cualquier función listada arriba está fuera del alcance y no debe presentarse
como disponible.

## Estructura del proyecto

```text
agent-royo-learn/
├── cmd/royo-learn/        # entry point CLI/MCP, comandos y e2e
├── internal/
│   ├── buildinfo/         # metadatos de versión (inyectados vía ldflags)
│   ├── capture/           # captura idempotente y relaciones candidatas
│   ├── config/            # configuración global y de proyecto
│   ├── curate/            # máquina de estados y acciones de curación
│   ├── doctor/            # diagnóstico de salud
│   ├── domain/            # entidades, enums, errores tipados
│   ├── engram/            # cliente HTTP local opcional (nunca la DB)
│   ├── evidence/          # recolección, hashing, redacción de secretos
│   ├── integration/       # glue de composición
│   ├── logging/           # logging estructurado a stderr
│   ├── mcpserver/         # tools, schemas, middleware, stdio
│   ├── project/           # resolución segura de raíz y project key
│   ├── publish/           # planning, patch, atomic write, verify, rollback
│   ├── record/            # materialización Markdown auditable
│   ├── recurrence/        # fingerprint y métricas
│   ├── selfupdate/        # auto-actualización del binario
│   ├── setup/             # registro MCP, distribución de Skills, backup
│   ├── storage/           # SQLite, migraciones, repositorios, FTS5, WAL
│   └── testutil/          # utilidades de test
├── migrations/            # SQL versionado
├── schemas/               # JSON schemas
├── skills/                # Skills del proyecto
├── docs/                  # PRD, arquitectura, contratos, reports
├── examples/              # entradas de ejemplo
├── install.sh             # instalador Linux/macOS/WSL
├── install.ps1            # instalador Windows (PowerShell)
├── Makefile               # fmt, vet, test -race, build, build-all, quality
├── .goreleaser.yml        # pipeline de release
├── AGENTS.md              # instrucciones del agente ejecutor
├── CODEX_START_HERE.md    # orden de lectura para Codex
├── PROMPT_FOR_CODEX.md    # prompt canónico de Codex
├── TASKS.md               # plan de implementación por tramos
└── README.md              # este archivo
```

## Documentación

La fuente de verdad, en orden de precedencia:

1. `docs/14-ACCEPTANCE-CRITERIA.md` — criterios de aceptación.
2. `docs/01-PRD.md` — producto.
3. `docs/02-ARCHITECTURE.md` — arquitectura.
4. `docs/04-CLI-SPEC.md`, `docs/05-MCP-SPEC.md`, `docs/06-DATABASE-SCHEMA.md`,
   `docs/09-PUBLISHING-ENGINE.md`, `docs/10-SECURITY.md`,
   `docs/11-INSTALLATION.md`, `docs/12-TEST-PLAN.md`,
   `docs/15-OPERATIONS.md`, `docs/17-ERROR-CODES.md`,
   `docs/18-REFERENCES.md`.
5. `TASKS.md` — tareas por tramos.
6. `AGENTS.md` — reglas no negociables.
7. `README.md` (este archivo) — entrada al proyecto.

`docs/FINAL-IMPLEMENTATION-REPORT.md` resume el cierre del proyecto con la
tabla `Requisito | Estado | Prueba | Evidencia` (estados válidos: `PASS`,
`FAIL`, `NOT_APPLICABLE`). Un `FAIL` impide declarar terminado.

`docs/generated/CLI_REFERENCE.md`, `MCP_REFERENCE.md`, `ERROR_REFERENCE.md` y
`PROFILES.md` se generan automáticamente a partir del código; **no** se
copian a mano para evitar que el README contradiga al binario.

## Licencia

MIT.