# Integración con Gentle-AI y Codex

## Gentle-AI

Gentle-AI sigue siendo el configurador. Agent Royo Learn se instala como compañero.

### Skills

Instalar:

```text
skills/capture-learning/SKILL.md
skills/curate-learning/SKILL.md
skills/publish-learning/SKILL.md
```

Después:

```bash
gentle-ai skill-registry refresh --force
```

Verificar:

```text
.atl/skill-registry.md
```

La aplicación debe resolver la Skill canónica desde el registro cuando exista, pero debe funcionar sin él.

### Archivos administrados

No modificar:

- Skills embebidas instaladas por Gentle-AI;
- persona;
- prompts gestionados;
- presets;
- state interno;
- config global generada.

Las Skills propias permanecen en el repositorio Agent Royo Learn o en una biblioteca compartida explícita.

### Doctor

Si `gentle-ai` existe:

```bash
gentle-ai doctor
```

Se ejecuta como check externo de solo lectura.

## Codex

### Registro recomendado

```bash
codex mcp add royo-learn -- royo-learn mcp
```

Verificación:

```bash
codex mcp list
```

Codex comparte la configuración MCP entre CLI, app e IDE en el mismo host.

### Config TOML equivalente

```toml
[mcp_servers.royo-learn]
command = "royo-learn"
args = ["mcp", "--tools", "agent"]
startup_timeout_sec = 10
tool_timeout_sec = 120
enabled = true
required = false
default_tools_approval_mode = "writes"

[mcp_servers.royo-learn.tools.learning_publish]
approval_mode = "approve"

[mcp_servers.royo-learn.tools.learning_approve]
approval_mode = "approve"
```

El instalador debe:

1. detectar Codex;
2. ofrecer registro;
3. hacer backup de `~/.codex/config.toml`;
4. preferir `codex mcp add`;
5. no duplicar entradas;
6. verificar con `codex mcp list`;
7. permitir `--skip-codex`.

### Project config

También puede generarse `.codex/config.toml`, pero solo en repositorios confiables y sin sobrescribir valores existentes.

## Activación de Skills

Codex/Gentle-AI deben usar descripciones que indiquen triggers. No confiar únicamente en comandos manuales.

### Trigger de captura

- el usuario corrige al agente;
- se resuelve un bug no obvio;
- se descubre una limitación;
- se repite un error;
- se termina una fase importante.

### Trigger de curación

- hay candidatos pendientes;
- se intenta transferir aprendizaje;
- existe conflicto;
- se propone modificar una Skill o regla.

### Trigger de publicación

- existe curación aprobada;
- el preview fue revisado;
- se dispone de aprobación cuando es obligatoria.

## Compatibilidad con otros agentes

El proyecto no debe contener lógica Codex-only. Proveer ejemplos para OpenCode y configuración MCP genérica.
