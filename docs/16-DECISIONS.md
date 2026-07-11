# Decisiones arquitectónicas

## ADR-001 — Proyecto independiente

No fork de Gentle-AI ni Engram. Reduce deuda y permite uso agnóstico.

## ADR-002 — Go

Binario único, multiplataforma, consistente con el ecosistema.

## ADR-003 — SQLite + FTS5

Suficiente para búsqueda lexical y operación local. Sin vector DB en v1.

## ADR-004 — Sin LLM embebido

El cliente MCP ya dispone de LLM. Evita claves, costos, lock-in y razonamiento duplicado.

## ADR-005 — Tres Skills

Separar captura, curación y publicación impide que una observación se convierta automáticamente en regla.

## ADR-006 — DB + records Markdown

DB para operación; Markdown para auditoría, Git y reconstrucción.

## ADR-007 — Engram por API pública

No acoplarse a su esquema interno.

## ADR-008 — Preview hash

Evita aprobar una cosa y publicar otra.

## ADR-009 — Humano para alto impacto

AGENTS.md, shared y modificaciones de Skills existentes requieren aprobación explícita.

## ADR-010 — MCP stdio primero

Ideal para agentes locales y sin puerto público.

## ADR-011 — Publicación determinista

El LLM suministra contenido/patch; el binario no reescribe libremente documentos.

## ADR-012 — Sin auto-capture crudo

Privacidad y calidad. La Skill decide cuándo hay aprendizaje.
