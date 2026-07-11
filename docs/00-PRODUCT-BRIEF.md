# Product Brief

## Nombre

- Repositorio: `agent-royo-learn`
- Producto: Agent Royo Learn
- Binario: `royo-learn`
- Servidor MCP: `royo-learn`
- Namespace de tools: nombres con prefijo `learning_`

## Propuesta

Convertir experiencia acumulada en cambios verificables del comportamiento futuro de agentes.

## Diferencia entre memoria y aprendizaje

| Capa | Pregunta | Ejemplo |
|---|---|---|
| Memoria | ¿Qué ocurrió? | “La migración falló porque ya estaba aplicada.” |
| Conocimiento | ¿Qué significa? | “Las migraciones ejecutadas son inmutables.” |
| Procedimiento | ¿Qué hacer? | “Consultar estado, crear una nueva migración y probar rollback.” |
| Regla | ¿Qué se exige siempre? | “No modificar migraciones aplicadas.” |
| Evaluación | ¿Funcionó la enseñanza? | “El error reapareció después de publicar la regla.” |

Agent Royo Learn cubre las últimas cuatro sin duplicar la función de Engram.

## Usuarios

- desarrolladores individuales;
- equipos con varios agentes;
- usuarios de Codex, OpenCode, Claude Code, Pi u otros clientes MCP;
- proyectos gestionados por Gentle-AI;
- proyectos sin Gentle-AI.

## Resultado

Una corrección explicada una vez debe poder convertirse en una capacidad reusable y medible, sin contaminar indiscriminadamente la memoria de todos los agentes.
