import { access, mkdir, readFile, writeFile } from "node:fs/promises"
import { dirname, resolve } from "node:path"

const DEFAULT_FILES = {
  "project/spec.md": "# Spec\n",
  "project/architecture.md": "# Architecture\n",
  "project/rules.md": "# Rules\n",
  "project/checklist.md": "# Checklist\n",
  "project/project_state.md": "# Project State\n",
  "logs/.gitkeep": "",
  "agents/metre_prompt.md": "Eres `Metre`.\n",
  "agents/cocinero_prompt.md": "Eres `Cocinero`.\n",
  "agents/auditor_prompt.md": "Eres `Auditor`.\n",
  "agents/caja_prompt.md": "Eres `Caja`.\n",
  "system/base_system_prompt.md": "Sistema de agentes.\n",
  "schemas/output_schema_global.json": "{}\n"
}

const DEFAULT_CONFIG = {
  provider: {
    name: "mock",
    model: "odrys-mock-1"
  },
  workspace: {
    path: ".",
    include: ["package.json", "README.md", "src", "app", "lib"],
    exclude: ["node_modules", ".git", "opencode-dev", "dist", "build", "coverage"]
  },
  permission: {
    read: { "*": "allow" },
    edit: {
      "*": "ask",
      ".odrys-sandbox/**": "allow"
    },
    list: { "*": "allow" },
    search: { "*": "allow" },
    bash: {
      "*": "ask",
      "git status --short": "allow",
      "git rev-parse --is-inside-work-tree": "allow"
    }
  },
  worker: {
    maxReviewLoops: 2,
    planner: {
      auto: true
    },
    summarizeOnSuccess: true
  }
}

export function projectRoot() {
  return process.cwd()
}

export async function fileExists(filepath) {
  try {
    await access(filepath)
    return true
  } catch {
    return false
  }
}

export async function ensureProjectScaffold(root) {
  for (const [relative, content] of Object.entries(DEFAULT_FILES)) {
    const filepath = resolve(root, relative)
    if (await fileExists(filepath)) continue
    await mkdir(dirname(filepath), { recursive: true })
    await writeFile(filepath, content, "utf8")
  }

  const configPath = resolve(root, "odrys.config.json")
  if (!(await fileExists(configPath))) {
    await writeFile(configPath, JSON.stringify(DEFAULT_CONFIG, null, 2) + "\n", "utf8")
  }
}

export async function loadConfig(root) {
  const configPath = resolve(root, "odrys.config.json")
  const raw = await readFile(configPath, "utf8")
  const parsed = JSON.parse(raw)

  return {
    provider: {
      ...DEFAULT_CONFIG.provider,
      ...(parsed.provider ?? {})
    },
    workspace: {
      ...DEFAULT_CONFIG.workspace,
      ...(parsed.workspace ?? {})
    },
    permission: {
      ...DEFAULT_CONFIG.permission,
      ...(parsed.permission ?? {})
    },
    worker: {
      ...DEFAULT_CONFIG.worker,
      ...(parsed.worker ?? {}),
      planner: {
        ...DEFAULT_CONFIG.worker.planner,
        ...(parsed.worker?.planner ?? {})
      }
    }
  }
}
