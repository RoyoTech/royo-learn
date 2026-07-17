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

Esta sección recorre un caso real, de punta a punta, **usando Royo-Learn
desde un agente** — el caso de uso para el que el sistema fue diseñado. El
ejemplo primario está escrito para **OpenCode**; los bloques siguientes
indican las diferencias concretas (cuando las hay) para **Claude Code**,
**Codex CLI** y **Pi**.

Donde un campo o valor de retorno pueda variar entre ejecuciones, se
marca con `<…>`. Cada herramienta, nombre de campo y estado del JSON que
aparece abajo está verificado contra `internal/mcpserver/tools.go`,
`internal/mcpserver/profiles.go` e `internal/domain/types.go`.

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

### Compatibilidad entre agentes

Royo-Learn expone un único servidor MCP por `stdio` con herramientas
estables y JSON Schema estricto. **OpenCode, Claude Code, Codex CLI y
Pi** lo consumen de la misma forma: registran el binario como servidor
MCP, descubren las herramientas al iniciar, y las invocan con los
mismos argumentos. La diferencia entre clientes es solo el lugar donde
se declara el servidor:

```text
OpenCode      ~/.config/opencode/opencode.json   "mcp": { "royo-learn": { ... } }
Claude Code   ~/.claude/mcp.json                 "mcpServers": { ... }
Codex CLI     codex mcp add royo-learn -- royo-learn mcp-serve
Pi            ~/.pi/agent/mcp.json               "mcpServers": { ... }
```

Una vez registrado, las herramientas y sus respuestas son idénticas. El
ejemplo de abajo está narrado en OpenCode porque es el cliente que
mantiene este repo; en cualquier otro cliente, lo único que cambia es la
configuración inicial, no los tool calls.

### Acto 1 — Inicialización (una vez por raíz)

El sistema no existe hasta que el proyecto decide crearlo. Cada raíz de
proyecto tiene su propio `.royo-learn/` con DB, records y backups. En el
flujo MCP, esto se hace desde la terminal **una sola vez por proyecto**;
después, todos los agentes del usuario lo encuentran automáticamente.

```bash
$ royo-learn init --project-root ~/code/mi-app --json
{
  "status": "initialized",
  "project_root": "~/code/mi-app",
  "store": "~/code/mi-app/.royo-learn/store.db",
  "config": "~/code/mi-app/.royo-learn/config.yaml",
  "next_action": "run \"royo-learn doctor --project-root ~/code/mi-app --json\""
}
```

Después, en **OpenCode**, Maria abre la carpeta del proyecto. La Skill
`royo-learn-onboarding` (distribuida por `setup install`) detecta que
falta el `.royo-learn/` y le pregunta si quiere inicializar. Ella acepta,
OpenCode corre `init` por ella y, a partir de acá, todas las tools MCP
tienen un proyecto válido al cual resolver. La primera llamada útil que
hace el agente para confirmar el estado es `learning_doctor`:

> **Maria:** *"¿Está todo sano en este proyecto?"*

