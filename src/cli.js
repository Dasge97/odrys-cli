#!/usr/bin/env node

import { resolve } from "node:path"
import { ensureProjectScaffold, loadConfig, projectRoot } from "./core/config.js"
import { runWorker } from "./core/worker.js"
import { resolveWorkspace, scanWorkspace } from "./core/workspace.js"
import { writeTextFile } from "./tools/write.js"
import { applyPatch } from "./tools/apply-patch.js"
import { readFile } from "node:fs/promises"
import { startTui } from "./tui/client.js"

function parseArgs(argv) {
  const [command = "tui", ...rest] = argv
  const flags = {}
  const positionals = []

  for (let i = 0; i < rest.length; i += 1) {
    const item = rest[i]
    if (!item) continue
    if (item.startsWith("--")) {
      const key = item.slice(2)
      const next = rest[i + 1]
      if (!next || next.startsWith("--")) {
        flags[key] = true
        continue
      }
      flags[key] = next
      i += 1
      continue
    }
    positionals.push(item)
  }

  return { command, flags, positionals }
}

function printHelp() {
  console.log(`Odrys CLI

Uso:
  odrys
  node src/cli.js init
  node src/cli.js doctor
  node src/cli.js tui
  node src/cli.js run "tu objetivo" [--provider mock]
  node src/cli.js workspace scan [--workspace ../repo]
  node src/cli.js workspace write ruta.txt --content "hola"
  node src/cli.js workspace patch --file ./cambio.patch

Opciones:
  --provider <name>   mock | openai-compatible
  --model <id>        sobrescribe el modelo del config
  --no-plan           desactiva planning automatico
  --workspace <path>  repo objetivo sobre el que construir contexto
`)
}

async function main() {
  const root = projectRoot()
  const { command, flags, positionals } = parseArgs(process.argv.slice(2))

  if (command === "help" || command === "--help" || command === "-h") {
    printHelp()
    return
  }

  if (command === "tui") {
    await ensureProjectScaffold(root)
    await startTui()
    return
  }

  if (command === "init") {
    await ensureProjectScaffold(root)
    console.log(`Scaffold listo en ${root}`)
    return
  }

  if (command === "doctor") {
    await ensureProjectScaffold(root)
    const cfg = await loadConfig(root)
    console.log(JSON.stringify({
      root,
      configPath: resolve(root, "odrys.config.json"),
      provider: cfg.provider,
      workspace: cfg.workspace,
      permission: cfg.permission,
      worker: cfg.worker
    }, null, 2))
    return
  }

  if (command === "workspace") {
    await ensureProjectScaffold(root)
    const cfg = await loadConfig(root)
    if (flags.workspace) cfg.workspace.path = String(flags.workspace)

    const subcommand = positionals[0] ?? "scan"
    if (subcommand === "scan") {
      const workspace = await resolveWorkspace(root, cfg.workspace)
      const snapshot = await scanWorkspace(workspace, cfg.permission)
      console.log(JSON.stringify(snapshot, null, 2))
      return
    }

    if (subcommand === "write") {
      const target = positionals[1]
      const content = flags.content
      if (!target || typeof content !== "string") {
        throw new Error("Uso: node src/cli.js workspace write ruta.txt --content \"texto\"")
      }
      const workspace = await resolveWorkspace(root, cfg.workspace)
      const result = await writeTextFile(workspace, target, content, cfg.permission)
      console.log(JSON.stringify(result, null, 2))
      return
    }

    if (subcommand === "patch") {
      const filepath = flags.file
      if (typeof filepath !== "string") {
        throw new Error("Uso: node src/cli.js workspace patch --file ./cambio.patch")
      }
      const workspace = await resolveWorkspace(root, cfg.workspace)
      const patchText = await readFile(resolve(root, filepath), "utf8")
      const result = await applyPatch(workspace, patchText, cfg.permission)
      console.log(JSON.stringify(result, null, 2))
      return
    }

    throw new Error("Subcomando de workspace no soportado. Usa: scan, write o patch")
  }

  if (command === "run") {
    await ensureProjectScaffold(root)
    const goal = positionals.join(" ").trim()
    if (!goal) {
      throw new Error("Debes indicar un objetivo. Ejemplo: node src/cli.js run \"crear una API de tareas\"")
    }

    const cfg = await loadConfig(root)
    if (flags.provider) cfg.provider.name = String(flags.provider)
    if (flags.model) cfg.provider.model = String(flags.model)
    if (flags["no-plan"]) cfg.worker.planner.auto = false
    if (flags.workspace) cfg.workspace.path = String(flags.workspace)

    const result = await runWorker({
      root,
      goal,
      config: cfg
    })

    console.log(JSON.stringify(result, null, 2))
    return
  }

  printHelp()
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error))
  process.exitCode = 1
})
