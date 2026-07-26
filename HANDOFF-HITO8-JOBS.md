# HANDOFF — Royo-Learn post-Hito 8 → arranque de Hito 9 (retrieval lexical)

> Documento de continuidad entre sesiones. Léelo completo antes de tocar
> nada. Hito 8 (motor de jobs) está implementado en el working tree actual
> (NO commiteado, NO pusheado). Ola 1 está cerrada (v0.2.0 en main).

## 0. Frase para pegar en la próxima sesión

```
Continuá Royo-Learn desde el working tree actual (rama main, tag v0.2.0).
Hito 8 (motor de jobs) está implementado pero NO commiteado:
- 10 archivos nuevos en internal/experience/jobs/
- 6 archivos modificados (CLI, MCP, domain/errors, migrations)
- 34 tests, 80.3% coverage
- build/vet/gofmt verdes

Antes de actuar:
1. Leé HANDOFF-HITO8-JOBS.md completo.
2. Verificá: git status, go build ./..., go test -gcflags="-l" ./internal/experience/jobs/
3. Leé docs/lessons.md para patrones operacionales.
4. Si todo verde, creá feat/hito8-jobs desde origin/main, commiteá y abrí PR.
5. Después: Hito 9 (retrieval lexical) según docs/26-IMPLEMENTATION-ROADMAP.md.
```

---

## 1. Qué se construyó en esta sesión

### Hito 8 — Motor de jobs (lease-based)

10 archivos nuevos, 6 modificados, 34 tests, 80.3% coverage.

| Capa | Archivo | Qué hace |
|---|---|---|
| Migración | `internal/storage/migrations/007_jobs.sql` | `job_state` + `job_registry` |
| Dominio | `internal/experience/jobs/types.go` | `JobStatus`, `JobState`, `JobRegistryEntry`, `LeaseBounds`, `RunResult` |
| Repositorio | `internal/experience/jobs/repository.go` | CRUD sobre job_state y job_registry |
| Servicio | `internal/experience/jobs/service.go` | `Register`, `AcquireLease`, `ReleaseLease`, `RunDue`, `executeWithRetry`, `RecoverStaleLeases`, `ComputeDigest` |
| Tests | `contract_test.go`, `repository_test.go`, `service_test.go`, `coverage_test.go` | 34 tests |
| CLI | `cmd/royo-learn/experience_jobs.go` | `list`, `register`, `run-due`, `recover` |
| MCP | `internal/mcpserver/tools.go`, `profiles.go` | `experience_jobs_list` (read), `experience_jobs_register` (admin), `experience_jobs_recover` (admin) |
| Dominio | `internal/domain/errors.go` | `ErrJobNotFound` agregado |

### Modificados (6 archivos)

- `cmd/royo-learn/experience.go` — wired `experience jobs` subcommand
- `internal/domain/errors.go` — `ErrJobNotFound` + `AllErrorCodes()` + `ExitCode()`
- `internal/mcpserver/tools.go` — 3 handlers MCP + import jobs
- `internal/mcpserver/profiles.go` — 3 tool registrations
- `internal/mcpserver/contract_test.go` — contract extensions
- `internal/mcpserver/server_test.go` — updated allowedTools + count

---

## 2. Estado actual del repositorio

- `main` HEAD: `d7352c8` (merge: Hito 4 — progressive trace)
- Tag: `v0.2.0`
- Working tree: **Hito 8 sin commitear** (10 nuevos, 6 modificados)
- Untracked preservado: `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`
- Migraciones activas: 001–007
- PRs abiertos: ninguno (Ola 1 cerrada)

### Gates verificados

- [x] `go build ./...` verde
- [x] `go vet ./...` verde
- [x] `gofmt` aplicado
- [x] `go test -gcflags="-l" ./internal/experience/jobs/` — 34 tests, 80.3% coverage
- [x] `go test -gcflags="-l" ./internal/mcpserver/` — verde
- [x] `go test -gcflags="-l" ./internal/...` — todos los paquetes verdes

---

## 3. Plan de acción para la próxima sesión

### 3.1. (Inmediato) Commitear Hito 8

