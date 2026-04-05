# Architecture

## Componentes

- `cli`: interfaz de terminal
- `worker`: orquestacion
- `context-engine`: seleccion de contexto
- `workspace`: resolucion del repo objetivo
- `tools`: lectura e inspeccion local
- `permission`: reglas por accion
- `state-manager`: memoria externa y logs
- `provider`: adaptador al modelo

## Flujo

1. cargar config
2. cargar contexto base
3. planificar si hace falta
4. ejecutar tarea
5. revisar
6. reiterar o cerrar
7. resumir estado y guardar logs