OpenCode invoca:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "learning_doctor",
    "arguments": {}
  }
}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"ok\":true,\"version\":\"v0.1.10\",\"checks\":[{\"name\":\"database\",\"status\":\"pass\",\"message\":\"database connection ok\"},{\"name\":\"project\",\"status\":\"pass\",\"message\":\"project resolved\"}]}"
      }
    ]
  }
}
```

El doctor **no** reporta Engram en esta versión (el binario no consulta
Engram en `learning_doctor`; lo hace en el doctor CLI cuando está
disponible). Si el `ok` viene `false`, OpenCode aborta y le pide a Maria
revisar antes de seguir.

### Acto 2 — Captura (la LLM redacta, el binario valida)

> **Maria:** *"Guardame este aprendizaje: nunca aplicar una migración sin
> verificar primero su estado real en la base."*

La Skill `capture-learning` de OpenCode toma esa frase, la estructura en
los campos del schema JSON de `learning_capture` y hace la llamada.
**Maria no ve el JSON** — OpenCode se lo muestra solo si ella lo pide.

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "learning_capture",
    "arguments": {
      "title": "Migrations must be idempotent and state-checked",
      "type": "procedure",
      "context": "Production incident: migration 0047 re-ran on a half-applied DB.",
      "observation": "ORM idempotency check failed mid-transaction; user_prefs table left inconsistent; manual rollback took half a day.",
      "reusable_lesson": "Always query schema_migrations before running make migrate; refuse to apply if state diverges from expected.",
      "scope_guess": "project",
      "confidence": "high",
      "evidence_level": "moderate",
      "proposed_destination": "agents_rule",
      "recommended_procedure": [
        "psql -c 'SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 5'",
        "if divergence: abort and request human review"
      ],
      "retrieval_terms": ["migration", "schema_migrations", "idempotent", "make migrate"],
      "idempotency_key": "maria-2026-07-17-migration-state",
      "actor": { "kind": "human", "name": "maria" }
    }
  }
}
```

Respuesta del servidor MCP (el texto que OpenCode muestra a Maria):

```json
{
  "learning_id": "<ULID>",
  "status": "needs_evidence",
  "new": true,
  "evidence_count": 0,
  "evidence_ids": [],
  "redacted": false
}
```

Cuatro garantías que el binario aplicó en esa sola llamada, sin que la
LLM tuviera que pedirlas:

1. **Validación de contrato** según el JSON Schema (campos requeridos,
   enums de `type`, `scope_guess`, `confidence`, `evidence_level`,
   `proposed_destination`).
2. **Redacción de secretos** — si Maria hubiera pegado una contraseña en
   `context`, se reemplaza por `[REDACTED]` **antes** de tocar SQLite,
   el blob store, el Markdown, el audit log y esta misma respuesta. El
   flag `redacted: true` lo indicaría.
3. **Hash normalizado + búsqueda en FTS5** — si ya existía un candidato
   con el mismo fingerprint, devuelve el existente y `new: false`.
4. **Estado inicial**: `needs_evidence`, porque el destino propuesto
   (`agents_rule`) exige evidencia antes de poder aprobarse.

### Acto 3 — Evidencia

Maria tiene el diff del fallo en `incident-0047.diff`. Se la pasa al
agente y OpenCode llama a `learning_add_evidence` con un array `evidence`
(mínimo uno; aquí adjuntamos dos).

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "learning_add_evidence",
    "arguments": {
      "learning_id": "<ULID>",
      "evidence": [
        {
          "kind": "file",
          "source": "incident-0047.diff",
          "summary": "Failed migration run + ORM error log + manual fix diff",
          "content": "<contenido del diff>"
        },
        {
          "kind": "git_commit",
          "source": "abc1234",
          "summary": "Manual revert commit that recovered production"
        }
      ],
      "evidence_level": "strong",
      "actor": { "kind": "human", "name": "maria" }
    }
  }
}
```

```json
{
  "learning_id": "<ULID>",
  "evidence_count": 2,
  "evidence_ids": ["<ULID>", "<ULID>"],
  "evidence_level": "strong",
  "redacted": false
}
```

El **estado se mantiene** en `needs_evidence`. La promoción a `approved`
no la hace el adjuntar evidencia — la hace la curación.

### Acto 4 — Curación

Maria revisa el candidato, ve que la evidencia alcanza, y aprueba con
fundamento. La tool MCP usa el campo `decision` (no `action` como la CLI)
y exige los tres campos `decision`, `rationale`, `actor`:

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "learning_curate",
    "arguments": {
      "learning_id": "<ULID>",
      "decision": "approve",
      "rationale": "Validated against incident-0047; same pattern would prevent the next half-applied migration.",
      "actor": { "kind": "human", "name": "maria" }
    }
  }
}
```

```json
{
  "curation_id": "<ULID>",
  "learning_id": "<ULID>",
  "new_status": "approved"
}
```

