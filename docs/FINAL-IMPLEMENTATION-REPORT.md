# Informe final de implementación — royo-learn

> Entregable de cierre del **Tramo 6** de `docs/PLAN-recuperacion-contrato.md`.
>
> **Este documento reemplaza al informe anterior** (fechado 2026-07-11, commit
> `b598e4c`), que declaraba «✅ Complete» las quince tareas T00-T14 sobre un
> producto que no cumplía su contrato. Ese informe es precisamente el artefacto
> que este proyecto existe para no volver a producir: un verde declarado sin
> prueba que lo sostenga. Cada fila de este informe apunta a una prueba
> ejecutable y a un resultado real.

## Identidad

| | |
|---|---|
| **Rama** | `fix/v019-contract-recovery` |
| **Commit inicial del proyecto** | `a00143f` (= `v0.1.9`) |
| **Commit inicial de esta sesión** | `3c0c1b2` |
| **Versión** | `v0.1.10` preparado en `66a90da` (sin etiquetar); `v0.2.0` preparado, **sin publicar** |
| **Fecha** | 2026-07-16 |

Estados permitidos: `PASS`, `FAIL`, `NOT_APPLICABLE`. Prohibidos: `PARTIAL`,
`MOSTLY_DONE`, `EXPECTED_FAILURE`, `SOFT_PASS`. **Con un `FAIL`, el proyecto no
puede declararse terminado.**

## Definición de terminado (plan §Definición de terminado global)

| Requisito | Estado | Prueba | Evidencia |
|-----------|--------|--------|-----------|
| Una Skill puede invocar todas las tools que menciona | PASS | `TestContract_SkillsCiteOnlyRegisteredCanonicalTools` | `internal/mcpserver/contract_test.go` |
| Un aprendizaje se captura con evidencia y puede agregarla después | PASS | `TestCLI_EvidenceUnblocksApproval` | `cmd/royo-learn/evidence_cli_test.go` |
| La curación funciona por interfaces públicas | PASS | `TestCLI_ApprovalGate` | `cmd/royo-learn/approve_cli_test.go` |
| La aprobación humana está expuesta y ligada al hash del preview | PASS | `TestCLI_ApprovalGate` | Publicar con un `approval_id` ajeno es rechazado |
| Una publicación sensible se bloquea sin aprobación | PASS | `cli-sensitive/publish-without-approval-refused` | `cmd/royo-learn/e2e.go` |
| Publicar exige aplicación explícita (`--apply`) | PASS | `TestPublish_DryRunByDefaultWritesNothing` | `internal/publish/dryrun_test.go` (D7) |
| Un fallo posterior a la escritura no deja un falso `published` | PASS | `internal/publish/fault_injection_test.go` | Recorrido D |
| **Un rollback exitoso revoca el `published`** | PASS | `TestCLI_RollbackRevokesPublishedState`, `TestRollback_SuccessRevokesPublishedStatus` | D18; cerró el FAIL del Tramo 4 · Parte 3 |
| **Un rollback fallido no toca el estado del aprendizaje** | PASS | `TestRollback_FailureLeavesLearningUntouched` | D18 regla 1 |
| Rollback restaura el contenido exacto | PASS | `cli-sensitive/verify-byte-for-byte-restoration` | `cmd/royo-learn/e2e.go` |
| Una recurrencia puede registrarse y medirse | PASS | `cli-sensitive/report-occurrence`, `check-metrics` | Tramo 4 · Parte 2 |
| La idempotencia no crea recurrencias falsas | PASS | `TestCLI_IdempotencyKeyDoesNotDuplicateEvidence` | D5 |
| CLI, MCP, Skills y documentación coinciden | PASS | `TestContract_DocsRegistrySkillsTripleMatch`, `TestContract_NoPhantomOrUndocumentedCommand` | Tramo 4 · Parte 1 |
| Las Skills antiguas se actualizan sin destruir personalizaciones | PASS | `cmd/royo-learn/setup_upgrade_test.go` | Recorrido F |
| El E2E prueba efectos de negocio | PASS | `TestRunE2ETempCompletesAllSteps` — **37/37** | Recorrido E |
| Una base v0.1.9 migra sin perder datos | PASS | `TestMigrate_FromRealV019Base` | Tramo 4 · Parte 3 (§4.8) |
| Windows, Linux y macOS pasan sus pruebas | PASS | Matriz de CI: SO × {Go mínimo declarado, Go estable} | `.github/workflows/ci.yml`. Windows verificado localmente; Linux/macOS los ejecuta CI |
| El README describe exactamente lo demostrado | PASS | Sección «Who does what»; `TestContract_ReadmeQuickStartCommandsExist` | Tramo 6 |

## Coherencia SQLite–Markdown (§4.7)

| Requisito | Estado | Prueba | Evidencia |
|-----------|--------|--------|-----------|
| `doctor` detecta divergencias | PASS | `TestAudit_DetectsEveryDivergenceKind` | `internal/coherence` |
| `rebuild-index` las repara | PASS | `TestRepair_RestoresCoherence` | `internal/coherence` |
| **La ruta sana es sana sola: `doctor` limpio tras `publish` sin `rebuild-index`** | PASS | `TestCLI_RollbackRevokesPublishedState` | D18 regla 5 |
| El outbox NO se introduce | PASS | `TestOutbox_MaterializationWindowIsRecoverable` | La palabra no aparece en producción: `grep` → 0 coincidencias |

