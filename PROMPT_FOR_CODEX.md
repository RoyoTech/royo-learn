# Prompt de ejecución para Codex

Trabaja dentro de este repositorio y construye Agent Royo Learn completo.

Lee primero `AGENTS.md` y `CODEX_START_HERE.md`, luego todos los documentos allí exigidos. Implementa `TASKS.md` en orden. No entregues solamente análisis: crea código, pruebas, scripts de instalación y artefactos de release.

Requisitos esenciales:

- Go 1.24+;
- un solo binario `royo-learn`;
- CLI y servidor MCP stdio;
- SQLite + FTS5 sin CGO;
- integración opcional con Engram por HTTP público local;
- integración no invasiva con Gentle-AI;
- registro MCP en Codex con backup;
- tres Skills incluidas;
- preview hash, aprobación, publicación atómica, verificación y rollback;
- seguridad de rutas, secretos y comandos;
- pruebas unitarias, integración y e2e;
- compilación Windows/Linux/macOS;
- instalación local y desinstalación;
- `docs/FINAL-IMPLEMENTATION-REPORT.md`.

No hagas fork de Gentle-AI o Engram. No accedas a sus bases internas. No añadas proveedor LLM al binario.

Continúa hasta que `docs/14-ACCEPTANCE-CRITERIA.md` esté satisfecho o documenta con precisión un bloqueo externo real. No marques tareas como terminadas sin evidencia.
