# Integración con Engram

## Objetivo

Aprovechar memoria existente sin duplicarla ni corromperla.

## Regla principal

> Nunca abrir, leer ni escribir directamente la base SQLite de Engram.

## Transporte v1

HTTP local:

```text
http://127.0.0.1:7437
```

Configurable, pero solo se permite loopback por defecto.

## Endpoints utilizados

### Health

```http
GET /health
```

### Buscar antecedentes

```http
GET /search?q=<query>&project=<project>&scope=project&limit=10
```

También puede buscar `personal` o `global` cuando la Skill lo solicite.

### Contexto

```http
GET /context?project=<project>&scope=project
```

### Guardar referencia opcional

```http
POST /observations
```

Contenido recomendado:

```json
{
  "session_id": "<known-session-or-generated>",
  "type": "learning",
  "title": "Published learning: <title>",
  "content": "**What**: ...\n**Why**: ...\n**Where**: ...\n**Learned**: Canonical record at ...",
  "project": "<project>",
  "scope": "project",
  "topic_key": "learning/<stable-key>"
}
```

La escritura a Engram:

- está desactivada por defecto;
- nunca bloquea la publicación local si falla;
- no copia evidencia sensible;
- guarda solo una referencia breve al artifact canónico.

## Arranque

`royo-learn` no debe iniciar servicios ocultos por defecto.

Política:

1. comprobar `/health`;
2. si no responde, marcar degradación;
3. solo `royo-learn doctor --fix-safe` puede intentar ejecutar `engram serve` cuando configuración lo autoriza;
4. registrar PID únicamente para procesos iniciados por royo-learn;
5. no matar procesos ajenos.

## Resolución de proyecto

Preferir project key propia y mapear a Engram mediante:

- `.engram/config.json`;
- respuesta de `/project/current?cwd=...`;
- Git remote/root.

Si Engram devuelve ambigüedad, no elegir. Mostrar opciones.

## Deduplicación

Engram ya deduplica sus observaciones. Agent Royo Learn mantiene su identidad propia. Un vínculo usa:

```text
source = engram
external_id = observation ID
external_uri = engram://observation/<id>
```

No convertir automáticamente cada memoria en aprendizaje.

## Degradación

Códigos:

```text
not_installed
service_not_running
connection_refused
timeout
unknown_project
ambiguous_project
api_incompatible
write_disabled
```

## Pruebas

- fake HTTP server;
- Engram real cuando esté instalado;
- health;
- búsqueda;
- timeout;
- respuesta inválida;
- ambigüedad;
- ausencia;
- no acceso al DB;
- redacción antes de POST.
