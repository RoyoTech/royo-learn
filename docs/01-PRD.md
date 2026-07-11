# PRD — Agent Royo Learn v1

## 1. Problema

Los agentes guardan contexto, pero muchas enseñanzas se pierden entre sesiones:

- el usuario vuelve a corregir el mismo comportamiento;
- un bug resuelto no se convierte en prevención;
- una solución queda enterrada en una conversación;
- cada agente reconstruye procedimientos ya descubiertos;
- `AGENTS.md` termina saturado porque no existe una clasificación;
- memorias específicas se aplican erróneamente como reglas universales;
- no hay medición de si el error reaparece.

## 2. Objetivo

Crear un motor local, agnóstico de agente, que gestione un ciclo auditable desde la experiencia hasta una mejora reusable.

## 3. No objetivos de v1

- entrenar modelos;
- modificar pesos;
- evolución automática estilo GEA;
- almacenar conversaciones completas;
- sustituir Engram;
- sustituir Gentle-AI;
- sincronización cloud propia;
- base vectorial;
- panel web;
- proveedor LLM embebido;
- autoedición irrestricta del sistema del usuario.

## 4. Casos de uso obligatorios

### CU-01: Capturar un error corregido

El agente envía un aprendizaje con contexto, observación, lección y evidencia. El sistema crea un candidato idempotente.

### CU-02: Buscar antes de repetir

El agente consulta aprendizajes propios y, opcionalmente, antecedentes de Engram. El sistema devuelve resultados con estado, alcance y relaciones.

### CU-03: Curar

El agente decide si el candidato se rechaza, necesita evidencia, se fusiona o se aprueba para un destino.

### CU-04: Publicar una Skill

El agente propone contenido completo o parche. El sistema valida frontmatter, ruta, duplicados y aprobación; crea un preview; aplica de forma atómica; actualiza el registro de Skills si Gentle-AI está disponible.

### CU-05: Publicar una regla

Solo permite una regla breve dentro de un bloque administrado. Exige aprobación humana.

### CU-06: Registrar una reincidencia

Al reaparecer el problema, el agente registra la ocurrencia y la vincula con el aprendizaje. El sistema incrementa métricas y puede marcar la enseñanza como ineficaz.

### CU-07: Funcionar sin Engram

Todo el ciclo debe funcionar con el almacenamiento propio.

### CU-08: Integrarse con Codex

Codex descubre las herramientas MCP y puede ejecutar el ciclo sin llamadas shell ad hoc.

## 5. Requisitos funcionales

### RF-001 Proyecto

Resolver la raíz del proyecto por:

1. argumento explícito;
2. `cwd` dentro de Git;
3. raíz MCP informada por el cliente;
4. configuración `.royo-learn/config.yaml`.

Si hay ambigüedad, fallar. Nunca elegir al azar.

### RF-002 Captura

Aceptar:

- título;
- tipo;
- contexto;
- observación;
- lección reusable;
- procedimiento;
- límites;
- evidencia;
- términos de búsqueda;
- alcance propuesto;
- destino propuesto;
- actor/modelo/sesión;
- clave de idempotencia.

### RF-003 Evidencia

Soportar referencias a:

- archivo;
- commit;
- diff Git;
- comando y resultado;
- prueba;
- Engram observation ID;
- issue/PR;
- texto breve suministrado.

No almacenar secretos.

### RF-004 Deduplicación

- hash normalizado exacto;
- FTS5 lexical;
- relaciones semánticas suministradas por el agente;
- `duplicate_of`, `extends`, `supersedes`, `contradicts`, `narrows`, `related`.

### RF-005 Estado

Transiciones válidas:

```text
captured
  ├── needs_evidence
  ├── rejected
  ├── approved
  └── merged

needs_evidence
  ├── approved
  ├── rejected
  └── needs_evidence

approved
  ├── published
  ├── superseded
  └── rejected

published
  ├── superseded
  └── archived
```

No permitir saltos fuera de esta máquina de estados.

### RF-006 Preview

Toda publicación produce:

- archivos afectados;
- diff;
- hash SHA-256;
- riesgos;
- comandos de verificación;
- requisito de aprobación;
- plan de rollback.

### RF-007 Aprobación

Aprobación ligada a:

- ID de aprendizaje;
- hash exacto del preview;
- actor;
- fecha;
- razón;
- vencimiento opcional.

Cambiar el preview invalida la aprobación.

### RF-008 Publicación

Destinos:

- conocimiento de proyecto;
- conocimiento compartido;
- nueva Skill;
- actualización de Skill;
- regla administrada en `AGENTS.md`;
- archivo de prueba/regresión;
- ninguno.

### RF-009 Auditoría

Registrar toda acción relevante con:

- timestamp UTC;
- actor;
- comando/tool;
- entidad;
- estado anterior;
- estado nuevo;
- hash de payload;
- resultado;
- error tipado.

### RF-010 Reincidencia

Registrar una ocurrencia con:

- fingerprint;
- aprendizaje relacionado;
- fecha;
- proyecto;
- evidencia;
- si la regla fue recuperada;
- si la Skill se activó;
- causa probable.

### RF-011 Exportación

Exportar e importar JSONL y Markdown auditable sin depender de IDs autoincrementales.

### RF-012 Doctor

Comprobar:

- DB;
- migraciones;
- permisos;
- Git;
- configuración;
- Engram;
- Gentle-AI;
- Skill registry;
- Codex MCP;
- rutas compartidas;
- integridad de records.

## 6. Requisitos no funcionales

- local-first;
- multiplataforma;
- arranque MCP inferior a 500 ms en equipo normal;
- operaciones locales habituales inferiores a 250 ms sin Engram;
- cero red obligatoria;
- salida JSON estable;
- migraciones versionadas;
- escritura atómica;
- respaldo antes de publicación;
- compatibilidad CRLF;
- logs estructurados a `stderr`;
- datos UTC;
- IDs ULID o UUID estables;
- máximo de payload configurable;
- no más de 1 MB por respuesta MCP.

## 7. Métricas del producto

- candidatos capturados;
- porcentaje aprobado/rechazado;
- duplicados evitados;
- aprendizajes publicados;
- reincidencias antes/después;
- tiempo desde captura a publicación;
- Skill más reutilizada;
- publicaciones revertidas;
- búsquedas con resultado;
- tasa de recuperación previa a ejecutar una tarea.

## 8. Criterio de éxito

Se considera exitoso cuando un aprendizaje de un proyecto puede:

1. capturarse;
2. aprobarse con evidencia;
3. convertirse en Skill o regla;
4. ser recuperado en otra sesión;
5. impedir o detectar una repetición;
6. demostrarlo mediante auditoría.
