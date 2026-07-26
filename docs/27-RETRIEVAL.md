# Retrieval (Hito 9)

> Contrato público del paquete `internal/retrieval/`. Este documento es
> la fuente de verdad para el ranking lexical de Hito 9; cualquier
> cambio en los pesos, las reglas de sanitización o el orden de
> tiebreaker debe venir acompañado de una edición aquí.

- **Estado**: EN PROGRESO (rama `feat/hito9-retrieval` desde `dba80e1`).
- **PR estimada**: #9.
- **Cross-ref**: [`docs/26-IMPLEMENTATION-ROADMAP.md`](./26-IMPLEMENTATION-ROADMAP.md)
  §3-§4 (PR 9 dentro de Ola 2), `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md`
  §Hito 9, [`docs/12`](./12) §108 (gate p95 < 250 ms).

## 1. Objetivo

Reemplazar la búsqueda FTS5 "raw" (que perdía keywords literales, no
validaba longitud ni caracteres de control, y no prevenía path
traversal) por un pipeline en Go con:

1. **Sanitización endurecida** de la consulta — whitelist por
   carácter, límite de 16 términos, dedupe preservando orden,
   escape FTS5 con doble-comilla.
2. **Ranking compuesto** con cinco componentes aditivos y pesos
   fijos v1.
3. **Determinismo** del orden — tiebreaker
   `(score DESC, fingerprint ASC, id ASC)`.
4. **Score visible** en CLI y MCP — `score` y `score_components`
   son aditivos, no rompen consumidores.

Sin migración nueva (`learnings_fts` ya existía en `001_init.sql`).
Sin error codes nuevos (reutiliza `ErrInvalidArgument`).

## 2. API pública

### 2.1 Tipos

```go
// internal/retrieval/types.go
type Query struct {
    Text      string
    Limit     int
    Offset    int
    ProjectID domain.ProjectID
}

type Weights struct {
    BM25           float64 `json:"bm25"`
    RetrievalTerms float64 `json:"retrieval_terms"`
    TitleExact     float64 `json:"title_exact"`
    EvidenceLevel  float64 `json:"evidence_level"`
    Recency        float64 `json:"recency"`
}

type ScoreComponents struct {
    BM25           float64 `json:"bm25"`
    RetrievalTerms float64 `json:"retrieval_terms"`
    TitleExact     float64 `json:"title_exact"`
    EvidenceLevel  float64 `json:"evidence_level"`
    Recency        float64 `json:"recency"`
}

type Hit struct {
    Learning   *domain.Learning
    Score      float64
    Components ScoreComponents
}

type Result struct {
    Hits   []Hit
    Total  int
    Query  string
    TookMS int64
}
```

### 2.2 Constantes y errores

| Símbolo            | Valor       | Significado                                     |
|--------------------|-------------|-------------------------------------------------|
| `DefaultLimit`     | `50`        | `q.Limit <= 0` → 50.                            |
| `MaxLimit`         | `200`       | `q.Limit > 200` → capeado silenciosamente.      |
| `MaxTermsPerQuery` | `16`        | Sanitización falla con `ErrTooManyTerms`.       |
| `MaxTermLength`    | `256`       | Términos más largos se descartan sin error.     |
| `ErrTooManyTerms`  | sentinel    | Devuelto por `Sanitize` cuando hay >16 tokens.  |
| `ErrNotImplemented`| sentinel    | Devuelto por `SearchWithEngram` (stub Hito 10). |

### 2.3 Constructores

```go
// Service
repo  := retrieval.NewRepository(db)            // envuelve storage.DB
svc   := retrieval.NewService(repo, retrieval.DefaultWeights())
svc.SetNow(func() time.Time { return time.Now() }) // opcional, inyectable

// Resultado
res, err := svc.Search(ctx, retrieval.Query{
    Text:      "kubernetes deployment",
    ProjectID: projectID,
    Limit:     50,
    Offset:    0,
})
```

