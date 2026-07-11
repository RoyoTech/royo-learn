# Agent Royo Learn

**Agent Royo Learn** es un motor local de aprendizaje institucional para agentes de IA.

No reemplaza a Gentle-AI ni a Engram:

- **Gentle-AI** configura agentes, Skills, workflows y MCP.
- **Engram** conserva memoria persistente de sesiones, decisiones, descubrimientos y errores.
- **Agent Royo Learn** transforma experiencias verificadas en cambios reutilizables de comportamiento: conocimiento, Skills, reglas, pruebas y alertas de reincidencia.

El repositorio produce un único binario multiplataforma:

```text
royo-learn        # Linux/macOS
royo-learn.exe    # Windows
```

## Problema que resuelve

Guardar una memoria no asegura que el siguiente agente trabaje mejor. El proyecto añade un ciclo explícito:

```text
experiencia
    ↓
captura estructurada
    ↓
búsqueda de duplicados y antecedentes
    ↓
curaduría con evidencia
    ↓
aprobación
    ↓
publicación controlada
    ↓
verificación
    ↓
detección de reincidencias
```

## Principio arquitectónico

> El LLM interpreta; el binario garantiza.

Las tres Skills determinan qué significa una experiencia:

1. `capture-learning`
2. `curate-learning`
3. `publish-learning`

El binario garantiza:

- persistencia;
- estados válidos;
- idempotencia;
- trazabilidad;
- deduplicación lexical;
- integración opcional con Engram;
- evidencia de Git y pruebas;
- publicación segura;
- aprobación humana;
- rollback;
- detección de reincidencias;
- MCP por `stdio`;
- CLI equivalente.

## Inicio obligatorio para Codex

Codex debe leer, en este orden:

1. `AGENTS.md`
2. `CODEX_START_HERE.md`
3. `docs/01-PRD.md`
4. `docs/02-ARCHITECTURE.md`
5. `TASKS.md`

No debe comenzar a implementar desde este README.

## Alcance de la versión 1

La versión 1 es local, sin servicio cloud y sin proveedor LLM embebido. El razonamiento semántico lo realiza el agente que llama al servidor MCP.

La aplicación debe funcionar aunque Gentle-AI o Engram no estén instalados. Sus integraciones son opcionales y degradables.

## Estructura final esperada

```text
agent-royo-learn/
├── cmd/royo-learn/
├── internal/
│   ├── approval/
│   ├── audit/
│   ├── capture/
│   ├── config/
│   ├── domain/
│   ├── engram/
│   ├── evidence/
│   ├── gitx/
│   ├── mcpserver/
│   ├── project/
│   ├── publish/
│   ├── recurrence/
│   ├── redact/
│   ├── search/
│   ├── storage/
│   └── validate/
├── migrations/
├── schemas/
├── skills/
├── scripts/
├── docs/
├── examples/
├── e2e/
├── AGENTS.md
├── TASKS.md
├── Makefile
├── go.mod
├── go.sum
└── .goreleaser.yaml
```

## Resultado esperado

Al finalizar, esto debe funcionar:

```bash
royo-learn doctor
royo-learn init
royo-learn capture --file examples/capture-request.json
royo-learn search "migraciones"
royo-learn review
royo-learn mcp
```

Y Codex debe poder registrar el MCP:

```bash
codex mcp add royo-learn -- royo-learn mcp
codex mcp list
```

La definición de “terminado” está en `docs/14-ACCEPTANCE-CRITERIA.md`.
