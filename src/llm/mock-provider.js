function buildPlan(goal) {
  return {
    status: "planned",
    summary: `Plan base para: ${goal}`,
    phases: [
      {
        name: "base",
        goal: "establecer la estructura inicial",
        tasks: [
          {
            id: "task-1",
            description: goal,
            done_when: ["la tarea principal tiene una primera implementacion o propuesta concreta"],
            depends_on: []
          }
        ]
      }
    ],
    next_action: "execute"
  }
}

function buildOperations(task) {
  const createMatch = task.match(/crear archivo\s+([^\s]+)\s+con contenido\s+(.+)/i)
  if (createMatch) {
    return [
      {
        tool: "write",
        path: createMatch[1],
        content: createMatch[2]
      }
    ]
  }

  const writeMatch = task.match(/escribir en\s+([^\s]+)\s*:\s*(.+)/i)
  if (writeMatch) {
    return [
      {
        tool: "write",
        path: writeMatch[1],
        content: writeMatch[2]
      }
    ]
  }

  return []
}

export class MockProvider {
  constructor(config) {
    this.config = config
  }

  async run(input) {
    if (input.agent === "planner") {
      return buildPlan(input.goal)
    }

    if (input.agent === "executor") {
      const retry = input.feedback?.reviewer?.errors?.length ? "Corrige errores previos y reintenta." : "Primera ejecucion."
      const operations = buildOperations(input.task)
      return {
        status: "success",
        summary: `${input.agent_name ?? "Cocinero"} preparado para trabajar la tarea: ${input.task}. ${retry}`,
        changes: [
          "Se ha producido una propuesta estructurada de implementacion",
          "Se ha generado una salida trazable para revision"
        ],
        assumptions: [
          "La v1 prioriza arquitectura y control de flujo sobre automatizacion total"
        ],
        open_questions: [],
        operations,
        next_action: "review"
      }
    }

    if (input.agent === "reviewer") {
      return {
        status: "approved",
        summary: `${input.agent_name ?? "Auditor"} considera que la propuesta para la tarea "${input.task}" cumple el contrato minimo de la v1`,
        errors: [],
        verified_against: [
          "project/spec.md",
          "project/architecture.md",
          "project/rules.md",
          "project/checklist.md"
        ],
        next_action: "finish"
      }
    }

    if (input.agent === "summarizer") {
      return {
        status: "updated",
        project_state: `# Project State

Ultima actualizacion automatica.

- Objetivo reciente: ${input.goal}
- Estado: se ejecuto el flujo completo del worker
- Proximo paso natural: habilitar cambios automaticos y aprobaciones interactivas
`,
        next_action: "store_state"
      }
    }

    throw new Error(`Agente no soportado por mock: ${input.agent}`)
  }
}
