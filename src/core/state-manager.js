import { mkdir, readFile, readdir, writeFile } from "node:fs/promises"
import { resolve } from "node:path"
import { getAgent } from "./agents.js"

export async function readPrompt(root, relative) {
  return readFile(resolve(root, relative), "utf8")
}

export async function readAgentPrompt(root, role) {
  const agent = getAgent(role)
  return readPrompt(root, `agents/${agent.slug}_prompt.md`)
}

export async function readProjectState(root) {
  return readFile(resolve(root, "project/project_state.md"), "utf8")
}

export async function updateProjectState(root, content) {
  await writeFile(resolve(root, "project/project_state.md"), content.trim() + "\n", "utf8")
}

export async function writeRunLog(root, payload) {
  const dir = resolve(root, "logs/runs")
  await mkdir(dir, { recursive: true })
  const stamp = new Date().toISOString().replaceAll(":", "-")
  const filepath = resolve(dir, `${stamp}.json`)
  await writeFile(filepath, JSON.stringify(payload, null, 2) + "\n", "utf8")
  return filepath
}

export async function listRunLogs(root, limit = 10) {
  const dir = resolve(root, "logs/runs")
  await mkdir(dir, { recursive: true })
  const items = await readdir(dir)
  return items
    .filter((item) => item.endsWith(".json"))
    .sort()
    .slice(-limit)
    .reverse()
    .map((item) => resolve(dir, item))
}

export async function readRunLog(filepath) {
  const raw = await readFile(filepath, "utf8")
  return JSON.parse(raw)
}