```bash
git checkout -b feat/hito8-jobs origin/main
git add internal/experience/jobs/ internal/storage/migrations/007_jobs.sql \
        internal/domain/errors.go cmd/royo-learn/experience.go \
        cmd/royo-learn/experience_jobs.go internal/mcpserver/tools.go \
        internal/mcpserver/profiles.go internal/mcpserver/contract_test.go \
        internal/mcpserver/server_test.go ROADMAP.md
# No commitear PROMPT-LLM-EJECUTOR-ROYO-LEARN.md
git commit -m "feat(jobs): Hito 8 — lease-based job engine with migration 007"
```

### 3.2. (Después) Hito 9 — Retrieval lexical

Según `docs/26-IMPLEMENTATION-ROADMAP.md` §4 y `PLAN-MAESTRO` §Hito 9:

1. Interfaz retrieval (`internal/search/` o `internal/experience/retrieval/`)
2. Backend FTS (extender `internal/storage/fts.go`)
3. Ranking + score components
4. Patrones y eventos en búsquedas internas
5. Saneamiento FTS (no SQL injection)
6. Benchmark

**Acceptance**: contratos anteriores siguen, resultados mejoran en dataset, p95 local dentro del presupuesto, determinista, búsquedas ES/EN.

### 3.3. (Futuro) Resto de Ola 2

| PR | Hito | Descripción |
|----|------|-------------|
| 9 | 9 | Retrieval lexical + FTS |
| 10 | 3 | OpenCode `--watch` (opcional) |
| 11 | 10 | Claude Code adapter |
| 12 | 10 | Codex adapter |

---

## 4. Lecciones operativas de esta sesión

- **Migration 006 fue consumida por Hito 4 (trace).** Hito 8 usa 007. El PLAN-MAESTRO dice 006 porque fue escrito antes, pero en la práctica la migración 006 es de trace. Siempre verificar `ls internal/storage/migrations/` antes de crear una migración nueva.
- **`domain.ErrorCode` no implementa `error`.** Para crear errores tipados usables con `errors.Is`, usar `domain.NewNotFoundError(code, entity)`. El patrón correcto es declarar la variable en el paquete (como `patterns.ErrPatternNotFound`) y exponer `ErrorIs()` para tests.
- **`go test` en Windows requiere `-gcflags="-l"`.** Sin esto, el antivirus (Windows Defender) bloquea el test binary con "Access is denied". No es un bug del código.
- **Al agregar MCP tools, actualizar 3 lugares:** `profiles.go` (registration), `contract_test.go` (contractExtensions), `server_test.go` (allowedTools + conteo).
- **El MCP server ya tiene `srv.projectID` y `srv.projectRoot`.** Los handlers MCP no necesitan recibir `ProjectRoot` como input; usan `srv.projectID` directamente.
- **Patrón de paquete consistente:** `types.go` → `repository.go` → `service.go` → `contract_test.go` → `repository_test.go` → `service_test.go` → `coverage_test.go`. Respetarlo para Hito 9.

---

## 5. Lo que NO hay que hacer

- **No** commitear `PROMPT-LLM-EJECUTOR-ROYO-LEARN.md`
- **No** crear rama desde `main` local (usar `origin/main`)
- **No** mezclar Hito 9 con Hito 8 en el mismo PR
- **No** usar `domain.ErrorCode` directamente en `fmt.Errorf("...: %w", code)` — no funciona
- **No** olvidar `-gcflags="-l"` en Windows para tests
- **No** modificar `AGENTS.md` sin aprobación humana

---

## 6. Referencias clave

- `docs/26-IMPLEMENTATION-ROADMAP.md` — roadmap por ola, gates de salida
- `PLAN-MAESTRO-MEMSEARCH-A-ROYO-LEARN.md` — §Hito 8 y §Hito 9
- `docs/21-EXPERIENCE-DOMAIN.md` §8 — contrato de JobState
- `docs/lessons.md` — patrones operacionales (shell, WSL, review, finalize-dropped)
- `internal/experience/jobs/` — implementación completa de Hito 8
- `internal/experience/trace/` — patrón de referencia para Hito 9 (misma estructura)
- `internal/storage/fts.go` — backend FTS actual (a extender en Hito 9)
