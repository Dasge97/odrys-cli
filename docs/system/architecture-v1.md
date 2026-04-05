# Arquitectura V1

## Que implementa esta repo

Esta repo ya no es solo una coleccion de prompts.
Ahora contiene una implementacion minima de una CLI local con:

- una entrada de terminal
- un `worker` determinista
- carga de prompts desde disco
- seleccion de contexto desde `project/`
- snapshot local de un `workspace` objetivo
- tools locales para `list`, `read`, `search` y `bash`
- permisos basicos por accion
- registro de ejecuciones en `logs/`
- proveedor `mock` para probar el flujo sin red
- proveedor `openai-compatible` para conectar un modelo real

## Componentes

### `src/cli.js`

Punto de entrada.
Expone comandos de inicializacion, diagnostico y ejecucion.

### `src/core/worker.js`

Orquesta el flujo completo de planificacion, ejecucion, review, resumen y persistencia.

### `src/core/context-engine.js`

Selecciona el contexto base que se inyecta a cada agente desde `project/` y desde el `workspace` objetivo.

### `src/core/state-manager.js`

Lee y escribe `project_state.md`, crea logs de ejecucion y actualiza archivos base.

### `src/core/workspace.js`

Resuelve el workspace objetivo y construye snapshots deterministas del repositorio local.

### `src/llm/*`

Adaptadores de proveedor.
La interfaz es pequena y estable.

### `src/tools/*`

Implementa las herramientas locales que usa el sistema para inspeccionar un repo.

## Filosofia

La v1 resuelve antes el `control plane` que la automatizacion total.
Es mejor tener una herramienta pequena, clara y fiable que una pseudo-plataforma enorme con demasiadas piezas mal acopladas.
