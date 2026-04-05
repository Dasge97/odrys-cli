import { readFile } from "node:fs/promises"
import { resolve } from "node:path"
import { assertPermission } from "../core/permission.js"
import { listDirectory } from "./list.js"

export async function searchInWorkspace(workspace, pattern, permission, options = {}) {
  assertPermission(permission, "search", pattern)
  const files = await listDirectory(workspace, ".", permission)
  const regex = new RegExp(pattern, "i")
  const matches = []

  for (const item of files) {
    if (item.type !== "file") continue
    if (matches.length >= (options.limit ?? 20)) break
    try {
      const content = await readFile(resolve(workspace.root, item.path), "utf8")
      const lines = content.split("\n")
      for (let index = 0; index < lines.length; index += 1) {
        if (!regex.test(lines[index])) continue
        matches.push({
          path: item.path,
          line: index + 1,
          text: lines[index].trim()
        })
        if (matches.length >= (options.limit ?? 20)) break
      }
    } catch {
      continue
    }
  }

  return matches
}
