# Plan de pruebas

## Pirámide

### Unitarias

- normalización;
- hashes;
- estados;
- políticas;
- redacción;
- path security;
- fingerprint;
- preview canonicalization;
- config precedence;
- error mapping.

### Integración

- SQLite real temporal;
- migrations;
- FTS5;
- atomic writes;
- backups;
- Git repo temporal;
- Engram fake;
- Gentle-AI fake;
- MCP stdio.

### E2E

Caso obligatorio:

1. crear repo Git temporal;
2. `royo-learn init`;
3. capturar aprendizaje;
4. comprobar similar search;
5. curar como nueva Skill;
6. generar preview;
7. comprobar bloqueo sin aprobación;
8. aprobar;
9. publicar;
10. validar Skill;
11. registrar reincidencia;
12. consultar métricas;
13. rollback;
14. verificar restauración.

## Matriz

| Área | Linux | Windows | macOS |
|---|---:|---:|---:|
| build | sí | sí | sí |
| unit | sí | sí | sí |
| SQLite/FTS | sí | sí | sí |
| paths/symlinks | sí | sí | sí |
| MCP stdio | sí | sí | sí |
| installer | sí | sí | sí |
| Gentle-AI optional | sí | sí | smoke |
| Engram optional | sí | sí | smoke |

## Golden tests

- salida JSON;
- Markdown record;
- preview diff;
- Skill generated;
- AGENTS block;
- error envelopes.

## Property/fuzz tests

- normalización;
- path input;
- YAML frontmatter;
- redaction;
- JSON MCP inputs;
- record parser.

## Concurrencia

- dos captures mismas idempotency keys;
- dos previews;
- publish simultáneo;
- lock huérfano;
- SQLite busy.

## Fault injection

- disco lleno simulado;
- rename falla;
- verification command timeout;
- Engram cae;
- target cambia tras preview;
- DB locked;
- record corrupto.

## Performance

Dataset:

- 10.000 aprendizajes;
- 50.000 evidencias;
- 30.000 ocurrencias.

Objetivos:

- search p95 < 250 ms;
- get p95 < 50 ms;
- capture p95 < 300 ms sin evidencia externa;
- startup MCP < 500 ms.

## Coverage

- domain/storage >= 80%;
- approval/publish/security >= 90%;
- no fijar cobertura global artificial si reduce calidad.

## CI

Jobs:

```text
lint
unit-linux
unit-windows
unit-macos
race-linux
integration
e2e
govulncheck
build-release
```

## Prueba manual de Codex

1. registrar MCP;
2. abrir Codex en repo temporal;
3. `/mcp` o equivalente;
4. confirmar tools;
5. pedir “registra este aprendizaje”;
6. revisar que use `learning_capture`;
7. pedir publicación;
8. confirmar que solicita aprobación.
