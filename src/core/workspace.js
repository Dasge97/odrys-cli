import { stat } from "node:fs/promises"
import { resolve } from "node:path"
import { listDirectory } from "../tools/list.js"
import { readTextFile } from "../tools/read.js"
import { searchInWorkspace } from "../tools/search.js"
import { runCommand } from "../tools/bash.js"

export async function resolveWorkspace(root, workspaceConfig) {
  const path = resolve(root, workspaceConfig?.path ?? ".")
  const info = await stat(path)
  if (!info.isDirectory()) {
    throw new Error(`El workspace debe ser un directorio: ${path}`)
  }

  return {
    root: path,
    include: workspaceConfig?.include ?? [],
    exclude: workspaceConfig?.exclude ?? []
  }
}

async function safeRead(workspace, relativePath, permission) {
  try {
    return await readTextFile(workspace, relativePath, permission)
  } catch (error) {
    return `No se pudo leer ${relativePath}: ${error instanceof Error ? error.message : String(error)}`
  }
}

export async function scanWorkspace(workspace, permission) {
  const files = await listDirectory(workspace, ".", permission)
  const selected = files
    .filter((item) => workspace.include.some((entry) => item.path === entry || item.path.startsWith(`${entry}/`)))
    .slice(0, 40)

  const keyFiles = []

  for (const item of selected) {
    if (item.type !== "file") continue
    if (!/(package\.json|README|tsconfig|pyproject|Cargo\.toml|go\.mod|pom\.xml|Gemfile|requirements)/i.test(item.path)) {
      continue
    }
    keyFiles.push({
      path: item.path,
      content: await safeRead(workspace, item.path, permission)
    })
  }

  let git = null
  try {
    git = {
      inside_work_tree: (await runCommand(workspace, "git rev-parse --is-inside-work-tree", permission)).stdout.trim(),
      status: (await runCommand(workspace, "git status --short", permission)).stdout.trim()
    }
  } catch (error) {
    git = {
      error: error instanceof Error ? error.message : String(error)
    }
  }

  let hints = []
  try {
    hints = await searchInWorkspace(workspace, "TODO|FIXME|HACK", permission, { limit: 20 })
  } catch {
    hints = []
  }

  return {
    root: workspace.root,
    file_count: files.length,
    selected_files: selected,
    key_files: keyFiles,
    git,
    search_hints: hints
  }
}
