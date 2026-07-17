# Informe de implementación — candidato de seguridad v0.1.10

Este informe reemplaza el reporte de 2026-07-11, que declaraba el producto
completo sin vincular cada afirmación a evidencia ejecutable. El resultado de
esta intervención es un **candidato local de `v0.1.10`**, no un release.

## Identidad

| Campo | Valor |
|-------|-------|
| Fecha | 2026-07-16 |
| Rama | `fix/v0110-release-safety` |
| Base inmutable | `66a90daaac98ed6e64bbe1235dbe425cde7f18c3` |
| Snapshot revisado | `9767e25198afc4f789c50d8a1a00b4c4e4ea2da2` (`v0110-release-safety-v1`) |
| Código verificado | `e6d846f` y sus ancestros; este informe es el cierre documental posterior |
| Versión objetivo | `v0.1.10` |
| Estado | Candidato preparado; publicación no autorizada |

No se creó tag, no se hizo push, no se abrió PR y no se ejecutó GoReleaser.

## Resultado

| Área | Estado local | Evidencia |
|------|--------------|-----------|
| Backups, rutas y escritura CAS | PASS | `internal/publish/filesystem_safety_test.go`, `internal/publish/publish_test.go` |
| Intento recuperable y rollback convergente | PASS | `internal/publish/recovery_safety_test.go` |
| Estado SQLite y registro Markdown | PASS | `internal/publish/materialization_safety_test.go`, `internal/record/record_test.go` |
| Errores CLI/MCP y patch de reversión | PASS | `cmd/royo-learn/errors_test.go`, `TestRunRollbackConflictReturnsRecoveryArtifact`, `internal/mcpserver/error_envelope_test.go` |
| Contrato JSON `PolicyEvaluation` | PASS | `TestPolicyEvaluationPreservesPublicJSONKeys` |
| Plan persistido, lock y reconciliación | PASS | `plan_enforcement_test.go`, `filesystem_hardening_test.go`, `reconciliation_contract_test.go` |
| Cobertura domain/storage/publish | PASS | comando exacto de CI: `95.5% / 81.9% / 90.1%` |
| Instalación, actualización, rollback y desinstalación | PASS | `scripts/test-install.sh`, `scripts/test-install.ps1` |
| Release ligado al CI del SHA etiquetado | PASS estático / NOT_RUN remoto | `internal/integration/release_safety_test.go`, `.github/workflows/release.yml` |
| Matriz CI y tres SO | NOT_RUN remoto | Configurada en `.github/workflows/ci.yml`; requiere push y GitHub Actions |

## Cambios cerrados

### Publicación antes de escribir

- La publicación `in_progress` se persiste antes del primer cambio en destinos.
- Cada destino conserva existencia, modo y hash originales, backup verificado,
  hash publicado esperado y estado individual de recuperación.
- El journal `attempting` comparte el `publication_id` usado por SQLite.

### Filesystem seguro

- Backups, journal y artefactos se validan contra sus raíces autorizadas.
- Traversal, escapes por symlink y rutas de dispositivo se rechazan.
- El backup nace del mismo snapshot que alimenta la identidad CAS.
- Un cambio externo en la frontera final se preserva; nunca se sobrescribe.

### Rollback y verdad derivada

- Rollback reconoce destinos ya restaurados y persiste progreso por archivo.
- Un rollback exitoso cambia publicación a `rolled_back` y aprendizaje a
  `approved` en una misma transacción.
- Un rollback fallido conserva el aprendizaje `published` y crea un patch
  accionable sin modificar el destino conflictivo.
- `publish` y `rollback` re-materializan el registro desde SQLite mediante
  `internal/record`; `internal/publish` no depende de `internal/capture`.
- Fallos posteriores al commit declaran `committed`, estado e identificador,
  conservan todas las causas y prohíben reintentos ciegos.
- `rollback --list --json` descubre intentos interrumpidos y la reconciliación
  converge desde SQLite sin repetir una mutación ya confirmada.

### Interfaces públicas

- La CLI deriva su exit code del código de dominio; `rollback_failed` sale con
  código 13, no como `invalid_argument`.
