# Spec

Construir una CLI personal de agentes orquestados para desarrollo asistido por IA.

## Objetivos funcionales

- permitir ejecutar un objetivo de desarrollo desde terminal
- usar un `worker` determinista
- soportar `Metre`, `Cocinero`, `Auditor` y `Caja`
- cargar memoria externa desde archivos
- registrar cada ejecucion
- soportar un proveedor `mock`
- soportar un proveedor `openai-compatible`
- soportar un `workspace` objetivo local
- construir un snapshot real del repo objetivo
- aplicar permisos basicos sobre tools locales

## No objetivos de la v1

- server HTTP
- TUI avanzada
- multiusuario
- plugins
- MCP
- LSP
- ejecucion distribuida
