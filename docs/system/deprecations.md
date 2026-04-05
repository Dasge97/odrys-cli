# Deprecaciones del modelo anterior

La arquitectura antigua separaba mas roles de los necesarios.

Se eliminan como agentes principales:

- `Fixer`
- `Context Selector`
- `Final Evaluator`

## Razon

La v1 personal necesita fiabilidad y simplicidad, no teatralidad arquitectonica.

- `Fixer` se integra en `Cocinero`
- la seleccion de contexto pasa al `worker`
- el cierre se decide con `Auditor` y reglas del `worker`
