# CODEX_START_HERE

## Objetivo de esta ejecución

Construir el repositorio completo `agent-royo-learn`, instalar el binario localmente, registrar su servidor MCP en Codex y demostrar mediante pruebas que:

1. captura un aprendizaje;
2. consulta antecedentes propios y de Engram cuando está disponible;
3. permite curarlo;
4. produce una previsualización de publicación;
5. exige aprobación cuando corresponde;
6. publica de forma atómica;
7. verifica el resultado;
8. registra una reincidencia;
9. no rompe Gentle-AI ni Engram.

## Lectura obligatoria

Leer en este orden:

```text
AGENTS.md
docs/01-PRD.md
docs/02-ARCHITECTURE.md
docs/03-DOMAIN-MODEL.md
docs/04-CLI-SPEC.md
docs/05-MCP-SPEC.md
docs/06-DATABASE-SCHEMA.md
docs/07-ENGRAM-INTEGRATION.md
docs/08-GENTLE-AI-CODEX-INTEGRATION.md
docs/09-PUBLISHING-ENGINE.md
docs/10-SECURITY.md
docs/11-INSTALLATION.md
docs/12-TEST-PLAN.md
docs/13-IMPLEMENTATION-PLAN.md
docs/14-ACCEPTANCE-CRITERIA.md
TASKS.md
```

## Comandos iniciales

Codex debe comprobar:

```bash
git --version
go version
codex --version
gentle-ai --version
engram --version
```

La ausencia de Gentle-AI o Engram no bloquea la construcción. Debe registrarse como integración no disponible y continuar.

## Dependencias elegidas

- Go 1.25+
- SDK oficial MCP para Go
- `database/sql`
- SQLite sin CGO
- YAML v3 para configuración
- biblioteca estándar para HTTP, Git y procesos
- framework CLI: se permite Cobra, pero solo si la reducción de complejidad compensa la dependencia

Codex debe pinchar versiones estables en `go.mod`, ejecutar `go mod tidy` y registrar versiones en `docs/IMPLEMENTATION-NOTES.md`.

## Entrega esperada

La ejecución solo finaliza cuando existe:

```text
bin/royo-learn
dist/royo-learn_windows_amd64/royo-learn.exe
dist/royo-learn_linux_amd64/royo-learn
dist/royo-learn_darwin_arm64/royo-learn
```

y pasa el criterio de aceptación completo.
