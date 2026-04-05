import { readdir } from "node:fs/promises"
import { resolve, relative } from "node:path"
import { assertPermission } from "../core/permission.js"

async function walk(root, current, exclude, output) {
  const entries = await readdir(current, { withFileTypes: true })
  for (const entry of entries) {
    const absolute = resolve(current, entry.name)
    const rel = relative(root, absolute) || "."
    if (exclude.some((item) => rel === item || rel.startsWith(`${item}/`))) continue
    output.push({
      path: rel,
      type: entry.isDirectory() ? "directory" : "file"
    })
    if (entry.isDirectory()) {
      await walk(root, absolute, exclude, output)
    }
  }
}

export async function listDirectory(workspace, relativePath, permission) {
  assertPermission(permission, "list", relativePath)
  const absolute = resolve(workspace.root, relativePath)
  const output = []
  await walk(workspace.root, absolute, workspace.exclude ?? [], output)
  return output
}
