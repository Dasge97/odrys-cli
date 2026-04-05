import { readFile } from "node:fs/promises"
import { resolve } from "node:path"
import { scanWorkspace } from "./workspace.js"

async function readProjectFile(root, relative) {
  const filepath = resolve(root, relative)
  const content = await readFile(filepath, "utf8")
  return {
    path: relative,
    content
  }
}

export async function buildContext(root, mode = "execute", options = {}) {
  const files = [
    "project/spec.md",
    "project/architecture.md",
    "project/rules.md",
    "project/project_state.md"
  ]

  if (mode === "review") {
    files.push("project/checklist.md")
  }

  const items = await Promise.all(files.map((file) => readProjectFile(root, file)))
  if (options.workspace) {
    const snapshot = await scanWorkspace(options.workspace, options.permission ?? {})
    items.push({
      path: "workspace/snapshot.json",
      content: JSON.stringify(snapshot, null, 2)
    })
  }
  return items
}

export function formatContext(context) {
  return context
    .map((item) => `## ${item.path}\n\n${item.content.trim()}`)
    .join("\n\n")
    .trim()
}

export function shouldUsePlanner(goal) {
  const normalized = goal.trim()
  if (normalized.length > 120) return true
  const separators = [" y ", " luego ", " después ", " after ", ",", ";"]
  return separators.some((token) => normalized.toLowerCase().includes(token))
}
