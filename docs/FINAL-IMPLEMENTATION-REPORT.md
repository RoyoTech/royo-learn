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
| Matriz CI mínima/estable y tres SO | NOT_RUN | Configurada en `.github/workflows/ci.yml`; requiere push y GitHub Actions |

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

### Interfaces públicas

- La CLI deriva su exit code del código de dominio; `rollback_failed` sale con
  código 13, no como `invalid_argument`.
- MCP usa el envelope documentado `{ "error": { ... } }` y conserva
  `recoverable`, `details`, `next_action` y `recovery_artifact`.
- Las claves públicas `PolicyName`, `Passed` y `Reason` permanecen estables.

## CI y portabilidad

La CI configurada ejecutará:

- Linux, Windows y macOS con Go `1.25.0` y `1.26.x`;
- `go test -race -count=1 ./...` en Linux;
- pruebas sin race en Windows y macOS;
- cross-build `CGO_ENABLED=0` para Linux, macOS y Windows en `amd64` y `arm64`;
- instalación limpia seguida de `init`, `doctor --json` y `e2e --temp`.

Los seis cross-builds pasaron localmente. La instalación limpia se verificó con
un `GOBIN` temporal: `init` y `doctor` salieron 0, y `e2e --temp` pasó 37/37.

## Verificación local

| Comando | Resultado real |
|---------|----------------|
| `go mod verify` | PASS (`all modules verified`) |
| `go vet ./...` | PASS |
| `go test -race -count=1 ./...` | PASS, todos los paquetes, sin cache |
| `go test ./...` (corrida anterior) | Windows bloqueó `internal/buildinfo.test.exe` con `Access is denied`; no hubo fallos de aserción |
| seis cross-builds con `CGO_ENABLED=0` | PASS |
| binario instalado: `royo-learn e2e --temp` | PASS, 37/37 |

La falla anterior de `internal/buildinfo` ocurrió antes de ejecutar aserciones y
se reprodujo aislada en ese momento. La verificación final, completa, sin cache y
con race pasó después. Se conserva el incidente porque confirma que el antivirus
de Windows es intermitente, no porque la puerta local siga roja.

## Riesgos y límites

1. La matriz de GitHub Actions no se ejecutó porque esta intervención no hace
   push. El release permanece bloqueado hasta verla verde.
2. El antivirus de Windows puede impedir ejecutar binarios bajo `go-build` o
   demorar el borrado de `TempDir`; los paquetes afectados pasan en ejecuciones
   focalizadas y la corrida final `-race -count=1 ./...` fue verde.
3. `doctor` informa degradación explícita en checks opcionales todavía stub; los
   checks requeridos del proyecto limpio pasan. No se oculta esa degradación.
4. No se añadió outbox, proveedor LLM, embeddings, base vectorial ni servicio de
   red obligatorio.
5. El `royo-learn` global de esta máquina sigue siendo `v0.1.9`. La verificación
   37/37 invocó explícitamente el binario recién instalado en el `GOBIN` temporal;
   esta intervención no reemplaza instalaciones globales sin autorización.

## Puerta de release

Estado: **CANDIDATO PREPARADO, NO PUBLICADO**.

Antes de etiquetar `v0.1.10` deben cumplirse ambas condiciones:

1. La matriz completa de GitHub Actions termina verde.
2. Un humano revisa este rango de commits y autoriza explícitamente tag y push.