## 3. Score components

Pesos v1 cerrados (suma = 1.0). Validado por
`Weights.Validate()` (tolerancia ±0.001).

| Componente       | Peso  | Rango | Regla                                                                                                |
|------------------|-------|-------|------------------------------------------------------------------------------------------------------|
| `bm25`           | 0.50  | [0,1] | `1 / (1 + |raw| / max_abs)` sobre el pool actual.                                                   |
| `retrieval_terms`| 0.20  | {0,1} | `1.0` si la intersección normalizada con `patterns.NormalizeRetrievalTerms` no es vacía, si no `0`. |
| `title_exact`    | 0.15  | {0,1} | `1.0` si `EqualFold(Trim(title), Trim(query))`.                                                      |
| `evidence_level` | 0.10  | [0,1] | `strong=1.0`, `moderate=0.7`, `weak=0.4`, `insufficient=0.1`, desconocido=`0.5`.                     |
| `recency`        | 0.05  | [0,1] | `1.0` si `<7 días`; decaimiento lineal `1 - min(1, días/365)` en otro caso.                        |

`Score = Σ pesos[i] × componente[i]`.

### 3.1 Determinismo

El ranking es `sort.SliceStable` por:

1. `score DESC`
2. `fingerprint ASC`
3. `id ASC`

La misma consulta contra la misma base produce el mismo orden byte
a byte en ejecuciones repetidas.

### 3.2 Normalización de términos

Ambos lados de la intersección (`query_terms` y
`retrieval_terms` del candidato) se procesan con
`patterns.NormalizeRetrievalTerms`:

- lowercase, trim, sort, dedupe
- strip UUIDs, puertos, hashes, paths absolutos, redacted markers
- strip keywords sensibles (`secret`, `password`, `token`...)

El contrato compartido está en
[`internal/experience/patterns/fingerprint.go`](../internal/experience/patterns/fingerprint.go).

## 4. Internacionalización

Tokenizador `unicode61` (vigente desde `001_init.sql`). Sin stemmer
en v1 (cambia el schema; queda como mejora de Ola 3 según
`docs/26` §5).

Consequences documentadas:

- ES: palabras acentuadas se tokenizan correctamente; plurales y
  conjugaciones NO colapsan. Una consulta por `búsqueda` no
  matchea una fila con `buscar`.
- EN: stemming ausente — `retrieval` no matchea `retrieve`. La
  intersección con `retrieval_terms` (que se conserva literalmente)
  compensa parcialmente.

## 5. Sanitización

### 5.1 Pipeline

1. `tokenize` parte en whitespace Unicode Y en cualquier rune que
   no esté en `[A-Za-zÀ-ÿ0-9_.-]` (whitelist).
2. Por cada término, `reject` descarta cuando:
   - está vacío,
   - es `..` o empieza con `/`,
   - mide más de 256 caracteres,
   - falla la whitelist.
3. Dedupe preservando orden de primera aparición.
4. Si la entrada cruda tenía más de 16 tokens, retorna
   `ErrTooManyTerms`.
5. Entrada vacía o totalmente filtrada → `[]` sin error.

### 5.2 Diferencias con `sanitizeFTS` previo

| Comportamiento              | Antes                | Ahora                                       |
|----------------------------|----------------------|---------------------------------------------|
| `AND`, `OR`, `NOT`, `NEAR` | eliminados           | sobreviven como términos literales          |
| `..`, `/etc/passwd`        | pasan al FTS5        | `..` filtrado; `/etc/...` filtra `/etc`     |
| longitud por término       | sin límite           | 256 chars máx                               |
| control chars (`\x00`...)  | pasan al FTS5        | descartan el término                        |
| comillas dobles (`"`)      | eliminadas           | escapadas con `""` (estándar FTS5)          |
| conteo de tokens           | sin límite           | 16 máx, `ErrTooManyTerms` explícito         |
| dedupe                     | no                   | sí, preservando orden                       |

