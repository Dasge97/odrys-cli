# Sistema CLI de Agentes Orquestados: Arquitectura V1

## Objetivo

Odrys CLI es una herramienta personal para coordinar agentes de IA sobre tareas de desarrollo sin depender de una sesion conversacional opaca.

El sistema se basa en una idea simple:

- el LLM no es el sistema
- el `worker` es el sistema

## Principio central

Los modelos son `stateless`.
Por eso toda la inteligencia operativa debe vivir en:

- el flujo del `worker`
- la memoria externa en archivos
- el contrato JSON de cada agente
- la validacion posterior

## Modelo recomendado

La v1 usa solo estos agentes:

1. `Metre`
2. `Cocinero`
3. `Auditor`
4. `Caja` solo cuando aporte valor

Y estos modulos no LLM:

1. `Context Engine`
2. `State Manager`
3. `Worker`

## Regla de diseno

Separar agentes solo cuando exista conflicto real de incentivos.

Por eso:

- `Cocinero` y `Auditor` deben estar separados
- `Builder` y `Fixer` no
- `Context Selector` no merece ser un agente en v1
- `Final Evaluator` tampoco

## Flujo nominal

1. El usuario da un objetivo
2. El `worker` resuelve el `workspace` objetivo
3. Construye contexto minimo
4. Decide si planificar
5. `Cocinero` propone avance
6. `Auditor` valida
7. Si falla, vuelve a `Cocinero`
8. Si pasa, se actualiza `project_state.md`
9. Se guarda un log estructurado

## Resultado buscado

Una base minima, fiable y extensible para una CLI personal inspirada en OpenCode, pero mucho mas pequena y controlable.

## Documentacion principal

- `README.md`
- `docs/system/architecture-v1.md`
- `docs/system/worker-guide.md`
- `system/base_system_prompt.md`
- `agents/*.md`
