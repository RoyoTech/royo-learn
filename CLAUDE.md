# CLAUDE.md — Instrucciones para Claude (Anthropic) en este repositorio

Este archivo existe para que Claude (Anthropic) o cualquier agente que
opere en nombre de Anthropic tenga contexto explícito sobre cómo
trabajar en este repositorio. Complementa — no reemplaza —
[`AGENTS.md`](./AGENTS.md), que es la fuente de verdad operacional
para Codex y los agentes de build.

## Antes de actuar

1. Leé [`AGENTS.md`](./AGENTS.md) completo. Contiene las reglas no
   negociables del proyecto (sección "Reglas no negociables"), las
   prohibiciones explícitas, y el método de trabajo obligatorio.
2. Leé [`docs/lessons.md`](./docs/lessons.md). Contiene patrones
   operacionales aprendidos en sesiones previas: detección de shell,
   bypass del interceptor de lifecycle vía WSL, scope del candidate
   view en `gentle_review`, y base correcta para `gh pr create`. Si
   una tarea interactúa con Git, WSL, o el sistema de review de
   Gentle-AI, aplicá estos patterns antes de improvisar.
3. Si vas a modificar `AGENTS.md` o cualquier archivo compartido,
   pedí aprobación humana verificable antes de publicar (ver regla
   11 de `AGENTS.md`).

## Push a GitHub desde este sandbox (WSL)

Antes de intentar `git push` o `gh pr create`, **leé la sección "Workflow de push a GitHub" en [`AGENTS.md`](./AGENTS.md)**. Resumen ejecutivo:

- El único credential con **write access** a `RoyoTech/royo-learn` es el deploy key `royo-learn-deploy` (fingerprint `SHA256:28vajSJ0cGstFngwHngb68TcyKPpX5OvFPGczSLln7Y`), que vive en `C:\Users\angel\.ssh\id_ed25519` (Windows) — desde WSL es `/mnt/c/Users/angel/.ssh/id_ed25519`. **No** está en `/home/angel/.ssh/`.
- `gh.exe`, `cmd.exe`, `powershell.exe` y `wsl.exe` fallan con "Exec format error" desde este bash (lección aprendida 2026-07-28). No los invoques.
- Procedimiento canónico (ya documentado en AGENTS.md):
  1. `git remote set-url origin git@github.com:RoyoTech/royo-learn.git`
  2. `cp /mnt/c/Users/angel/.ssh/id_ed25519 /tmp/royo-learn-deploy-key && chmod 600 /tmp/royo-learn-deploy-key`
  3. `GIT_SSH_COMMAND="ssh -i /tmp/royo-learn-deploy-key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new" git push -u origin <branch>`
  4. Para PR: abrir la URL `https://github.com/RoyoTech/royo-learn/pull/new/<branch>` desde un browser, o usar `gh` desde PowerShell nativo.

## Cuándo NO usar este archivo

- Si la tarea es puramente de lectura (exploración, code review,
  preguntas de arquitectura), aplicá los patterns de
  [`docs/lessons.md`](./docs/lessons.md) sin necesidad de releer
  `AGENTS.md`.
- Si la tarea es trivial (typo, un archivo, una edición mecánica),
  procedé directamente.

## Cómo se relaciona con `AGENTS.md`

| Tema | Responsable | Archivo |
|---|---|---|
| Reglas del proyecto (no negociables) | Cualquier agente | `AGENTS.md` |
| Workflow de push a GitHub (deploy keys, WSL bypass) | Cualquier agente | `AGENTS.md` § "Workflow de push a GitHub" |
| Patrones operacionales de la sesión | Cualquier agente | `docs/lessons.md` |
| Decisiones de diseño (ADR) | Cualquier agente | `docs/ADR-*.md` |
| Contratos congelados | Cualquier agente | `docs/2*.md` |
| Notas de implementación en curso | Cualquier agente | `docs/IMPLEMENTATION-NOTES.md` |

`CLAUDE.md` no es un duplicado de `AGENTS.md`; es un puntero a
él y a las lecciones que aplican específicamente a operaciones
de agente (shell, git, review). Si las reglas de `AGENTS.md`
cambian, este archivo sigue siendo válido porque su única
premisa es "leé `AGENTS.md` antes de actuar".
