# Odrys CLI

CLI personal de agentes orquestados para desarrollo asistido por IA.

Odrys es un cliente de desarrollo asistido por IA, pensado para terminal, que orquesta agentes especializados, memoria persistente del proyecto y operaciones reales sobre el workspace mediante una TUI propia.

La migracion principal ya esta cerrada: la ruta activa del producto funciona en `Go + Bubble Tea`.

## Fases de migracion

1. Cliente Bubble Tea
Completada.
La TUI principal ya vive en [cmd/odrys/main.go](/home/tekilatime/PROYECTOS/Odrys-CLI/cmd/odrys/main.go) e [internal/app/model.go](/home/tekilatime/PROYECTOS/Odrys-CLI/internal/app/model.go).

2. Backend nativo en Go
Completada.
Configuracion, worker, permisos, workspace, estado, providers y operaciones ya viven en [internal/backend/runtime.go](/home/tekilatime/PROYECTOS/Odrys-CLI/internal/backend/runtime.go), [internal/backend/config.go](/home/tekilatime/PROYECTOS/Odrys-CLI/internal/backend/config.go) e [internal/backend/types.go](/home/tekilatime/PROYECTOS/Odrys-CLI/internal/backend/types.go).

3. CLI y wrappers
Completada.
`odrys`, `bin/odrys`, `make run`, `make build`, `doctor`, `run` y `workspace` ya salen por la implementacion Go.

4. Legacy Node
Eliminado del camino principal.
La implementacion heredada ya no forma parte del arbol activo.

## Estructura

- `cmd/odrys/`: entrada principal del cliente y CLI
- `internal/app/`: TUI Bubble Tea
- `internal/backend/`: runtime nativo de Odrys
- `system/`: prompt base del sistema
- `agents/`: prompts por agente
- `project/`: memoria externa del proyecto
- `schemas/`: contratos JSON
- `docs/`: documentacion operativa

## Estado actual

Hoy ya incluye:

- cliente Bubble Tea propio
- worker nativo en Go
- flujo de agentes `Metre`, `Cocinero`, `Auditor` y `Caja`
- memoria externa en archivos
- soporte de `workspace` real
- escritura controlada en sandbox y operaciones basicas

Todavia no incluye:

- sesiones persistentes de larga duracion
- aprobaciones interactivas para permisos `ask`

## Arranque

Ruta principal:

```bash
./odrys
```

Tambien puedes usar:

```bash
make run
make build
./dist/odrys
odrys doctor
odrys run "crear archivo .odrys-sandbox/demo.txt con contenido hola"
odrys workspace scan
```

Si quieres enlazarlo localmente:

```bash
chmod +x install-local.sh
./install-local.sh
source ~/.bashrc
hash -r
odrys
```

## Proveedores

### `mock`

No usa red. Sirve para validar flujo, contratos, logs y operaciones.

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

## Workspace real

Odrys puede trabajar contra un repositorio objetivo local mediante `workspace.path` o `--workspace`.

Comandos utiles:

```bash
odrys workspace scan
odrys run "analizar la estructura y proponer la siguiente iteracion"
odrys workspace write .odrys-sandbox/todo.txt --content "primera nota"
odrys workspace patch --file ./cambio.patch
```

El runtime usa estas capacidades:

- `list`
- `read`
- `search`
- `bash`
- `write`
- `apply_patch`

## Permisos

Las reglas viven en `odrys.config.json`.
Cada accion puede ser `allow`, `ask` o `deny`.

En esta version:

- `allow` ejecuta
- `deny` bloquea
- `ask` falla con un mensaje claro porque aun no hay flujo interactivo de aprobacion

`write` y `apply_patch` dependen del permiso `edit`.
Por defecto solo queda permitido sin aprobacion el sandbox `.odrys-sandbox/**`.

## Artefactos ignorados

El repositorio ya excluye:

- `dist/`
- `.go-cache/`
- `.go-mod-cache/`
- `.go-path/`
- `.odrys-sandbox/`
- `logs/runs/`
- `opencode-dev/`
