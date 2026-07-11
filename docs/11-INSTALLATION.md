# Instalación y distribución

## Requisitos para construir

- Git;
- Go 1.24+;
- herramienta de release;
- opcional: Codex CLI;
- opcional: Gentle-AI;
- opcional: Engram.

## Desarrollo

```bash
git clone <repository>
cd agent-royo-learn
go mod download
go test ./...
go build -o bin/royo-learn ./cmd/royo-learn
```

Windows:

```powershell
go build -o bin\royo-learn.exe .\cmd\royo-learn
```

## Instalación local

### Windows

Destino preferido:

```text
%LOCALAPPDATA%\agent-royo-learn\bin\royo-learn.exe
```

El script:

1. detecta arquitectura;
2. copia binario;
3. agrega PATH de usuario sin duplicar;
4. crea config en `%APPDATA%\agent-royo-learn\config.yaml`;
5. hace backup de config Codex;
6. opcionalmente registra MCP;
7. verifica.

### Linux/macOS

Destino:

```text
~/.local/bin/royo-learn
```

o Homebrew futuro.

## Registro Codex

```bash
codex mcp add royo-learn -- royo-learn mcp
codex mcp list
```

Desinstalación:

```bash
codex mcp remove royo-learn
```

El script no debe asumir que este comando existe: consultar `codex mcp --help`.

## Instalación de Skills

Opciones:

### Proyecto

Copiar `skills/` al repositorio objetivo.

### Biblioteca compartida

Configurar `shared_root` versionado en Git.

No copiar automáticamente a todas las carpetas de agentes en v1. La biblioteca debe tener un instalador explícito.

## Gentle-AI

Después de instalar Skills:

```bash
gentle-ai skill-registry refresh --force
```

Si no existe, informar sin fallar.

## Engram

No instalarlo automáticamente como dependencia. Si el usuario solicita stack completo:

```bash
engram setup codex
```

Solo usar comandos oficiales detectados en la versión instalada.

## Release

Usar GoReleaser para:

```text
windows/amd64
windows/arm64
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
```

Artefactos:

- zip/tar.gz;
- SHA256SUMS;
- SBOM;
- changelog;
- signatures si hay infraestructura.

## Update

`royo-learn update` no es obligatorio en v1. El instalador debe ser idempotente.

## Uninstall

Debe poder:

- retirar binario;
- retirar MCP;
- retirar PATH si lo agregó;
- conservar DB/records por defecto;
- `--purge-data` explícito para borrar datos;
- no borrar Skills publicadas.

## Instalación verificada

El script final ejecuta:

```bash
royo-learn version
royo-learn doctor --json
codex mcp list
```
