function escapeRegex(text) {
  return text.replace(/[|\\{}()[\]^$+?.]/g, "\\$&")
}

function patternToRegex(pattern) {
  const parts = Array.from(pattern, (char) => {
    if (char === "*") return ".*"
    if (char === "?") return "."
    return escapeRegex(char)
  })
  return new RegExp(`^${parts.join("")}$`)
}

function entriesFor(value) {
  if (!value) return []
  if (typeof value === "string") return [["*", value]]
  return Object.entries(value)
}

export function evaluatePermission(config, action, target) {
  const rules = entriesFor(config?.[action])
  let match = "ask"

  for (const [pattern, decision] of rules) {
    if (patternToRegex(pattern).test(target)) {
      match = decision
    }
  }

  return match
}

export function assertPermission(config, action, target) {
  const decision = evaluatePermission(config, action, target)
  if (decision === "allow") return
  if (decision === "deny") {
    throw new Error(`Permiso denegado para ${action}: ${target}`)
  }
  throw new Error(`Aprobacion requerida para ${action}: ${target}`)
}
