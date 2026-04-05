import { writeTextFile } from "./write.js"
import { readTextFile } from "./read.js"

function parseBlocks(patchText) {
  const lines = patchText.replaceAll("\r\n", "\n").split("\n")
  const blocks = []
  let current = null

  for (const line of lines) {
    if (line.startsWith("*** Update File: ")) {
      if (current) blocks.push(current)
      current = {
        type: "update",
        path: line.slice("*** Update File: ".length),
        hunks: []
      }
      continue
    }

    if (line.startsWith("*** Add File: ")) {
      if (current) blocks.push(current)
      current = {
        type: "add",
        path: line.slice("*** Add File: ".length),
        lines: []
      }
      continue
    }

    if (!current) continue

    if (current.type === "add") {
      if (line.startsWith("+")) current.lines.push(line.slice(1))
      continue
    }

    if (current.type === "update") {
      if (line.startsWith("@@")) {
        current.hunks.push({ search: "", replace: [] })
        continue
      }
      if (current.hunks.length === 0) {
        current.hunks.push({ search: "", replace: [] })
      }
      const hunk = current.hunks[current.hunks.length - 1]
      if (line.startsWith("-")) {
        hunk.search += `${line.slice(1)}\n`
        continue
      }
      if (line.startsWith("+")) {
        hunk.replace.push(line.slice(1))
        continue
      }
      if (line.startsWith(" ")) {
        hunk.search += `${line.slice(1)}\n`
        hunk.replace.push(line.slice(1))
      }
    }
  }

  if (current) blocks.push(current)
  return blocks
}

function applyUpdate(original, hunks, path) {
  let output = original
  for (const hunk of hunks) {
    const search = hunk.search.endsWith("\n") ? hunk.search.slice(0, -1) : hunk.search
    const replace = hunk.replace.join("\n")
    if (!search) continue
    if (!output.includes(search)) {
      throw new Error(`No se encontro el bloque a reemplazar en ${path}`)
    }
    output = output.replace(search, replace)
  }
  return output
}

export async function applyPatch(workspace, patchText, permission) {
  const blocks = parseBlocks(patchText)
  const changes = []

  for (const block of blocks) {
    if (block.type === "add") {
      const result = await writeTextFile(workspace, block.path, block.lines.join("\n"), permission)
      changes.push({ type: "add", ...result })
      continue
    }

    if (block.type === "update") {
      const original = await readTextFile(workspace, block.path, permission)
      const next = applyUpdate(original, block.hunks, block.path)
      const result = await writeTextFile(workspace, block.path, next, permission)
      changes.push({ type: "update", ...result })
    }
  }

  return changes
}
