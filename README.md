# Odrys CLI

CLI personal de agentes orquestados para desarrollo asistido por IA.

Odrys es un cliente de desarrollo asistido por IA, pensado para terminal, que orquesta agentes especializados, memoria persistente del proyecto y operaciones reales sobre el workspace mediante una TUI propia.

La nueva direccion del proyecto es `Go + Bubble Tea` para el cliente.
La capa Node sigue viva de forma temporal como backend puente mientras terminamos la migracion.

## Estructura

- `cmd/odrys/`: entrada principal del cliente en Go
- `internal/`: TUI Bubble Tea y puente temporal con el backend Node
- `src/`: backend y runtime heredado en Node
- `system/`: prompt base del sistema
- `agents/`: prompts por agente
- `project/`: memoria externa del proyecto
- `schemas/`: contratos JSON
- `docs/`: documentacion operativa
- `logs/`: historial de ejecuciones
- `opencode-dev/`: referencia externa local, no forma parte de Odrys

## Estado

El proyecto esta en una v1 funcional y experimental.

Hoy ya incluye:

- cliente Go en migracion con Bubble Tea
- backend Node operativo como puente
- flujo de agentes `Metre`, `Cocinero`, `Auditor` y `Caja`
- memoria externa en archivos
- soporte de `workspace` real
- escritura controlada en sandbox y operaciones basicas

Todavia no incluye:

- sesiones persistentes de larga duracion
- aprobaciones interactivas para permisos `ask`
- reescritura completa del worker al stack Go

## Arranque

Ruta nueva recomendada:

```bash
./odrys
```

El wrapper intenta usar este orden:

1. `./dist/odrys` si ya existe el binario compilado
2. `go run ./cmd/odrys` si Go esta instalado
3. `node src/cli.js` como fallback temporal

Tambien puedes usar:

```bash
make run
make build
./dist/odrys
```

Si quieres enlazarlo localmente:

```bash
chmod +x install-local.sh
./install-local.sh
source ~/.bashrc
hash -r
odrys
```

## Comandos legacy del backend Node

Mientras dura la migracion, el backend actual sigue disponible:

```bash
node src/cli.js init
node src/cli.js run "crear una CLI para gestionar notas" --provider mock
node src/cli.js doctor
node src/cli.js workspace scan --workspace ../mi-proyecto
node src/cli.js workspace write .odrys-sandbox/todo.txt --content "primera nota"
node src/cli.js workspace patch --workspace ../mi-proyecto --file ./cambio.patch
```

## Cliente

La TUI nueva nace con separacion de vistas real:

- `home`
- `help`
- `session`

Y el backend actual sigue aportando:

- estado actual de workspace y proveedor
- ejecucion de `Cocinero` y `Auditor`
- operaciones aplicadas
- logs estructurados

## Migracion a Go

La TUI vieja en Node queda como ruta `legacy`.
La migracion actual usa un puente: el cliente Go ejecuta el backend Node mediante comandos internos y renderiza la experiencia con Bubble Tea.

Esto nos permite:

- eliminar problemas de render y foco del cliente anterior
- mejorar la arquitectura visual sin rehacer aun todo el worker
- migrar por capas en vez de romper lo que ya funciona

En esta maquina no he podido compilar la nueva app porque `go` no esta instalado todavia.
En cuanto lo tengas, la ruta normal sera:

```bash
make build
./dist/odrys
```

## Proveedores

### `mock`

No usa red. Sirve para validar el flujo de trabajo, logs, estado y contratos.

### `openai-compatible`

Usa un endpoint compatible con OpenAI Chat Completions.

Variables soportadas:

```bash
export ODYRS_PROVIDER=openai-compatible
export ODYRS_MODEL=gpt-4.1-mini
export ODYRS_API_KEY=tu_api_key
export ODYRS_BASE_URL=https://api.openai.com/v1
```

Tambien acepta `OPENAI_API_KEY`.

## Flujo

1. El `worker` carga contexto desde `project/`
2. Si hay `workspace`, construye un snapshot local del repositorio objetivo
3. Decide si hace falta `Metre`
4. Ejecuta `Cocinero`
5. Pasa por `Auditor`
6. Si falla, reitera sobre `Cocinero`
7. Si pasa, actualiza `project_state.md`
8. Guarda logs estructurados en `logs/runs/`

## Workspace real

La v1 ya puede apuntar a un repositorio objetivo local mediante `workspace.path` o `--workspace`.

Comandos utiles:

```bash
node src/cli.js workspace scan --workspace ../mi-proyecto
node src/cli.js run "analizar la estructura y proponer la siguiente iteracion" --workspace ../mi-proyecto --provider mock
```

El snapshot del workspace usa tools locales y permisos basicos:

- `list`
- `read`
- `search`
- `bash`
- `write`
- `apply_patch`

## Permisos

Las reglas viven en `odrys.config.json`.
Cada accion puede ser `allow`, `ask` o `deny`.

En esta v1:

- `allow` ejecuta
- `deny` bloquea
- `ask` falla con un mensaje claro porque aun no hay flujo interactivo de aprobacion

`write` y `apply_patch` dependen del permiso `edit`.
Por defecto solo queda permitido sin aprobacion el sandbox `.odrys-sandbox/**`.

Ejemplo:

```json
{
  "permission": {
    "bash": {
      "*": "ask",
      "git status --short": "allow"
    }
  }
}
```

## Subir a GitHub

El repositorio ya ignora artefactos locales como:

- `node_modules/`
- `dist/`
- `.odrys-sandbox/`
- `logs/runs/`
- `.codex`
- `opencode-dev/`

La carpeta `opencode-dev/` se ha usado solo como referencia local y no forma parte de Odrys.
