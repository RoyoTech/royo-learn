# ADR-0001 — MemSearch no es runtime de Royo-Learn

- **Estado:** aceptada
- **Fecha:** 2026-07-21
- **Contexto de decisión:** Hito 0 del plan maestro `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`
- **Repositorio de referencia estudiado:** `zilliztech/memsearch`
- **Alcance:** toda la capa de descubrimiento de experiencia (ingestión, estabilidad de turnos, checkpoints, recuperación progresiva, minería de patrones, salud de jobs/índices)

## 1. Contexto

Royo-Learn gobierna aprendizajes desde que un aprendizaje estructurado entra al
sistema: captura idempotente, evidencia, curación, aprobación ligada a
`preview_hash`, publicación atómica, verificación, rollback y auditoría. Su
debilidad está **antes** de esa entrada: depende de que el agente recuerde
llamar a `capture-learning` e interprete correctamente la experiencia.

MemSearch resuelve mejor la mitad de entrada y recuperación: lee sesiones de
harnesses automáticamente, mantiene checkpoints por sesión/turno, espera a que
el último turno se estabilice, conserva procedencia al transcript, descubre
recurrencias y expone recuperación progresiva. Sin embargo, su gobernanza de
aprendizaje es más débil: trata Markdown como fuente de verdad, puede convertir
una conversación en Skill sin curación humana, y su stack (Milvus, Python, Bash,
`os.system`, red) es incompatible con los invariantes de Royo-Learn.

La tentación natural sería instalar MemSearch junto a Royo-Learn e integrarlos.
Esta ADR rechaza esa opción de forma explícita y permanente.

## 2. Decisión

**MemSearch no se instala, no se ejecuta, no se importa y no se mantiene en
paralelo.** Se estudia como referencia de ingeniería y se **reimplementan en Go**
únicamente sus capacidades superiores, bajo los contratos y garantías existentes
de Royo-Learn.

Concretamente:

1. MemSearch **no** aparece en `go.mod`, instaladores, imágenes, scripts ni
   runtime de Royo-Learn.
2. No se incorpora Milvus, Zilliz Cloud, ni ningún proveedor vectorial
   obligatorio.
3. No se introduce Python, `bash -c`, `exec`, `os.system` ni interpolación de
   shell en ninguna ruta de código.
4. No se introduce un daemon obligatorio ni una dependencia de red obligatoria.
5. Markdown, Skills e índices siguen siendo **proyecciones/derivados**; SQLite +
   modelo de dominio siguen siendo la **verdad operacional**.
6. Ninguna conversación se convierte en Skill sin pasar por promoción → captura
   (`capture.Service`) → curación → aprobación → publicación.
7. La búsqueda vectorial nunca es requisito para que el producto funcione.

Lo que **sí** se reimplementa en Go, como capacidad propia y testeable:

| Capacidad tomada como idea | Reimplementación en Royo-Learn |
|---|---|
| Ingestión automática de sesiones | Adaptadores de plataforma read-only + `internal/experience` |
| Estabilidad del último turno | Lógica de dominio determinista (`stability`) con `tail_quiet_period` configurable |
| Checkpoints por sesión/turno | `IngestionCursor` reconstruible en SQLite |
| Procedencia al original | `TranscriptLocator` + tabla `learning_experience_sources` |
| Recuperación progresiva | `search → get → trace` |
| Candidatos de workflow recurrente | `ExperiencePattern` inerte, nunca instalado |
| Salud de job/índice | `JobState` con `ok/degraded/error` en SQLite |

## 3. Alternativas consideradas y rechazadas

- **A. Instalar MemSearch como servicio adjunto.** Rechazada: importa Milvus,
  Python, Bash y red; duplica la autoridad de aprendizaje; degrada la gobernanza
  de Royo-Learn; rompe el soporte multiplataforma (Windows).
- **B. Vendorizar módulos Python de MemSearch invocados por CLI.** Rechazada:
  reintroduce `os.system`/shell, dependencia de intérprete y superficie de
  inyección; contradice el invariante "sin shell".
- **C. Portar el stack vectorial primero.** Rechazada: la búsqueda semántica es
  opcional y derivada; debe existir un benchmark lexical previo (Hito 9/11) antes
  de considerar embeddings. Ver `docs/23-PATTERN-MINING.md` §clustering v1.
- **D. Adoptar Markdown como fuente de verdad de recuerdos.** Rechazada:
  contradice `docs/02-ARCHITECTURE.md` §8; SQLite es la fuente operacional y los
  records son proyecciones auditables reconstruibles.

## 4. Consecuencias

**Positivas**

- Un binario Go único, sin runtime externo, se mantiene en Windows/Linux/macOS.
- Las garantías actuales (evidencia, redacción, aprobación por hash, publicación
  atómica, verificación, rollback, auditoría) permanecen como autoridad única.
- La experiencia observada entra como **evidencia preliminar**, nunca como
  conocimiento aprobado ni como instalación automática.

**Costos / obligaciones**

- Hay que reimplementar en Go la lógica de turnos, estabilidad, cursores y
  minería, con fixtures reales anonimizados y tests de raza.
- Cada adaptador vive detrás de una versión de esquema (`opencode/sqlite-v1`,
  etc.); un cambio upstream produce `experience_source_schema_unsupported`, no
  una rotura silenciosa.

## 5. Cumplimiento y verificación

Esta ADR se considera violada —y el PR debe rechazarse— si:

- aparece `memsearch` en `go.mod`, instaladores o runtime;
- se introduce `python3`, `bash -c`, `exec` u `os.system`;
- se crea una Skill desde un transcript sin promoción y curación;
- se almacena transcript completo en SQLite sin opt-in explícito;
- un adaptador escribe en la DB de OpenCode/Claude/Codex/Pi;
- se añade una base vectorial antes de existir un benchmark lexical;
- un job publica o aprueba;
- la configuración del repositorio amplía trust roots o redirige credenciales.

## 6. Referencias

- `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` §0.1, §3.1, §31, §36
- `docs/02-ARCHITECTURE.md` §8, §10
- `docs/24-EXPERIENCE-THREAT-MODEL.md`
- `docs/16-DECISIONS.md`