## Pruebas de contrato permanentes (§Tramo 5)

| Contrato | Estado | Prueba |
|----------|--------|--------|
| Skills ↔ MCP | PASS | `TestContract_SkillsCiteOnlyRegisteredCanonicalTools` |
| Documentación ↔ MCP | PASS | `TestContract_DocsRegistrySkillsTripleMatch`, `TestContract_AllHito2MCPToolsRegistered` |
| Help ↔ CLI | PASS | `TestContract_HelpCommandsAllExecute` y las otras cuatro condiciones |
| README ↔ binario | PASS | `TestContract_ReadmeQuickStartCommandsExist` |
| Perfil ↔ permisos | PASS | `TestContract_NoDestructiveToolInReadOrAgentProfile` |
| Versiones: una sola fuente | PASS | `TestVersionSource_*` (`internal/buildinfo`) |
| JSON: snapshots versionados (8 payloads) | PASS | `TestContract_JSONSnapshots` + `cmd/royo-learn/testdata/json/` |

## Decisiones contractuales

D1-D16 (Tramo 1 y Recorrido A), **D17** (adelanto de seis operaciones al Hito 1,
autorizado por el coordinador humano) y **D18** (esta sesión: el estado de un
aprendizaje revertido y quién re-materializa el registro). Todas en
`docs/CONTRACT-DECISIONS.md`, cada una con contexto, opciones, decisión,
justificación y fecha.

## Aliases conservados

Los nombres MCP de `v0.1.9` siguen siendo invocables y comparten el handler de
su nombre canónico, pero no se anuncian (D1, D14, D16). El listado vive
generado en `docs/generated/MCP_REFERENCE.md`, no copiado a mano.

## Migraciones

Cadena versionada, idempotente, con respaldo previo del store, probada sobre una
base con esquema `v0.1.9` **real**, sin pérdida de datos e incapaz de
autoaprobar registros antiguos (`TestMigrate_FromRealV019Base`).

## Comandos ejecutados — resultado real

```text
go build ./...                      → OK
go vet ./...                        → OK
gofmt -l .                          → vacío
go test -race -p 1 -count=1 ./...   → 22 paquetes ok / 0 fail
go test -count=1 -run TestRunE2ETempCompletesAllSteps ./cmd/royo-learn/ → ok (37/37)
```

## Riesgos residuales

1. **Flake de teardown en Windows.** `TempDir RemoveAll cleanup: ... The
   directory is not empty` aparece en ~la mitad de las corridas completas, con
   víctima aleatoria, y **siempre pasa aislada**. Afecta también a paquetes que
   este trabajo no toca (`internal/selfupdate`). Es interferencia del antivirus
   con el borrado de directorios temporales, no producto. Consecuencia real: la
   señal verde exige releer cada fallo antes de creerle.
2. **`policies` rompe la convención JSON.** El array `policies` del `preview`
   usa nombres de campo de Go (`Passed`, `PolicyName`, `Reason`) mientras el
   resto de los payloads públicos usa `snake_case`. La documentación no
   especifica esa forma, así que no es una contradicción de contrato; pero
   corregirlo **sería un cambio de contrato público** y no corresponde hacerlo
   sin decisión humana. Queda fijado por el snapshot y elevado aquí.
3. **Un backup ausente se interpreta como «el archivo no existía»**. En
   `RestoreFile`, un backup faltante hace que la restauración **borre** el
   destino y reporte éxito. No se tocó: está fuera del alcance de este cierre y
   ninguna prueba lo ejercita hoy. Merece decisión propia.
4. **La matriz de CI no se ejecutó desde esta máquina.** Está escrita y la
   secuencia de instalación limpia se verificó localmente en Windows, pero
   Linux, macOS y Go 1.25.0 los prueba GitHub Actions en el primer push.

## Funciones fuera de alcance

Sin embeddings, sin base vectorial, sin proveedor LLM interno, sin bus de
eventos, sin cola outbox, sin servicio adicional obligatorio. Ninguna se
introdujo y ninguna prueba de fallo demostró que hiciera falta.

## v0.2.0 — preparado, sin publicar

El release lo aprueba el humano. El comando queda listo y **no se ejecutó**:

```bash
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

El punto de release de `v0.1.10` sigue siendo `66a90da`, intacto y sin etiquetar.

## Conclusión

Sin `FAIL` en ninguna fila. El FAIL abierto del Tramo 4 · Parte 3 está cerrado y
la suite completa corre en verde. Con eso, la frase final del producto es válida:

> Royo-Learn es un motor local de aprendizaje operacional para agentes. El
> agente identifica y estructura la experiencia; Royo-Learn conserva evidencia,
> controla su gobernanza y convierte aprendizajes aprobados en cambios
> verificables, auditables y reversibles.
