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
- menu interactivo de ayuda y selector de sesiones dentro de la TUI
- worker nativo en Go
- flujo de agentes `Metre`, `Cocinero`, `Auditor` y `Caja`
- memoria externa en archivos
- sesiones persistentes en `logs/sessions/`
- reanudacion automatica configurable de la sesion reciente
- contexto enriquecido con resumen de sesion, mensajes relevantes, objetivos recientes, archivos tocados, notas recientes y runs recientes
- aprobaciones interactivas en la TUI para permisos `ask`
- soporte de `workspace` real
- escritura controlada en sandbox y operaciones basicas

Todavia no incluye:

- streaming de tokens
- paneles avanzados de sesiones y variantes
- politicas de aprobacion persistentes mas finas

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
odrys sessions
odrys sessions latest
odrys provider current
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

### `openai`

Odrys es `OpenAI-first`.
Puede conectarse a OpenAI de dos formas:

- `API key`
- `codigo` para iniciar sesion con tu cuenta de ChatGPT

Las credenciales persistidas por Odrys se guardan en `.odrys/auth.json`.
El fallback por entorno sigue funcionando con `OPENAI_API_KEY`.

Variables soportadas:

```bash
export OPENAI_API_KEY=tu_api_key
```

Notas:

- `OPENAI_API_KEY` sirve para la ruta clasica por API.
- La ruta `codigo` usa navegador + callback local gestionado por `odrys-core`.
- La build actual trata `ChatGPT Plus/Pro` como compatibilidad experimental tipo Codex.

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

Tambien puedes cambiar provider y modelo sin editar a mano la config:

```bash
odrys provider current
odrys provider set openai gpt-4.1-mini
odrys openai status
odrys openai connect --api-key "sk-..." --model gpt-4.1-mini
```

## Backend local

Odrys ya incluye un backend local propio:

- `odrys-core`
- o `odrys server`

Arranque:

```bash
make run-server
```

o:

```bash
odrys server --host 127.0.0.1 --port 4111
```

Endpoints base:

```bash
GET  /health
POST /api/v1/core/bootstrap
GET  /api/v1/sessions
POST /api/v1/sessions
GET  /api/v1/openai/status
POST /api/v1/openai/connect/api-key
POST /api/v1/openai/connect/device/start
GET  /api/v1/openai/connect/device/poll/:id
POST /api/v1/openai/disconnect
```

Estado de la migracion del core/backend:

- fases implementadas en [backend-phases.md](/home/tekilatime/PROYECTOS/Odrys-CLI/docs/system/backend-phases.md)

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
- `ask` abre una aprobacion interactiva dentro de la TUI
- en modo CLI no interactivo, `ask` devuelve un error claro para evitar ejecuciones ambiguas

`write` y `apply_patch` dependen del permiso `edit`.
Por defecto solo queda permitido sin aprobacion el sandbox `.odrys-sandbox/**`.

## Sesiones y contexto

Las sesiones se guardan en `logs/sessions/`.

Odrys usa esa memoria para reconstruir contexto con:

- resumen persistente de la sesion
- mensajes seleccionados por relevancia
- objetivos recientes
- archivos tocados recientemente
- notas compactas de tareas previas
- ultimos runs completados

Comandos utiles dentro del cliente:

```bash
/sessions
/resume latest
/resume <session_id>
```

La TUI tambien permite:

- abrir el menu con `ctrl+o`
- abrir la vista de modelos/providers con `ctrl+l`
- abrir `sessions` como lista interactiva de historiales guardados
- entrar en una sesion seleccionada y volver a su chat
- conectar OpenAI sin salir del cliente:
  - `API key`
  - `codigo` para ChatGPT

Tambien por CLI:

```bash
odrys sessions
odrys sessions latest
odrys sessions show <session_id>
```

## Artefactos ignorados

El repositorio ya excluye:

- `dist/`
- `.go-cache/`
- `.go-mod-cache/`
- `.go-path/`
- `.odrys-sandbox/`
- `logs/runs/`
- `logs/sessions/`
- `opencode-dev/`
