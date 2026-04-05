function hasKeys(value, keys) {
  return keys.every((key) => Object.prototype.hasOwnProperty.call(value, key))
}

export function validateAgentOutput(agent, value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`La salida del agente ${agent} no es un objeto JSON valido`)
  }

  if (agent === "planner") {
    if (!hasKeys(value, ["status", "summary", "phases", "next_action"])) {
      throw new Error("Salida de planner invalida")
    }
    return value
  }

  if (agent === "executor") {
    if (!hasKeys(value, ["status", "summary", "changes", "assumptions", "open_questions", "operations", "next_action"])) {
      throw new Error("Salida de executor invalida")
    }
    if (!Array.isArray(value.operations)) {
      throw new Error("Salida de executor invalida: operations debe ser un array")
    }
    return value
  }

  if (agent === "reviewer") {
    if (!hasKeys(value, ["status", "summary", "errors", "verified_against", "next_action"])) {
      throw new Error("Salida de reviewer invalida")
    }
    return value
  }

  if (agent === "summarizer") {
    if (!hasKeys(value, ["status", "project_state", "next_action"])) {
      throw new Error("Salida de summarizer invalida")
    }
    return value
  }

  throw new Error(`Agente no soportado: ${agent}`)
}
