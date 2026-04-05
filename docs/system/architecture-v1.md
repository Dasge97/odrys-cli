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

### `cmd/odrys/main.go`

Punto de entrada.
Expone TUI, inicializacion, diagnostico, ejecucion y workspace.

### `internal/backend/runtime.go`

Orquesta planificacion, ejecucion, review, resumen, providers, workspace y persistencia.

### `internal/backend/config.go`

Carga scaffold, defaults y configuracion del proyecto.

### `internal/app/model.go`

Implementa la experiencia Bubble Tea y el flujo visual principal.

### `internal/backend/types.go`

Define contratos compartidos del runtime y los agentes.

El runtime incluye tambien tools locales, permisos, estado, providers y operaciones.

## Filosofia

La v1 resuelve antes el `control plane` que la automatizacion total.
Es mejor tener una herramienta pequena, clara y fiable que una pseudo-plataforma enorme con demasiadas piezas mal acopladas.
