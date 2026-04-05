# Worker Guide

## Responsabilidades

El `worker` debe:

- decidir si hace falta `Metre`
- cargar prompts y contexto
- resolver el `workspace` objetivo
- construir snapshot local del repo
- invocar agentes
- validar JSON
- iterar con `Auditor`
- actualizar `project_state.md`
- guardar logs de cada run

## Politica de contexto

Siempre cargar:

- `project/spec.md`
- `project/rules.md`
- `project/architecture.md`
- `project/project_state.md`

Si hay `workspace`, cargar tambien:

- raiz objetivo
- lista inicial de archivos
- archivos clave detectados
- estado git si esta permitido

Durante review, incluir tambien:

- `project/checklist.md`

## Politica de planning

Usar `Metre` si el objetivo:

- es largo
- contiene varias acciones
- tiene dependencias o varias fases

## Politica de cierre

Cerrar cuando:

- `Auditor` devuelve `approved`
- no quedan errores `critical` o `major`
- el `worker` no detecta mas iteraciones utiles
