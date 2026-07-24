# Especificación de minería de patrones

- **Estado:** contrato congelado (Hito 0)
- **Principio:** la minería es un **pipeline auditable**, no una llamada única a
  un LLM sobre un diario enorme.
- **Frontera dura:** un patrón nunca se publica ni se instala automáticamente.
  La única salida hacia conocimiento es la **promoción gobernada** (§6).

## 1. Pipeline

```text
ExperienceEvent → normalización → fingerprint determinista
→ recuperación de vecinos → clustering → métricas de recurrencia
→ cualificación → síntesis asistida por agente → revisión → promoción a Learning
```

Cada etapa es reproducible: mismo input + misma versión de detector/miner =
mismo output.

## 2. Detección de eventos (dos capas)

### 2.1 Detectores deterministas (autoridad primaria)

```text
internal/experience/detectors/
    correction.go      # frases explícitas de corrección del usuario
    command_outcome.go # exit code != 0 seguido de versión corregida
    tests.go           # test fail seguido de test pass
    retry.go           # error recurrente idéntico corregido
    tool_limit.go      # fallback tras herramienta no disponible
```

Prioridad: **precisión sobre recall**. Cero eventos en conversación rutinaria.

### 2.2 Detector asistido por host (no confiable)

El adaptador puede aportar eventos sugeridos por el modelo nativo, con identidad
obligatoria:

```json
{ "detector": { "kind": "host_llm", "model": "...", "version": "...", "prompt_hash": "..." } }
```

El núcleo valida enums, limita texto, redacta, no ejecuta comandos, exige
fuentes, **no** eleva evidencia a `strong` y **no** promueve automáticamente.

El prompt de referencia se guarda en `prompts/experience-detection-v1.md` y su
hash se calcula en cada evento generado.

## 3. Fingerprint determinista de patrón

Se construye a partir de: event kind; tokens normalizados del problema;
herramienta/comando principal sin valores volátiles; tipo de resultado; paths
normalizados; extensión/lenguaje; términos de recuperación.

Se **eliminan**: UUID; timestamps; puertos no esenciales; hashes de commit;
paths absolutos de usuario; valores secretos; IDs de tickets circunstanciales.

## 4. Clustering v1 (sin embeddings)

Orden de señales:

1. exact fingerprint;
2. matching por `event kind`;
3. tokens normalizados;
4. similitud Jaccard de términos;
5. coincidencia de herramienta/comando;
6. relaciones existentes de `Learning`;
7. umbral conservador.

**No** se introducen embeddings en el primer PR de minería. La búsqueda
semántica es una fase opcional y posterior con ADR y benchmark previos.

## 5. Cualificación (`observed → qualified`)

Un patrón cualifica **solo si**:

- aparece en ≥ 3 sesiones distintas **o** ≥ 3 días distintos;
- tiene ≥ 2 resultados exitosos o una corrección explícita repetida;
- no está contradicho por evidencia posterior;
- no es un simple hecho o preferencia;
- no está cubierto por un `Learning` vigente;
- no proviene de reintentos técnicos duplicados;
- sus fuentes son trazables;
- la similitud no depende solo de una palabra genérica.

```yaml
patterns:
  enabled: false
  min_distinct_sessions: 3
  min_distinct_days: 2
  min_successful_occurrences: 2
  max_cluster_members: 100
  qualification_mode: conservative
```

## 6. Síntesis y promoción (gobernadas)

Royo-Learn **no** incorpora proveedor LLM obligatorio. La síntesis la hace el
agente cliente:

1. `learning_list_patterns(status=qualified)`;
2. `learning_get_pattern(pattern_id)`;
3. el agente usa `learning_trace` para comprobar comandos y resultados;
4. el agente llama `learning_promote_pattern` con propuesta estructurada.

```json
{
  "pattern_id": "...",
  "title": "...",
  "type": "procedure",
  "context": "...",
  "observation": "...",
  "reusable_lesson": "...",
  "recommended_procedure": ["..."],
  "limits": "...",
  "scope_guess": "project",
  "confidence": "medium",
  "evidence_level": "moderate",
  "proposed_destination": "skill",
  "retrieval_terms": ["..."],
  "actor": {}
}
```

El núcleo debe: re-comprobar el patrón; adjuntar eventos como
evidencia/referencias; **usar `capture.Service`** (no duplicar lógica de
`Learning`); aplicar la deduplicación existente; marcar el patrón `promoted`;
enlazar `ProposedLearningID`; auditar todo.

> Resultado: el `Learning` queda en estado `captured`. **La promoción no
> publica.** El resto del ciclo (evidencia → curación → aprobación → preview →
> publicación → verificación → rollback) es el existente en `docs/01-PRD.md`.

## 7. Dismissal

`learning_dismiss_pattern` con motivo tipado:

```text
one_off · not_reusable · already_covered · contradicted
insufficient_evidence · private_or_sensitive · false_cluster
```

Un patrón descartado no reaparece con los mismos miembros y `detector_version`
salvo evidencia nueva suficiente.

## 8. Errores tipados

```text
pattern_not_found · pattern_not_qualified · pattern_already_promoted
pattern_false_cluster · pattern_insufficient_sources
```

## 9. Invariantes

- La cualificación de una sola sesión con 3 reintentos **no** cualifica.
- Un patrón ya cubierto por un `Learning` no se duplica.
- Una contradicción posterior bloquea la cualificación.
- Todo miembro de un patrón es trazable hasta su turno.
- La promoción es idempotente y deja estado consistente ante error.
- Ningún job de minería aprueba ni publica.