`decision` también acepta `reject`, `needs_evidence`, `relate`, `merge`,
`supersede` y `archive`. Solo una promoción a `approved` habilita la
publicación.

### Acto 5 — Preview de publicación

**El sistema nunca escribe sin preview.** Antes de tocar un archivo del
proyecto, persiste el preview y devuelve su hash. La tool MCP devuelve
**menos campos que la CLI** (no incluye `targets`, `verification_commands`
ni `rollback_plan` — esos viven en el `preview` del CLI; en MCP el
agente los deduce del `diff`):

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "learning_publication_preview",
    "arguments": {
      "learning_id": "<ULID>",
      "actor": { "kind": "human", "name": "maria" }
    }
  }
}
```

```json
{
  "preview_id": "<ULID>",
  "preview_hash": "<sha256>",
  "risk": "low",
  "requires_approval": true,
  "diff": "@@ AGENTS.md\n+## Migrations\n+\n+- Always query schema_migrations before running make migrate.\n+- Refuse to apply if state diverges from expected.\n"
}
```

`requires_approval: true` aparece porque el destino `agents_rule`
(escribir una regla en `AGENTS.md` compartido) está marcado como
sensible. Sin aprobación humana ligada a este `preview_hash`, `publish`
rechaza la llamada.

### Acto 6 — Aprobación humana

> **Maria:** *"Apruebo este preview, con mi identidad."*

OpenCode llama a `learning_approve`. **La tool MCP exige cuatro campos
no opcionales**: `preview_hash`, `approved_by`, `reason` y
`approval_evidence` (referencia al consentimiento — un link, un ID de
mensaje, un ticket).

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "tools/call",
  "params": {
    "name": "learning_approve",
    "arguments": {
      "learning_id": "<ULID>",
      "preview_hash": "<sha256>",
      "approved_by": "maria",
      "reason": "Reviewed the incident diff and the proposed rule; ship it.",
      "approval_evidence": "https://github.com/RoyoTech/mi-app/issues/142#issuecomment-1",
      "actor": { "kind": "human", "name": "maria" }
    }
  }
}
```

```json
{
  "approval_id": "<ULID>",
  "learning_id": "<ULID>",
  "preview_hash": "<sha256>",
  "approved_by": "maria"
}
```

La aprobación está **ligada al hash exacto del preview**. Si alguien
recalcula el preview mañana porque cambió una palabra, el hash cambia y
la aprobación vieja queda inválida.

### Acto 7 — Publicación atómica

> **Maria:** *"Dale, publicá."*

OpenCode llama a `learning_publish` con `apply: true` (sin esto, es un
dry-run que solo reporta el plan):

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "tools/call",
  "params": {
    "name": "learning_publish",
    "arguments": {
      "learning_id": "<ULID>",
      "preview_hash": "<sha256>",
      "approval_id": "<ULID>",
      "apply": true,
      "actor": { "kind": "human", "name": "maria" }
    }
  }
}
```

```json
{
  "publication_id": "<ULID>",
  "learning_id": "<ULID>",
  "status": "published",
  "journal_id": "<ULID>"
}
```

**Qué pasó en el servidor, sin que Maria lo viera:**

1. Backup de `AGENTS.md` a `.royo-learn/backups/<journal>.bak`.
2. Validación de que el preview hash sigue siendo el mismo.
3. Validación de que la aprobación no está vencida.
4. Escritura atómica (temp file + rename).
5. Verificación post-aplicación.
6. Si la verificación falla → restaura desde backup, marca
   `rollback_failed`, devuelve el path del artefacto de recuperación.
7. Si todo va bien → estado `published`.

`journal_id` es la llave para revertir. Si Maria descubre dentro de un
mes que la regla estaba mal redactada:

```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "method": "tools/call",
  "params": {
    "name": "learning_rollback",
    "arguments": {
      "publication_id": "<ULID>",
      "actor": { "kind": "human", "name": "maria" }
    }
  }
}
```

(`learning_rollback` vive en el perfil `admin`, no en `agent` — es la
única tool destructiva del sistema.)

### Acto 8 — La regla vive

`AGENTS.md` ahora contiene, en una sección identificada:

<!-- prettier-ignore -->
```markdown
## Migrations

