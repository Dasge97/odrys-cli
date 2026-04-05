import { execFile } from "node:child_process"
import { promisify } from "node:util"
import { assertPermission } from "../core/permission.js"

const execFileAsync = promisify(execFile)

function parseCommand(command) {
  const trimmed = command.trim()
  if (!trimmed) throw new Error("Comando vacio")
  const parts = trimmed.split(/\s+/)
  return {
    file: parts[0],
    args: parts.slice(1)
  }
}

export async function runCommand(workspace, command, permission) {
  assertPermission(permission, "bash", command)
  const parsed = parseCommand(command)
  const result = await execFileAsync(parsed.file, parsed.args, {
    cwd: workspace.root,
    maxBuffer: 1024 * 1024
  })
  return {
    command,
    stdout: result.stdout,
    stderr: result.stderr
  }
}
