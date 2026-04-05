import { applyPatch } from "../tools/apply-patch.js"
import { writeTextFile } from "../tools/write.js"
import { readTextFile } from "../tools/read.js"

async function executeOperation(workspace, permission, operation) {
  if (operation.tool === "write") {
    const result = await writeTextFile(workspace, operation.path, operation.content ?? "", permission)
    return {
      tool: "write",
      path: operation.path,
      result,
      content: await readTextFile(workspace, operation.path, permission)
    }
  }

  if (operation.tool === "apply_patch") {
    const result = await applyPatch(workspace, operation.patch ?? "", permission)
    return {
      tool: "apply_patch",
      result
    }
  }

  throw new Error(`Operacion no soportada: ${operation.tool}`)
}

export async function applyExecutorOperations(workspace, permission, operations) {
  const applied = []
  for (const operation of operations ?? []) {
    applied.push(await executeOperation(workspace, permission, operation))
  }
  return applied
}
