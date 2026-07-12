# Agent Royo Learn

[![English](https://img.shields.io/badge/lang-en-blue.svg)](../README.md)
[![Español](https://img.shields.io/badge/lang-es-yellow.svg)](README.es.md)
[![Français](https://img.shields.io/badge/lang-fr-purple.svg)](README.fr.md)
[![Deutsch](https://img.shields.io/badge/lang-de-red.svg)](README.de.md)
[![中文](https://img.shields.io/badge/lang-zh-green.svg)](README.zh.md)
[![हिन्दी](https://img.shields.io/badge/lang-hi-orange.svg)](README.hi.md)
[![Português](https://img.shields.io/badge/lang-pt-lightgrey.svg)](README.pt.md)

**Agent Royo Learn** es un motor local de aprendizaje institucional para agentes de IA.

No reemplaza a Gentle-AI ni a Engram:

- **Gentle-AI** configura agentes, Skills, workflows y MCP.
- **Engram** conserva memoria persistente de sesiones, decisiones, descubrimientos y errores.
- **Agent Royo Learn** transforma experiencias verificadas en cambios reutilizables de comportamiento: conocimiento, Skills, reglas, pruebas y alertas de recurrencia.

---

## En palabras simples

Si no eres desarrollador, esto es lo que hace royo-learn, sin tecnicismos.

**El problema.** Los asistentes de IA que ayudan en un proyecto empiezan de cero
en cada conversación. Un asistente puede resolver un problema de forma brillante
hoy, pero mañana, en una sesión nueva, no recuerda nada. Si el mismo problema
reaparece, puede repetir el mismo error. Es como un empleado muy capaz que cada
mañana llega sin memoria del día anterior.

Guardar notas ayuda, pero no alcanza. Una nota que dice "esto salió mal" no
cambia, por sí sola, cómo trabaja la IA. Para que sirva, la lección tiene que
llegar a las instrucciones que la IA lee antes de empezar a trabajar.

**Qué hace royo-learn.** Funciona como un manual vivo del proyecto que se
mantiene al día. Cada vez que se aprende algo importante —un error que no debe
repetirse, una buena práctica, una regla del proyecto— se registra y se
convierte en una instrucción concreta que la IA lee la próxima vez. Así, la
siguiente vez ya sabe qué hacer y qué evitar.

En una línea: **royo-learn evita que un proyecto tropiece dos veces con la misma
piedra.**

**Por qué es valioso.** Recordar algo no cambia la forma de trabajar. Una
lección solo evita repetir un error si llega a las instrucciones que la IA
realmente usa, y si antes una persona la revisó y aprobó. Ese doble paso
—convertir un recuerdo en una instrucción y que un humano la apruebe— es lo que
separa a royo-learn de un simple bloc de notas, y lo que lo hace confiable: no
cualquier idea se vuelve regla, solo lo que fue verificado y aprobado.

**Cómo se relaciona con las otras piezas.**

| Pieza | Qué es | Analogía |
|---|---|---|
| **Gentle-AI** | El entorno que prepara a la IA: sus herramientas, habilidades y flujos de trabajo | La oficina, con sus normas y su equipo |
| **Engram** | La memoria de lo que ocurrió en sesiones anteriores | El diario donde se anota lo vivido |
| **royo-learn** | Convierte las lecciones aprobadas en reglas que la IA debe seguir | El manual de buenas prácticas |

En resumen: **Engram recuerda; royo-learn hace que ese recuerdo cambie la forma
de trabajar; y Gentle-AI es el entorno donde todo sucede.** royo-learn además
funciona por sí solo: si Engram o Gentle-AI están presentes los aprovecha, y si
no, cumple igual su función.

**Cómo se usa.** No hace falta escribir comandos ni saber programar. Se le habla
a la IA en lenguaje natural. Ocurre en tres momentos:

1. **Capturar** — Cuando pasa algo que conviene recordar, se lo dices a la IA:
   *"Aprende esto: …"*, *"Guarda esto para la próxima vez: …"* o *"No quiero que
   esto vuelva a pasar: …"*. Detrás de escena, la IA ordena tu frase y la guarda.
2. **Revisar** — Una lección capturada todavía no es una regla. Una persona la
   revisa y decide: aprobar, rechazar o pedir más pruebas. Este filtro evita que
   se acumule ruido.
3. **Publicar** — Una vez aprobada, la lección se convierte en una instrucción
   real dentro del proyecto. Desde ese momento la IA la lee y la aplica, y si
   algo sale mal, el cambio se puede deshacer.

Para usuarios técnicos, los mismos tres pasos están disponibles en la línea de
comandos (`royo-learn capture`, `curate`, `publish`).

---

### Cómo funciona — un ejemplo real

**La situación**: lanzamos v0.1.0 y actualizamos el README en inglés. Pero la
traducción al español seguía diciendo `v1.0.0` y usaba `--version` en los bloques
de PowerShell. El usuario ejecutó `install.ps1 --version v1.0.0` y falló. Después
de varias iteraciones arreglamos las traducciones y el script instalador.

**Paso 1 — Pedirle al modelo una frase de aprendizaje:**

> *"Dame la frase de aprendizaje que resume lo que acaba de pasar: el problema de
> versiones en los README multi-idioma, por qué falló, y cómo lo arreglamos."*

El modelo responde con una frase completa y bien estructurada:

> *"Aprendete esto: cuando un proyecto tiene READMEs multi-idioma con badges de
> traducción, después de cada release hay que sincronizar todas las traducciones
> con la fuente canónica en inglés. El error: el README en español referenciaba
> v1.0.0 y usaba `--version` en PowerShell, pero el release real era v0.1.0 y
> PowerShell requiere `-Version` con un solo guion. La solución: grep a todos los
> docs/README.*.md después de cada release para verificar consistencia de
> versiones, y hacer que install.ps1 acepte tanto `-Version` como `--version`.
> Bash mantiene `--version`, PowerShell usa `-Version`."*

**Paso 2 — Copiar la frase y disparar la captura:**

> *"Capturá este aprendizaje."* ← pegás la frase

**Paso 3 — El modelo ejecuta `capture_learning` vía royo-learn MCP.** El aprendizaje
queda persistido en la base de datos del proyecto con título, contexto, observación
y lección. En sesiones futuras el modelo lo recupera y lo aplica — no solo queda
guardado como memoria, sino estructurado para que el modelo pueda razonar sobre él.

**Frases que disparan el ciclo:**
- *"Dame la frase de aprendizaje para…"*
- *"Aprendete esto: …"*
- *"Capturá este aprendizaje"*
- *"No quiero que esto vuelva a pasar"*

---

### Engram + Royo-Learn: Conocimiento + Entendimiento

Existe una distinción útil entre dos conceptos:

- **Conocimiento**: datos crudos, hechos, respuestas — de fácil acceso. Hoy, herramientas como la IA nos dan acceso instantáneo al "conocimiento" sin esfuerzo.
- **Entendimiento**: el proceso cognitivo profundo de procesar, razonar e integrar esa información. Cuando delegamos todo, dejamos de quemar neuronas y perdemos la capacidad de entender de verdad.

Esta misma distinción se traslada a los dos sistemas:

| | Engram | Royo-Learn |
|---|---|---|
| **Rol** | Memoria persistente | Motor de aprendizaje |
| **Qué hace** | Almacena lo que pasó | Procesa, razona, integra |
| **Analogía** | Conocimiento (el cuaderno) | Entendimiento (el acto de estudiar) |

**Procesar**: Royo-Learn no recibe datos crudos y los guarda. El flujo de captura
([Arquitectura §4](../docs/02-ARCHITECTURE.md)) valida el payload, normaliza y
hashea, comprueba idempotencia, busca léxicamente (FTS5), recolecta evidencia
determinista, y solo entonces persiste el registro.

**Razonar**: El sistema de deduplicación
([RF-004](../docs/01-PRD.md#rf-004-deduplicación)) define relaciones semánticas
entre aprendizajes: `duplicate_of`, `extends`, `supersedes`, `contradicts`,
`narrows`, `related`. La máquina de estados
([RF-005](../docs/01-PRD.md#rf-005-estado)) fuerza decisiones: ¿esto se rechaza,
necesita evidencia, se fusiona, se aprueba? No es almacenamiento neutro — evalúa
la validez y coherencia del conocimiento.

**Integrar**: Un aprendizaje no se queda en una fila de base de datos. Se
convierte en una Skill o regla, se recupera en otra sesión, y *previene o detecta
una recurrencia* ([PRD §8](../docs/01-PRD.md)). El flujo de publicación
([Arquitectura §5](../docs/02-ARCHITECTURE.md)) — approved → preview → approve →
publish → verify → rollback — convierte el entendimiento en cambio operacional de
comportamiento.

Royo-Learn no entiende *por* el modelo. Es el andamiaje que hace que entender
Sirva de algo. Sin él, un LLM puede entender algo en una sesión, pero ese
entendimiento se evapora. Con él, ese entendimiento se vuelve persistente,
verificable, relacionable y accionable.

El repositorio produce un único binario multiplataforma:

```text
royo-learn        # Linux/macOS
royo-learn.exe    # Windows
```

## Instalación

### Linux / macOS

```bash
curl -fsSL https://github.com/RoyoTech/royo-learn/releases/latest/download/install.sh | bash
```

O manualmente:

```bash
# Descargar e instalar
./install.sh --version v0.1.1
# Desinstalar
./install.sh --uninstall
```

El binario se instala en `~/.local/bin/royo-learn`.

### Windows

```powershell
cd ~/Downloads
Invoke-WebRequest -Uri https://github.com/RoyoTech/royo-learn/releases/latest/download/install.ps1 -OutFile install.ps1
.\install.ps1
```

O manualmente:

```powershell
.\install.ps1 -Version v0.1.1
.\install.ps1 -Uninstall
```

El binario se instala en `%LOCALAPPDATA%\royo-learn\bin\royo-learn.exe`.

### Compilar desde fuente

```bash
# Requisito: Go 1.25+
git clone https://github.com/RoyoTech/royo-learn.git
cd royo-learn
make build       # Compilar para la plataforma actual
make build-all   # Compilación cruzada para todas las plataformas
make install     # Instalar en $GOPATH/bin
make clean       # Eliminar artefactos
make quality     # Control de calidad completo (fmt + test + vet + build)
```

## Inicio rápido

```bash
# Verificar versión
royo-learn version --json

# Inicializar un proyecto
royo-learn init --project-root /ruta/a/tu/proyecto

# Chequeo de salud
royo-learn doctor --project-root /ruta/a/tu/proyecto --json

# Capturar un aprendizaje
royo-learn capture \
  --project-root /ruta/a/tu/proyecto \
  --title "PostgreSQL connection pooling" \
  --context "despliegue en producción" \
  --observation "connection pool agotado con 100 concurrentes" \
  --lesson "configurar max_connections según memoria disponible" \
  --type "procedure" \
  --scope "project" \
  --json

# Curar (aprobar/rechazar) un aprendizaje
royo-learn curate \
  --project-root /ruta/a/tu/proyecto \
  --learning-id "<learning-id>" \
  --action "approve" \
  --rationale "validado con pruebas de carga" \
  --json

# Previsualizar antes de publicar
royo-learn preview \
  --project-root /ruta/a/tu/proyecto \
  --learning-id "<learning-id>" \
  --json

# Publicar (requiere hash de preview)
royo-learn publish \
  --project-root /ruta/a/tu/proyecto \
  --learning-id "<learning-id>" \
  --preview-hash "<preview-hash>" \
  --json

# Rollback de una publicación
royo-learn rollback \
  --project-root /ruta/a/tu/proyecto \
  --journal-id "<publication-id>" \
  --json

# Verificar recurrencias
royo-learn recurrences --learning-id "<learning-id>" --json
royo-learn metrics --learning-id "<learning-id>" --json

# Prueba end-to-end
royo-learn e2e --temp
```

## Configuración del servidor MCP

El comando `setup` registra royo-learn como servidor MCP e instala las Skills
del proyecto en Claude Code, Codex CLI y OpenCode — todo en un solo paso:

```bash
# Ver estado actual
royo-learn setup status

# Instalar en los tres agentes
royo-learn setup install --agent all

# Instalar en un agente específico (solo skills, sin MCP)
royo-learn setup install --agent claude-code --skip-mcp

# Dry-run primero
royo-learn setup install --agent all --dry-run --json

# Desinstalar
royo-learn setup uninstall --agent all
```

### Registro manual

Si preferís registrar manualmente:

**Codex**:
```bash
codex mcp add royo-learn -- royo-learn mcp-serve
```

**Claude Code / OpenCode** — agregar al archivo de configuración MCP:

```json
{
  "mcpServers": {
    "royo-learn": {
      "command": "royo-learn",
      "args": ["mcp-serve"],
      "env": {}
    }
  }
}
```

**OpenCode** usa la clave `"mcp"` (no `"mcpServers"`) con `"command"` como array
— usar `setup install --agent opencode` para el formato correcto.

**Perfiles**: `minimal` (capture, search, doctor), `standard` (por defecto; incluye
curate, preview, list, get), `full` (todas las herramientas, incluyendo publish).

```bash
royo-learn mcp-serve --profile full
```

## Arquitectura

```
LLM + Skill → propuesta semántica
royo-learn  → garantía operacional
```

Las tres Skills que definen qué significa una experiencia:

1. `capture-learning`
2. `curate-learning`
3. `publish-learning`

### Cómo capturar un aprendizaje

Con royo-learn MCP activo, decile al modelo en lenguaje natural:

> **"Aprendete esto: cada vez que hagamos un release, después de actualizar el README en inglés hay que revisar todas las traducciones en docs/README.*.md. Hoy el README en español tenía v1.0.0 y --version cuando el release real es v0.1.0 y PowerShell usa -Version con un solo guion. El usuario ejecutó install.ps1 --version v1.0.0 y falló. La lección es: bash usa --version, PowerShell usa -Version. Después de cada release, correr grep -r 'v[0-9]' docs/README.*.md para verificar que todas las traducciones tengan la versión correcta."**

El modelo extrae título, contexto, observación y lección automáticamente
y los persiste en la base de datos del proyecto. Otras frases que disparan la captura:

- *"Aprendete esto: …"*
- *"No quiero que esto vuelva a pasar: …"*
- *"Guardá esto para la próxima: …"*

El binario garantiza:

- persistencia
- estados válidos
- idempotencia
- trazabilidad
- deduplicación léxica
- integración opcional con Engram
- evidencia de Git y pruebas
- publicación segura
- aprobación humana
- rollback
- detección de recurrencias
- MCP sobre stdio

## Problema que resuelve

Guardar una memoria no asegura que el siguiente agente trabaje mejor. El proyecto
añade un ciclo explícito:

```
experiencia
    ↓
captura estructurada
    ↓
búsqueda de duplicados y antecedentes
    ↓
curación con evidencia
    ↓
aprobación
    ↓
publicación controlada
    ↓
verificación
    ↓
detección de recurrencias
```

## Alcance versión 1

La versión 1 es local, sin servicio cloud ni proveedor LLM embebido. El
razonamiento semántico lo realiza el agente que llama al servidor MCP.

La aplicación funciona aunque Gentle-AI o Engram no estén instalados. Sus
integraciones son opcionales y degradables.

## Onboarding para Codex

Codex debe leer, en este orden:

1. `AGENTS.md`
2. `CODEX_START_HERE.md`
3. `docs/01-PRD.md`
4. `docs/02-ARCHITECTURE.md`
5. `TASKS.md`

No comenzar a implementar desde este README.

## Desarrollo

```bash
make fmt        # Formatear código
make test       # Ejecutar pruebas
make vet        # Ejecutar vet
make build      # Compilar para la plataforma actual
make build-all  # Compilación cruzada
make quality    # Control de calidad completo (fmt + test + vet + build)
```

## Estructura del proyecto

```text
agent-royo-learn/
├── cmd/royo-learn/        # Punto de entrada CLI + e2e
├── internal/
│   ├── buildinfo/         # Metadatos de versión
│   ├── capture/           # Servicio de captura de aprendizajes
│   ├── config/            # Configuración de proyecto/usuario
│   ├── curate/            # Servicio de curación
│   ├── doctor/            # Chequeos de salud
│   ├── domain/            # Tipos de dominio y transiciones
│   ├── engram/            # Integración con Engram
│   ├── evidence/          # Recolección de evidencia (redacción, seguridad de rutas, git)
│   ├── logging/           # Logging estructurado
│   ├── mcpserver/         # Implementación del servidor MCP
│   ├── project/           # Resolución de proyectos
│   ├── publish/           # Motor de publicación
│   ├── recurrence/        # Detección de recurrencias
│   ├── setup/             # Helpers de instalación (skills, registro MCP, backup)
│   └── storage/           # Base de datos SQLite (migraciones, repos, FTS5)
├── migrations/            # Archivos de migración SQL
├── schemas/               # Esquemas JSON
├── skills/                # Skills del proyecto
├── docs/                  # Documentación
├── examples/              # Ejemplos de entrada
├── AGENTS.md              # Instrucciones para agentes
├── TASKS.md               # Tareas de implementación
├── Makefile               # Objetivos de build
├── .goreleaser.yml        # Configuración de release
├── install.sh             # Instalador Linux/macOS
├── install.ps1            # Instalador Windows
├── go.mod
└── go.sum
```

## Licencia

MIT