export const AGENTS = {
  planner: {
    role: "planner",
    slug: "metre",
    name: "Metre",
    purpose: "descompone objetivos en fases y tareas"
  },
  executor: {
    role: "executor",
    slug: "cocinero",
    name: "Cocinero",
    purpose: "implementa y corrige"
  },
  reviewer: {
    role: "reviewer",
    slug: "auditor",
    name: "Auditor",
    purpose: "revisa con criterio estricto"
  },
  summarizer: {
    role: "summarizer",
    slug: "caja",
    name: "Caja",
    purpose: "condensa el estado del proyecto"
  }
}

export function getAgent(role) {
  const agent = AGENTS[role]
  if (!agent) throw new Error(`Rol de agente no soportado: ${role}`)
  return agent
}
