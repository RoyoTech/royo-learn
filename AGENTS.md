# AGENTS.md — Instrucciones obligatorias para Codex

## Misión

Construir `agent-royo-learn` completo, instalable y probado según esta especificación. No entregar un prototipo decorativo ni reemplazar requisitos por TODOs.

## Fuente de verdad

Orden de precedencia:

1. `docs/14-ACCEPTANCE-CRITERIA.md`
2. `docs/01-PRD.md`
3. `docs/02-ARCHITECTURE.md`
4. contratos específicos de `docs/04` a `docs/10`
5. `docs/lessons.md` — patrones operacionales aprendidos en sesiones previas (shell detection, WSL bypass, review scope, PR base)
6. `TASKS.md`
7. este archivo
8. README

Cuando dos documentos parezcan incompatibles, detener la implementación de esa parte, registrar el conflicto en `docs/IMPLEMENTATION-NOTES.md` y escoger la opción más segura y reversible. No inventar capacidades.

## Reglas no negociables

1. Lenguaje: Go 1.25 o superior.
2. Un solo binario: `royo-learn`.
3. MCP principal por `stdio`.
4. En modo MCP, `stdout` se reserva exclusivamente para mensajes MCP. Logs y diagnósticos van a `stderr`.
5. SQLite mediante `database/sql` y un driver sin CGO, salvo que una prueba multiplataforma demuestre que existe una opción superior.
6. No acceder directamente a `~/.engram/engram.db`.
7. Engram se integra solo mediante interfaz pública local.
8. Gentle-AI no se forkea ni se modifica.
9. No insertar un proveedor LLM dentro del binario en v1.
10. Las operaciones de escritura son `dry-run` por defecto cuando pueden modificar Skills, `AGENTS.md` o bibliotecas compartidas.
11. Toda publicación compartida o cambio de `AGENTS.md` requiere aprobación humana verificable.
12. No ejecutar comandos mediante `sh -c`, `cmd /c` o concatenación de cadenas.
13. Validar rutas contra traversal, symlinks y escapes de raíz.
14. Redactar secretos antes de persistir evidencia.
15. Toda transición de estado debe ser transaccional y auditada.
16. No declarar “terminado” mientras falte un criterio obligatorio.
17. No ocultar fallos de integración: debe existir degradación explícita y observable.
18. No modificar archivos administrados por Gentle-AI.
19. Las Skills del proyecto se mantienen bajo `skills/`.
20. El sistema debe poder funcionar sin red.

## Modelo mental

El binario no decide por sí solo si una experiencia es una verdad universal. El agente, guiado por Skills, envía una propuesta estructurada. El binario valida, persiste, relaciona, controla permisos y aplica cambios aprobados.

```text
LLM + Skill → propuesta semántica
royo-learn → garantía operacional
```

## Método de trabajo obligatorio

### Antes de codificar

- Leer todos los documentos indicados en `CODEX_START_HERE.md`.
- Ejecutar el inventario de dependencias y herramientas.
- Crear `docs/IMPLEMENTATION-NOTES.md`.
- Crear un checkpoint Git inicial.
- Convertir `TASKS.md` en una secuencia ejecutable, sin borrar los criterios.

### Durante la implementación

- Implementar por slices verticales, no por carpetas vacías.
- Cada slice debe incluir dominio, persistencia, CLI/MCP y pruebas cuando corresponda.
- Ejecutar `go test ./...` después de cada slice.
- Ejecutar `go vet ./...`.
- Mantener `go.mod` y `go.sum` reproducibles.
- No agregar dependencias sin registrar la razón en `docs/IMPLEMENTATION-NOTES.md`.
- Preferir biblioteca estándar.
- Mantener funciones pequeñas y errores tipados.
- Todas las escrituras a archivos deben ser atómicas.
- Toda migración debe poder ejecutarse más de una vez sin dañar datos.

### Antes de terminar

Ejecutar exactamente:

```bash
go fmt ./...
go test -race ./...
go vet ./...
royo-learn doctor --json
royo-learn e2e --temp
```

En Windows, si `-race` no es soportado por el entorno, documentar esa limitación y ejecutar la carrera en Linux CI.

También:

- compilar Windows, Linux y macOS;
- probar MCP con el Inspector o con un cliente de prueba;
- probar instalación limpia;
- probar actualización;
- probar desinstalación;
- verificar que Gentle-AI y Engram continúan funcionando;
- revisar que no haya secretos ni rutas personales en fixtures;
- generar `docs/FINAL-IMPLEMENTATION-REPORT.md`.

## Prohibiciones

- No crear un fork de Gentle-AI.
- No crear un fork de Engram.
- No escribir en bases internas de terceros.
- No auto-publicar una regla global.
- No usar embeddings ni base vectorial en v1.
- No capturar conversaciones completas por defecto.
- No guardar razonamiento privado del modelo.
- No modificar globalmente Codex, OpenCode o Claude sin backup y consentimiento.
- No generar un instalador que descargue y ejecute código no fijado sin mostrar procedencia.
- No sustituir pruebas por mocks cuando el comportamiento real pueda verificarse localmente.
- No eliminar aprendizajes: usar estados `rejected`, `superseded` o `archived`.

## Calidad mínima

- Cobertura del paquete de dominio y storage: 80% o más.
- Ramas críticas de publicación, aprobación y path security: 90% o más.
- Pruebas e2e para el ciclo completo.
- Pruebas de compatibilidad Windows path/CRLF.
- Mensajes de error accionables.
- Salidas CLI estables en JSON.
- MCP tools con esquemas estrictos.
- Audit log append-only.

## Regla de entrega

Codex no debe limitarse a explicar cómo hacerlo. Debe crear, compilar, instalar y comprobar el producto dentro del entorno permitido.
