# Backend Phases

Odrys necesita backend propio para soportar una opcion tipo `ChatGPT Plus/Pro` sin depender de hacks en el cliente local.

## Fase 1

Base del servicio:

- binario `odrys-core`
- API HTTP local
- almacenamiento persistente en `.odrys/core-state.json`
- sesiones propias del backend

Endpoints:

- `GET /health`
- `GET /api/v1/sessions`
- `POST /api/v1/sessions`
- `GET /api/v1/sessions/:id`

## Fase 2

Auth OpenAI:

- `GET /api/v1/openai/status`
- `POST /api/v1/openai/connect/api-key`
- `POST /api/v1/openai/connect/device/start`
- `GET /api/v1/openai/connect/device/poll/:id`
- `POST /api/v1/openai/disconnect`

Persistencia:

- auth local de Odrys en `.odrys/auth.json`
- sesiones de auth del backend en `.odrys/core-state.json`

## Fase 3

Implementada:

- identidad propia de Odrys
- tokens de sesion del backend para el cliente
- asociacion usuario <-> conexiones OpenAI
- bootstrap de cliente via `POST /api/v1/core/bootstrap`
- proteccion de endpoints mediante token local de `odrys-core`

## Fase 4

En progreso:

- `odrys-core` como fuente autoritativa del estado/capacidades de OpenAI
- broker real para `ChatGPT Plus/Pro`
- inference proxy
- refresh y revocacion gestionados 100% por backend
- politicas de uso
- auditoria y rate limit