- Always query schema_migrations before running make migrate.
- Refuse to apply if state diverges from expected.
```

La próxima vez que cualquier agente — de cualquier sesión, de cualquier
cliente MCP — abra el proyecto, leerá `AGENTS.md` antes de empezar a
trabajar (esa es la convención del proyecto) y se topará con la regla.

### Acto 9 — Tres semanas después: la reincidencia

Un agente nuevo (otra sesión, otro usuario, otro día) arranca una tarea
sobre el mismo proyecto. Lee `AGENTS.md`, ve la regla y, antes de correr
`make migrate`, ejecuta la verificación. Detecta que el estado del ORM no
coincide con la migración esperada y **aborta con un mensaje claro** en
vez de avanzar y romper la base.

Maria registra el incidente como reincidencia **prevenida**:

```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "tools/call",
  "params": {
    "name": "learning_report_occurrence",
    "arguments": {
      "learning_id": "<ULID>",
      "summary": "Same half-applied migration pattern, but the new agent read AGENTS.md and aborted before running make migrate.",
      "outcome": "prevented",
      "retrieved": true,
      "skill_activated": false,
      "evidence": "session-2026-08-04.log#L42",
      "idempotency_key": "session-2026-08-04-prevented-1",
      "actor": { "kind": "agent", "name": "opencode", "model": "MiniMax-M3", "session_id": "sess-2026-08-04" }
    }
  }
}
```

```json
{
  "recurrence_id": "<ULID>",
  "learning_id": "<ULID>",
  "fingerprint": "<sha256>",
  "occurred_at": "2026-08-04T15:22:11Z",
  "outcome": "prevented",
  "retrieved": true,
  "skill_activated": false,
  "new": true
}
```

Y mide:

```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "method": "tools/call",
  "params": {
    "name": "learning_compute_metrics",
    "arguments": { "learning_id": "<ULID>" }
  }
}
```

```json
{
  "fingerprint": "<sha256>",
  "count": 1,
  "first_seen": "2026-08-04T15:22:11Z",
  "last_seen": "2026-08-04T15:22:11Z",
  "avg_interval": "0s",
  "trend": "stable",
  "needs_review": false,
  "review_reason": ""
}
```

**El ciclo cierra.** Una corrección explicada una vez se convirtió en un
cambio verificable del comportamiento, con auditoría completa y métrica
que demuestra que la regla sirvió.

### Por qué el ejemplo es el mismo en todos los clientes

Las herramientas, los nombres de campos, los valores de retorno, los
códigos de error (`code`, `recoverable`, `details`, `next_action`) y los
perfiles (`read`, `agent`, `admin`) son **propiedad del binario**, no del
cliente. OpenCode, Claude Code, Codex y Pi llaman a las mismas tools con
los mismos argumentos. Cambiar de cliente no rompe el ciclo ni la
auditoría.

Las únicas diferencias reales entre clientes son:

- **Configuración del servidor** (ya mostrada arriba).
- **Cómo se renderiza el JSON al usuario** (OpenCode lo muestra
  opcionalmente con `/tools`, Claude Code lo oculta por defecto, Codex
  CLI lo muestra en su panel de tools).
- **Qué triggers naturales reconoce la Skill** (las tres Skills del
  proyecto — `capture-learning`, `curate-learning`, `publish-learning`
  — son las mismas; cada cliente las invoca con su propio routing de
  intención).

### El mismo ciclo, vía CLI

Toda la secuencia anterior también está disponible como comandos CLI con
la misma semántica y los mismos nombres de campo, para integraciones
shell, scripts de CI o depuración. Ver `docs/04-CLI-SPEC.md`.

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