### 5.3 Errores

```go
terms, err := retrieval.Sanitize("foo bar baz")
if errors.Is(err, retrieval.ErrTooManyTerms) {
    // el caller envuelve como domain.NewValidationError(ErrInvalidArgument, ...)
}
```

`Service.Search` lo wrappea automáticamente como
`domain.NewValidationError(domain.ErrInvalidArgument, ...)`, que la
CLI/MCP mapean a exit code 2.

## 6. Performance

Benchmarks en Windows / AMD Ryzen 7 7730U / Go 1.26.5:

```
BenchmarkService_Search-16     2   7.151.850 ns/op   827.232 B/op   15.501 allocs/op
BenchmarkRepository_Search-16  2   4.395.550 ns/op   165.848 B/op    3.629 allocs/op
BenchmarkSanitize-16           2       3.350 ns/op     1.316 B/op        3 allocs/op
```

Dataset sintético: 1 000 learnings. Query: `alpha beta gamma` (3
términos). Gate `p95 < 250 ms` cumplido con margen de ~35×.

Para reproducir:

```bash
go test -bench=BenchmarkService_Search -benchmem -count=3 ./internal/retrieval/
```

## 7. Riesgos y no-go (Hito 9)

- **`gentle_review finalize` drop** (lessons.md entry 5): si la
  final del review de código se cae, los gates standalone
  (gofmt/vet/test -race/cross-build) son la evidencia mínima
  aceptable.
- **Triggers FTS5**: `learnings_ai/_au/_ad` de `001_init.sql` no se
  modifican. La búsqueda los respeta tal como están.
- **`storage.Search` deprecado**: se conserva por compatibilidad
  con planes previos. Marcado con `// Deprecated:` y redirección
  a `retrieval.Service`. Remoción pendiente.
- **`SearchWithEngram`**: stub `ErrNotImplemented`. La
  integración con Engram queda para Hito 10 según
  [`docs/26` §3](./26-IMPLEMENTATION-ROADMAP.md).
- **Benchmarks en CI**: el primer `Benchmark*` del repo lo
  escribe este PR. CI runners públicos (Linux + macOS) son más
  lentos que el workstation Windows local; el gate p95 < 250 ms
  debe re-evaluarse en el primer CI run.

## 8. Test data

Fixtures en [`internal/retrieval/testdata/`](../internal/retrieval/testdata/):

- `learnings_es.json` — 15 entradas en español cubriendo cada
  decisión de diseño.
- `learnings_en.json` — 15 entradas en inglés equivalentes.
- `queries.json` — 12 queries etiquetadas con `expected_ids` y
  `relevance` para smoke tests manuales y/o golden-file
  comparaciones futuras.

## 9. Compatibilidad

- CLI `royo-learn search` ahora expone `score` y `score_components`
  por hit — aditivo, los consumidores existentes siguen
  funcionando (decodifican JSON tolerante a campos extra).
- MCP `learning_search` requiere `include_components: true`
  explícito para incluir `score_components`. `score` siempre se
  emite. El campo nuevo `include_components` es opcional y
  default-a-`false` para preservar clientes que asumen shape
  estable.

## 10. Referencias internas

- [`docs/26-IMPLEMENTATION-ROADMAP.md`](./26-IMPLEMENTATION-ROADMAP.md)
  — ruta y gates.
- [`docs/12`](./12) §108 — gate p95.
- [`docs/15-OPERATIONS.md`](./15-OPERATIONS.md) — rutina diaria.
- [`internal/experience/patterns/fingerprint.go`](../internal/experience/patterns/fingerprint.go)
  — `NormalizeRetrievalTerms`.
- [`internal/storage/migrations/001_init.sql`](../internal/storage/migrations/001_init.sql)
  — esquema FTS5 (no modificado).
- [`internal/storage/fts.go`](../internal/storage/fts.go) — `Search`
  deprecado.
