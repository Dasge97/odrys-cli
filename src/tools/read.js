import { readFile } from "node:fs/promises"
import { resolve } from "node:path"
import { assertPermission } from "../core/permission.js"

export async function readTextFile(workspace, relativePath, permission) {
  assertPermission(permission, "read", relativePath)
  return readFile(resolve(workspace.root, relativePath), "utf8")
}