- MCP usa el envelope documentado `{ "error": { ... } }`, conserva los campos
  top-level de `v0.1.x` y añade `recoverable`, `details`, `next_action` y
  `recovery_artifact` anidados.
- Las claves públicas `PolicyName`, `Passed` y `Reason` permanecen estables.

## CI y portabilidad

La CI configurada ejecutará:

- Linux, Windows y macOS con Go `1.25.0` y `1.26.x`;
- `go test -race -count=1 ./...` en Linux;
- pruebas sin race en Windows y macOS;
- cross-build `CGO_ENABLED=0` para Linux, macOS y Windows en `amd64` y `arm64`;
- instalación limpia seguida de `init`, `doctor --json` y `e2e --temp`.
- gates de cobertura `80%` domain, `80%` storage y `90%` publish;
- harnesses de instalador Unix y PowerShell.

Los seis cross-builds pasaron localmente. Los harnesses verificaron instalación
limpia, actualización, rechazo de checksum/versión, restauración posterior al
reemplazo y desinstalación. El candidato `v0.1.10` ejecutó `doctor` con `ok=true`
y `e2e --temp` pasó 37/37, incluido un cliente MCP real por stdio.

## Verificación local

| Comando | Resultado real |
|---------|----------------|
| `go mod verify` | PASS (`all modules verified`) |
| `go vet ./...` | PASS |
| `go test -race ./...` | PASS, todos los paquetes |
| `go test -race -count=1 ./internal/publish` en `e6d846f` | PASS, incluida la regresión explícita de symlinks en preview |
| cobertura exacta de CI | PASS: domain `95.5%`, storage `81.9%`, publish `90.1%` |
| `go test -p 1 -count=1 ./...` | Windows bloqueó `internal/buildinfo.test.exe`; los demás paquetes pasaron |
| `buildinfo.test.exe` compilado y ejecutado desde ruta estable | PASS, 3/3 |
| seis cross-builds con `CGO_ENABLED=0` | PASS |
| harnesses `test-install.sh` / `test-install.ps1` | PASS |
| candidato `royo-learn doctor --json` | PASS, `ok=true`; 6 degradaciones explícitas |
| candidato `royo-learn e2e --temp` | PASS, 37/37 |

La falla de `internal/buildinfo` ocurrió antes de ejecutar aserciones. El mismo
binario de test compilado a una ruta estable pasó 3/3 y la verificación completa
con race pasó después. Una corrida paralela también demoró el cleanup de un
`TempDir` de evidence; el paquete pasó al reintentarlo. Se conservan ambos
incidentes como evidencia del antivirus/filesystem intermitente de Windows.

## Riesgos y límites

1. La matriz de GitHub Actions no se ejecutó porque esta intervención no hace
   push. El release permanece bloqueado hasta verla verde.
2. El antivirus de Windows puede impedir ejecutar binarios bajo `go-build` o
   demorar el borrado de `TempDir`; los paquetes afectados pasan en ejecuciones
   focalizadas y la corrida final `go test -race ./...` fue verde.
3. `doctor` informa degradación explícita en checks opcionales todavía stub; los
   checks requeridos del proyecto limpio pasan. No se oculta esa degradación.
4. No se añadió outbox, proveedor LLM, embeddings, base vectorial ni servicio de
   red obligatorio.
5. El `royo-learn` global de esta máquina sigue siendo `v0.1.9`. La verificación
   37/37 invocó explícitamente el candidato `v0.1.10` construido en una ruta temporal;
   esta intervención no reemplaza instalaciones globales sin autorización.

La trazabilidad individual de los 33 hallazgos BLOCKER/CRITICAL congelados está
en `docs/IMPLEMENTATION-LOG.md`, sección "Corrección de la revisión congelada
`v0110-release-safety-v1`".

## Puerta de release

Estado: **CANDIDATO PREPARADO, NO PUBLICADO**.

Antes de etiquetar `v0.1.10` deben cumplirse ambas condiciones:

1. La matriz completa de GitHub Actions termina verde.
2. Un humano revisa este rango de commits y autoriza explícitamente tag y push.
