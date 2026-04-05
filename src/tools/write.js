import { mkdir, writeFile } from "node:fs/promises"
import { dirname, resolve } from "node:path"
import { assertPermission } from "../core/permission.js"

export async function writeTextFile(workspace, relativePath, content, permission) {
  assertPermission(permission, "edit", relativePath)
  const filepath = resolve(workspace.root, relativePath)
  await mkdir(dirname(filepath), { recursive: true })
  await writeFile(filepath, content, "utf8")
  return {
    path: relativePath,
    bytes: Buffer.byteLength(content, "utf8")
  }
}
