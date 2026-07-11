# Plan de implementación

## Fase 0 — Bootstrap

- crear módulo Go;
- estructura mínima;
- version info;
- Makefile;
- CI;
- logger stderr;
- config;
- error types.

Gate:

```bash
go test ./...
go vet ./...
```

## Fase 1 — Dominio y storage

- entidades;
- máquina de estados;
- migration runner;
- schema inicial;
- repositories;
- transactions;
- FTS5;
- audit.

Gate: CRUD y transiciones con DB temporal.

## Fase 2 — Proyecto y evidencia

- git root;
- project identity;
- config project;
- redact;
- evidence blobs;
- command runner;
- path security.

Gate: pruebas Windows/Linux y secretos.

## Fase 3 — Captura y búsqueda

- idempotency;
- normal hash;
- lexical similarity;
- records Markdown;
- CLI capture/get/list/search.

Gate: capture duplicado no crea segunda entidad.

## Fase 4 — Curación

- decision contract;
- relations;
- evidence thresholds;
- status transitions;
- CLI curate/review.

Gate: no approve sin campos requeridos.

## Fase 5 — Preview y aprobación

- publication plan;
- target resolution;
- diff;
- preview canonical hash;
- policies;
- approval.

Gate: preview mutado invalida approval.

## Fase 6 — Publicación

- atomic writer;
- backups;
- managed blocks;
- Skill validator;
- verification runner;
- rollback;
- publication journal.

Gate: e2e local.

## Fase 7 — MCP

- official Go SDK;
- tools;
- schemas;
- profiles;
- middleware;
- limits;
- structured errors;
- stdio tests.

Gate: MCP Inspector/client test.

## Fase 8 — Integraciones

- Engram HTTP adapter;
- Gentle-AI registry adapter;
- Codex setup helper;
- doctor.

Gate: integraciones degradables.

## Fase 9 — Recurrencia y métricas

- occurrence;
- fingerprint;
- metrics;
- status.

Gate: aprendizaje publicado marcado ineffective según política configurable.

## Fase 10 — Instalación y release

- scripts;
- GoReleaser;
- cross builds;
- MCP registration;
- uninstall;
- docs final.

## Orden prohibido

No comenzar por:

- TUI;
- cloud;
- embeddings;
- plugin marketplace;
- auto-LLM;
- dashboard.

## Definición de slice completo

Una fase no termina con structs vacíos. Debe incluir:

- implementación;
- pruebas;
- documentación;
- errors;
- CLI o MCP cuando corresponda;
- aceptación.
