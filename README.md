# Agent Royo Learn

[![English](https://img.shields.io/badge/lang-en-blue.svg)](README.md)
[![Español](https://img.shields.io/badge/lang-es-yellow.svg)](docs/README.es.md)
[![Français](https://img.shields.io/badge/lang-fr-purple.svg)](docs/README.fr.md)
[![Deutsch](https://img.shields.io/badge/lang-de-red.svg)](docs/README.de.md)
[![中文](https://img.shields.io/badge/lang-zh-green.svg)](docs/README.zh.md)
[![हिन्दी](https://img.shields.io/badge/lang-hi-orange.svg)](docs/README.hi.md)
[![Português](https://img.shields.io/badge/lang-pt-lightgrey.svg)](docs/README.pt.md)

**Agent Royo Learn** is a local institutional learning engine for AI agents.

It does not replace Gentle-AI or Engram:

- **Gentle-AI** configures agents, Skills, workflows, and MCP.
- **Engram** preserves persistent memory of sessions, decisions, discoveries, and errors.
- **Agent Royo Learn** transforms verified experiences into reusable behavior changes: knowledge, Skills, rules, tests, and recurrence alerts.

### Engram + Royo-Learn: Knowledge + Understanding

There is a useful distinction between two concepts:

- **Knowledge**: raw data, facts, answers — easily accessible. Today, tools like AI give us instant access to "knowledge" with zero effort.
- **Understanding**: the deep cognitive process of processing, reasoning, and integrating that information. When we delegate everything, we stop burning neurons and lose the ability to truly understand.

This same distinction maps to the two systems:

| | Engram | Royo-Learn |
|---|---|---|
| **Role** | Persistent memory | Learning engine |
| **What it does** | Stores what happened | Processes, reasons, integrates |
| **Analogy** | Knowledge (the notebook) | Understanding (the act of studying) |

**Processing**: Royo-Learn does not accept raw data and store it. The capture flow ([Architecture §4](docs/02-ARCHITECTURE.md)) validates the payload, normalizes and hashes it, checks idempotency, searches lexically (FTS5), collects deterministic evidence, and only then persists the record.

**Reasoning**: The deduplication system ([RF-004](docs/01-PRD.md#rf-004-deduplicación)) defines semantic relationships between learnings: `duplicate_of`, `extends`, `supersedes`, `contradicts`, `narrows`, `related`. The state machine ([RF-005](docs/01-PRD.md#rf-005-estado)) forces decisions: is this rejected, does it need evidence, should it be merged or approved? It is not neutral storage — it evaluates the validity and coherence of knowledge.

**Integrating**: A learning does not stay in a database row. It becomes a Skill or a rule, gets recovered in another session, and *prevents or detects a recurrence* ([PRD §8](docs/01-PRD.md)). The publication flow ([Architecture §5](docs/02-ARCHITECTURE.md)) — approved → preview → approve → publish → verify → rollback — turns understanding into operational behavior change.

Royo-Learn does not understand *for* the model. It is the scaffolding that makes understanding matter. Without it, an LLM can understand something in one session, but that understanding evaporates. With it, that understanding becomes persistent, verifiable, relational, and actionable.

The repository produces a single cross-platform binary:

```text
royo-learn        # Linux/macOS
royo-learn.exe    # Windows
```

## Installation

### Linux / macOS

```bash
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/latest/download/install.sh | bash
```

Or manually:

```bash
# Download and install
./install.sh --version v0.1.0
# Uninstall
./install.sh --uninstall
```

The binary is installed to `~/.local/bin/royo-learn`. Add it to your PATH if needed.

### Windows

```powershell
# Download the installer
Invoke-WebRequest -Uri https://github.com/RoyoTech/royo-learn/releases/latest/download/install.ps1 -OutFile install.ps1

# Install
.\install.ps1 --version v0.1.0

# Uninstall
.\install.ps1 --uninstall
```

The binary is installed to `%LOCALAPPDATA%\royo-learn\bin\royo-learn.exe`.

### Build from source

```bash
# Prerequisites: Go 1.24+
git clone https://github.com/RoyoTech/royo-learn.git
cd royo-learn
make build       # Build for current platform
make build-all   # Cross-compile all platforms
make install     # Install to $GOPATH/bin
make clean       # Remove build artifacts
make quality     # Run fmt, test, vet, build
```

## Quick Start

```bash
# Check version
royo-learn version --json

# Initialize a project
royo-learn init --project-root /path/to/your/project

# Run health check
royo-learn doctor --project-root /path/to/your/project --json

# Capture a learning
royo-learn capture \
  --project-root /path/to/your/project \
  --title "PostgreSQL connection pooling" \
  --context "production deployment" \
  --observation "connection pool exhausted at 100 concurrent" \
  --lesson "set max_connections based on available memory" \
  --type "procedure" \
  --scope "project" \
  --json

# Curate (approve/reject) a learning
royo-learn curate \
  --project-root /path/to/your/project \
  --learning-id "<learning-id>" \
  --action "approve" \
  --rationale "validated with load testing" \
  --json

# Preview before publishing
royo-learn preview \
  --project-root /path/to/your/project \
  --learning-id "<learning-id>" \
  --json

# Publish (requires preview hash)
royo-learn publish \
  --project-root /path/to/your/project \
  --learning-id "<learning-id>" \
  --preview-hash "<preview-hash>" \
  --json

# Rollback a publication
royo-learn rollback \
  --project-root /path/to/your/project \
  --journal-id "<publication-id>" \
  --json

# Check recurrences
royo-learn recurrences --learning-id "<learning-id>" --json
royo-learn metrics --learning-id "<learning-id>" --json

# Run end-to-end smoke test
royo-learn e2e --temp
```

## MCP Server Setup

The `setup` command registers royo-learn as an MCP server and installs the
project's Skills across Claude Code, Codex CLI, and OpenCode — all in one step:

```bash
# See current status
royo-learn setup status

# Install in all three agents
royo-learn setup install --agent all

# Install in a specific agent (skills only, skip MCP)
royo-learn setup install --agent claude-code --skip-mcp

# Dry-run first
royo-learn setup install --agent all --dry-run --json

# Uninstall
royo-learn setup uninstall --agent all
```

### Manual registration

If you prefer to register manually:

**Codex**:
```bash
codex mcp add royo-learn -- royo-learn mcp-serve
```

**Claude Code / OpenCode** — add to your MCP config file:

```json
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

**OpenCode** uses the `"mcp"` key (not `"mcpServers"`) with `"command"` as an array — use `setup install --agent opencode` for correct formatting.

**Profiles**: `minimal` (capture, search, doctor), `standard` (default; includes curate, preview, list, get), `full` (all tools including publish).

```bash
royo-learn mcp-serve --profile full
```

## Architecture

```
LLM + Skill → semantic proposal
royo-learn  → operational guarantee
```

The three Skills that define what an experience means:

1. `capture-learning`
2. `curate-learning`
3. `publish-learning`

The binary guarantees:

- persistence
- valid states
- idempotency
- traceability
- lexical deduplication
- optional Engram integration
- Git evidence and tests
- secure publication
- human approval
- rollback
- recurrence detection
- MCP over stdio

## Problem It Solves

Storing a memory doesn't ensure the next agent works better. The project adds an explicit cycle:

```
experience
    ↓
structured capture
    ↓
duplicate and antecedent search
    ↓
curation with evidence
    ↓
approval
    ↓
controlled publication
    ↓
verification
    ↓
recurrence detection
```

## Version 1 Scope

Version 1 is local, without cloud service or embedded LLM provider. Semantic reasoning is performed by the agent calling the MCP server.

The application works even if Gentle-AI or Engram are not installed. Their integrations are optional and degradable.

## Codex Onboarding

Codex must read, in this order:

1. `AGENTS.md`
2. `CODEX_START_HERE.md`
3. `docs/01-PRD.md`
4. `docs/02-ARCHITECTURE.md`
5. `TASKS.md`

Do not start implementing from this README.

## Development

```bash
make fmt        # Format code
make test       # Run tests
make vet        # Run vet
make build      # Build for current platform
make build-all  # Cross-compile all platforms
make quality    # Full quality check (fmt + test + vet + build)
```

## Project Structure

```text
agent-royo-learn/
├── cmd/royo-learn/        # CLI entry point + e2e
├── internal/
│   ├── buildinfo/         # Version metadata
│   ├── capture/           # Learning capture service
│   ├── config/            # Project/user configuration
│   ├── curate/            # Curation service
│   ├── doctor/            # Health checks
│   ├── domain/            # Domain types and transitions
│   ├── engram/            # Engram integration
│   ├── evidence/          # Evidence collection (redaction, path security, git)
│   ├── logging/           # Structured logging
│   ├── mcpserver/         # MCP server implementation
│   ├── project/           # Project resolution
│   ├── publish/           # Publication engine
│   ├── recurrence/        # Recurrence detection
│   ├── setup/             # Setup helpers (skills, MCP registration, backup)
│   └── storage/           # SQLite database (migrations, repos, FTS5)
├── migrations/            # SQL migration files
├── schemas/               # JSON schemas
├── skills/                # Project Skills
├── docs/                  # Documentation
├── examples/              # Example inputs
├── AGENTS.md              # Agent instructions
├── TASKS.md               # Implementation tasks
├── Makefile               # Build targets
├── .goreleaser.yml        # Release configuration
├── install.sh             # Linux/macOS installer
├── install.ps1            # Windows installer
├── go.mod
└── go.sum
```

## License

MIT
