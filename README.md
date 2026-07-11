# Agent Royo Learn

**Agent Royo Learn** is a local institutional learning engine for AI agents.

It does not replace Gentle-AI or Engram:

- **Gentle-AI** configures agents, Skills, workflows, and MCP.
- **Engram** preserves persistent memory of sessions, decisions, discoveries, and errors.
- **Agent Royo Learn** transforms verified experiences into reusable behavior changes: knowledge, Skills, rules, tests, and recurrence alerts.

The repository produces a single cross-platform binary:

```text
royo-learn        # Linux/macOS
royo-learn.exe    # Windows
```

## Installation

### Linux / macOS

```bash
curl -fsSL https://github.com/angel-royo/royo-learn/releases/latest/download/install.sh | bash
```

Or manually:

```bash
# Download and install
./install.sh --version v1.0.0
# Uninstall
./install.sh --uninstall
```

The binary is installed to `~/.local/bin/royo-learn`. Add it to your PATH if needed.

### Windows

```powershell
# Download the installer
Invoke-WebRequest -Uri https://github.com/angel-royo/royo-learn/releases/latest/download/install.ps1 -OutFile install.ps1

# Install
.\install.ps1 --version v1.0.0

# Uninstall
.\install.ps1 --uninstall
```

The binary is installed to `%LOCALAPPDATA%\royo-learn\bin\royo-learn.exe`.

### Build from source

```bash
# Prerequisites: Go 1.24+
git clone https://github.com/angel-royo/royo-learn.git
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

Register royo-learn as an MCP server in your Codex/Claude configuration:

### Codex

```bash
codex mcp add royo-learn -- royo-learn mcp-serve
codex mcp list
```

### Claude Desktop / OpenCode

Add to your MCP config file:

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
