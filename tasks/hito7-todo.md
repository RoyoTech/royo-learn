# Hito 7 — Promotion Execution Plan

## Baseline

- [x] HandOFF-HITO6-CLOSEOUT.md leído completamente.
- [x] `main` HEAD local y remoto: `9f5dee1` (post-Hito 6 housekeeping).
- [x] `origin/main` HEAD: `9f5dee1` (sincronizado).
- [x] Tag `v0.2.0-rc1` sigue siendo el último (correcto: `v0.2.0` espera a Hito 4).
- [x] PRs abiertos: ninguno. PR #23 (Hito 6) cerrado.
- [x] Working tree limpio excepto `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md` (preservado).
- [x] Crear `feat/hito7-promotion` desde `origin/main` (sin tracking remote).

## Implementation — strict sequential TDD

- [x] **Slice 7.0** — scaffold del paquete `internal/experience/promotion/`
  con tipos (`PromotionInput`, `PromotionResult`, `RedactionSummary`,
  `SourceKind`), interface `PromotionService`, struct `Service` +
  `NewService` y stub `Promote`. Tabla de tests del contrato
  (`contract_test.go`) + tests de servicio (`service_test.go`). Nuevo
  `ErrorCode = "promotion_not_implemented"` en `domain/errors.go`.
  Commit: `d9617af`.
- [x] **Slice 7.1** — pipeline de redacción previo al `capture.Service.Ingest`.
  Extiende `internal/evidence` con la redacción de campos derivados
  del pattern (title, observation, retrieval terms, fingerprint
  preview hash). Todo el contenido redactado antes de la inserción.
  Commit: `27c8cd9` (`feat(evidence):`). Lifecycle gap
  operator-accepted per docs/lessons.md §5 (slice 7.1 entry).
- [ ] **Slice 7.2** — `Promote` transaccional: construye `CaptureInput`
  desde el pattern, llama `s.capture.Capture(ctx, projectID, input)`,
  inserta audit row en `audit_events`, actualiza
  `experience_patterns.status = promoted` y
  `proposed_learning_id = learning.ID`. La transacción cubre los
  tres pasos; rollback limpio en cualquier fallo.
- [ ] **Slice 7.3** — idempotencia: lookup pre-insert en
  `experiencle_patterns.proposed_learning_id`. Si el patrón ya
  promovió, retorna el `learning_id` existente con
  `WasNew = false`. Doble `Promote` no produce doble `Learning`.
- [ ] **Slice 7.4** — CLI `experience patterns promote --id <id>` + MCP
  `learning_promote_pattern` (admin-only). Acceptance test con
  patrón sintético que cubre los 3 paths (qualified → Learning,
  already-promoted → idempotente, not-qualified → error).

## Verification gates

- [ ] go build ./... verde en cada slice atómico.
- [ ] go vet ./... verde en cada slice.
- [ ] go test -race -count=1 ./... verde en cada slice.
- [ ] Cobertura `internal/experience/promotion/` ≥ 80% al cierre
      (gate de Hito 7, no de slice 7.0).
- [ ] Cross-build Windows amd64 verde (Linux/macOS no requerido per
      directiva del operador).
- [ ] e2e 37/37 pasos verde antes de merge.
- [ ] gentle_review: aplicar las lecciones de docs/lessons.md §5
      (finalize puede ser silently dropped). Confirmar con el
      operador antes de aceptar el gap.
- [ ] Push y PR (PR #6 per docs/26 §3) pendiente autorización
      explícita del operador.

## Slice 7.1 closure notes

- Branch: `feat/hito7-promotion`, commit `27c8cd9` (post-amend with
  gofmt -w; original commit was `bf10634`).
- 2 archivos cambiados, 546 LoC (impl 174 + tests 372).
- Funciones:
  - `evidence.RedactPromotionFields(*PromotionFields) RedactionReport`
    — redacta los 7 campos derivados (Title, Context, Observation,
    ReusableLesson, Limits, Recommended[i], RetrievalTerms[i]).
    Nil-safe, dedup'd list, determinista.
  - `evidence.PromotionFingerprint(PromotionFields) string` — SHA-256
    64-hex lowercase sobre la forma canónica del bag ya redactado,
    orden-independiente en slices.
- Tests: 7 (4 tests `RedactPromotionFields_*`, 3 tests
      `PromotionFingerprint_*`).
  Cobertura funciones nuevas: `RedactPromotionFields` 100%,
  `PromotionFingerprint` 90% (rama imposible de json.Marshal
  deliberadamente no ejercitada).
- Gates: `go test -race ./internal/{evidence,capture,experience}/...`
  verde; `go vet ./...` limpio; `gofmt -l` limpio (requirió amend con
  `gofmt -w` por newline final + alineación de map keys en el test).
- Lifecycle: `gentle_review validate` returned `receipt.status:
  not_applicable`, `applicability: ambiguous` (esperado: no hay
  lineage para el changeset de slice 7.1 aún). Patrón del handoff:
  operador aceptó el gap en su responsabilidad; documentado en
  docs/lessons.md §5.
- Flake MCP observado (TestMCP_Rollback_NotServedInReadOrAgent) pasa
  aislado (3.7s), falla bajo carga. Pre-existente, ADR-0002.
- No tocar `internal/experience/promotion` en este slice (queda en
  51.2% scaffold; ≥ 80% aplica al cierre del Hito 7).

## Slice 7.0 closure notes

- Branch: `feat/hito7-promotion`, commit `d9617af`.
- 6 archivos cambiados, 636 LoC.
- Tests del contrato: 12+ (PromotionInput.Validate, SourceKind.Enum,
  IsValidSourceKind, PromotionResult JSON, RedactionSummary JSON,
  PromotionService interface conformance, typed errors distinctness,
  ErrorIs, formatPromotionContext, MaxPromotionNoteBytes).
- Tests de servicio: 2 (NewService nil args, nil receiver guard).
- Cobertura paquete: 51.2% (scaffold; el gate de 80% aplica al Hito
  7 entero, slices 7.0–7.4).
- Build + vet clean.
- Domain errors: `ErrPromotionNotImplemented ErrorCode = "promotion_not_implemented"`
  agregado al enum + `AllErrorCodes()`.
- `docs/17-ERROR-CODES.md` actualizado.

## Outstanding risks

- Slice 7.2 requiere transacción atómica que cruza la frontera
  `capture.Service` (que ya abre su propia tx). Opciones: (a) el
  promotion service abre tx, captura sin tx interna, promote guarda
  pattern; (b) la captura se hace en tx separada antes de la tx de
  promotion. La decisión se define en slice 7.2 (TDD-led).
- Slice 7.3 lookup pre-insert puede tener race condition si dos
  `Promote` concurrentes se llaman. SQLite serializa escrituras,
  pero el patrón debe verificar el `Revision` antes de update.
- Slice 7.4 CLI/MCP integration requiere extender `internal/mcpserver/profiles.go`
  - `internal/mcpserver/tools.go` con `learning_promote_pattern` y
  `experience_patterns.go` con el subcommand `promote`. El patrón
  de Hito 6 (`learning_list_patterns`, `learning_get_pattern`,
  `learning_dismiss_pattern`) sirve como template.
- `gentle_review finalize` puede ser silently dropped otra vez
  (docs/lessons.md §5). Confirmar con el operador antes de aceptar
  el gap.

## Review

_To be completed when Hito 7 closes._
