# Odrys CLI

CLI personal de agentes orquestados para desarrollo asistido por IA.

Odrys es un cliente de desarrollo asistido por IA, pensado para terminal, que orquesta agentes especializados, memoria persistente del proyecto y operaciones reales sobre el workspace mediante una TUI propia.

Esta base implementa una v1 minima y util:

- `Metre` opcional
- `Cocinero`
- `Auditor`
- `Caja` ocasional
- `worker` determinista

No intenta replicar OpenCode completo.
Toma sus ideas utiles y las reduce a una herramienta local, entendible y mantenible para uso personal.

## Estructura

- `src/`: codigo de la CLI y del worker
- `system/`: prompt base del sistema
- `agents/`: prompts por agente
- `project/`: memoria externa del proyecto
- `schemas/`: contratos JSON
- `docs/`: documentacion operativa
- `logs/`: historial de ejecuciones
- `opencode-dev/`: referencia externa, no forma parte de la v1 propia

## Estado

El proyecto esta en una v1 funcional y experimental.

Hoy ya incluye:

- TUI propia de terminal
- flujo de agentes `Metre`, `Cocinero`, `Auditor` y `Caja`
- memoria externa en archivos
- soporte de `workspace` real
- escritura controlada en sandbox y operaciones basicas

Todavia no incluye:

- sesiones persistentes de larga duracion
- aprobaciones interactivas para permisos `ask`
- automatizacion completa del ciclo de edicion sobre cualquier repo

## Comandos

```bash
odrys
npm link
node src/cli.js init
node src/cli.js run "crear una CLI para gestionar notas" --provider mock
node src/cli.js doctor
node src/cli.js workspace scan --workspace ../mi-proyecto
node src/cli.js workspace write .odrys-sandbox/todo.txt --content "primera nota"
node src/cli.js workspace patch --workspace ../mi-proyecto --file ./cambio.patch
```

## Cliente

La experiencia principal ya puede lanzarse como un cliente de terminal propio.

Si haces:

```bash
npm link
odrys
```

se abre la TUI de Odrys.

La TUI ya muestra:

- estado actual de workspace y proveedor
- resumen del run ejecutado
- operaciones aplicadas por `Cocinero`
- historial reciente desde `logs/runs/`

Si `npm link` falla por permisos, usa el instalador local:

```bash
chmod +x install-local.sh
./install-local.sh
odrys
```

Tambien puedes arrancarlo sin instalar nada:

```bash
./odrys
```

## Subir a GitHub

El repositorio ya ignora artefactos locales como:

- `node_modules/`
- `.odrys-sandbox/`
- `logs/runs/`
- `.codex`
- `opencode-dev/`

La carpeta `opencode-dev/` se ha usado solo como referencia local y no forma parte de Odrys.

Dentro del cliente puedes usar:

- texto libre para lanzar una tarea
- `/scan`
- `/doctor`
- `/runs`
- `/show <n>`
- `/workspace <ruta>`
- `/provider <name>`
- `/model <id>`
- `/run <objetivo>`
- `/exit`

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

## Alcance real de esta v1

Esto todavia no aplica cambios automaticos sobre el workspace objetivo.
Pero ya tiene base real para leer, inspeccionar, validar permisos y construir contexto sobre un repo local.

El siguiente salto natural seria:

- conectar `Cocinero` a cambios automaticos sobre el workspace
- anadir aprobaciones interactivas para `ask`
- anadir subagentes cuando de verdad hagan falta
