import blessed from "blessed"
import { loadConfig, projectRoot } from "../core/config.js"
import { resolveWorkspace, scanWorkspace } from "../core/workspace.js"
import { runWorker } from "../core/worker.js"
import { listRunLogs, readRunLog } from "../core/state-manager.js"

const LOGO = [
  " ▄█████▄  ██████▄  ██████▄  ▀██  ██▀  ▄██████",
  "██▀   ▀██ ██   ▀██ ██   ▀██   ████   ██▀",
  "██     ██ ██    ██ ██████▀     ██    ▀████▄",
  "██▄   ▄██ ██   ▄██ ██  ██      ██        ▀██",
  " ▀█████▀  ██████▀  ██   ██     ██   ██████▀"
]

function normalizeLogo(lines) {
  const width = Math.max(...lines.map((line) => line.length))
  return lines.map((line) => line.padEnd(width, " "))
}

function homeLayout(screen) {
  const logoLines = normalizeLogo(LOGO)
  const logoWidth = Math.max(...logoLines.map((line) => line.length))
  const screenWidth = screen.width || 80
  const logoLeft = Math.max(0, Math.floor((screenWidth - logoWidth) / 2))
  return {
    logoLines,
    logoWidth,
    logoLeft,
    frameWidth: 48
  }
}

function stamp() {
  return new Date().toISOString()
}

function formatRunResult(result) {
  const lines = [
    `{green-fg}Run completado{/}`,
    `goal: ${result.goal}`,
    `planned: ${String(result.planned)}`,
    `log: ${result.log_path}`
  ]

  for (const item of result.tasks) {
    lines.push("")
    lines.push(`{cyan-fg}Tarea{/}: ${item.task}`)
    lines.push(`Cocinero: ${item.executor.summary}`)
    if (item.applied_operations?.length) {
      lines.push("Operaciones aplicadas:")
      for (const operation of item.applied_operations) {
        if (operation.tool === "write") {
          lines.push(`- write ${operation.path}`)
          continue
        }
        if (operation.tool === "apply_patch") {
          lines.push(`- apply_patch (${operation.result.length} cambios)`)
          continue
        }
        lines.push(`- ${operation.tool}`)
      }
    }
    lines.push(`Auditor: ${item.reviewer.summary}`)
  }

  return lines
}

function formatScan(snapshot) {
  const lines = [
    "{green-fg}Workspace escaneado{/}",
    `root: ${snapshot.root}`,
    `files: ${snapshot.file_count}`,
    `seleccionados: ${snapshot.selected_files.length}`
  ]

  if (snapshot.key_files.length) {
    lines.push("archivos clave:")
    for (const item of snapshot.key_files) {
      lines.push(`- ${item.path}`)
    }
  }

  if (snapshot.git?.error) {
    lines.push(`git: ${snapshot.git.error}`)
  }

  return lines
}

function formatDoctor(root, state) {
  return JSON.stringify({
    root,
    provider: state.provider,
    workspace: state.workspace,
    permission: state.permission,
    worker: state.worker
  }, null, 2).split("\n")
}

function formatStoredRun(run, indexLabel = "") {
  const lines = [
    `{green-fg}Run${indexLabel ? ` ${indexLabel}` : ""}{/}`,
    `goal: ${run.goal}`,
    `started_at: ${run.started_at}`,
    `completed_at: ${run.completed_at}`
  ]

  for (const task of run.result?.tasks ?? []) {
    lines.push("")
    lines.push(`{cyan-fg}Tarea{/}: ${task.task}`)
    lines.push(`Cocinero: ${task.executor?.summary ?? "sin resumen"}`)
    if (task.applied_operations?.length) {
      lines.push("Operaciones aplicadas:")
      for (const operation of task.applied_operations) {
        if (operation.tool === "write") {
          lines.push(`- write ${operation.path}`)
          continue
        }
        if (operation.tool === "apply_patch") {
          lines.push("- apply_patch")
          continue
        }
        lines.push(`- ${operation.tool}`)
      }
    }
    lines.push(`Auditor: ${task.reviewer?.summary ?? "sin resumen"}`)
  }

  return lines
}

function formatRunList(files) {
  if (!files.length) return ["No hay runs guardados."]
  return files.map((file, index) => `${index + 1}. ${file.split("/").pop()}`)
}

export async function startTui() {
  const root = projectRoot()
  const cfg = await loadConfig(root)
  const state = structuredClone(cfg)
  const app = {
    root,
    state,
    mode: "home",
    busy: false,
    sessionStartedAt: null,
    transcript: []
  }

  const screen = blessed.screen({
    smartCSR: true,
    dockBorders: true,
    fullUnicode: true,
    title: "Odrys"
  })

  const homeLogo = blessed.box({
    parent: screen,
    top: 2,
    left: 0,
    width: 58,
    height: 5,
    align: "left",
    valign: "middle",
    content: normalizeLogo(LOGO).join("\n"),
    style: {
      fg: "#f2f2f2"
    }
  })

  const homeInputFrame = blessed.box({
    parent: screen,
    top: 10,
    left: 0,
    width: 48,
    height: 5,
    border: "line",
    style: {
      border: { fg: "#4d8dff" },
      bg: "#1f1f1f"
    }
  })

  const homeInput = blessed.textbox({
    parent: homeInputFrame,
    inputOnFocus: true,
    keys: true,
    mouse: true,
    top: 1,
    left: 2,
    right: 2,
    height: 1,
    style: {
      fg: "#f5f5f5",
      bg: "#1f1f1f"
    }
  })

  const homeMeta = blessed.box({
    parent: homeInputFrame,
    bottom: 0,
    left: 2,
    height: 1,
    tags: true,
    content: "{#4d8dff-fg}Cocinero{/}  {white-fg}odrys-mock-1{/}  {gray-fg}mock{/}",
    style: {
      bg: "#1f1f1f"
    }
  })

  const homeHelp = blessed.box({
    parent: screen,
    top: 15,
    left: 0,
    width: 48,
    height: 1,
    tags: true,
    align: "center",
    content: "{#9e9e9e-fg}/help{/}",
    style: {
      fg: "#888888"
    }
  })

  const helpOverlay = blessed.box({
    parent: screen,
    top: "center",
    left: "center",
    width: "68%",
    height: "62%",
    border: "line",
    tags: true,
    hidden: true,
    scrollable: true,
    alwaysScroll: true,
    keys: true,
    vi: true,
    style: {
      border: { fg: "#5c5c5c" },
      bg: "#141414",
      fg: "#f0f0f0",
      scrollbar: {
        bg: "#666666"
      }
    }
  })

  const sessionHeader = blessed.box({
    parent: screen,
    top: 0,
    left: 1,
    right: 1,
    height: 3,
    border: "line",
    hidden: true,
    tags: true,
    style: {
      border: { fg: "#5c5c5c" },
      bg: "#151515"
    }
  })

  const timeline = blessed.box({
    parent: screen,
    top: 4,
    left: 1,
    right: 1,
    bottom: 8,
    tags: true,
    scrollable: true,
    alwaysScroll: true,
    keys: true,
    vi: true,
    hidden: true,
    style: {
      bg: "#0b0b0b",
      fg: "#f0f0f0",
      scrollbar: {
        bg: "#666666"
      }
    }
  })

  const composerFrame = blessed.box({
    parent: screen,
    left: 1,
    right: 1,
    bottom: 2,
    height: 6,
    border: "line",
    hidden: true,
    style: {
      border: { fg: "#4d8dff" },
      bg: "#1f1f1f"
    }
  })

  const composerInput = blessed.textbox({
    parent: composerFrame,
    inputOnFocus: true,
    keys: true,
    mouse: true,
    top: 1,
    left: 2,
    right: 2,
    height: 1,
    style: {
      fg: "#f5f5f5",
      bg: "#1f1f1f"
    }
  })

  const composerMeta = blessed.box({
    parent: composerFrame,
    bottom: 1,
    left: 2,
    height: 1,
    tags: true,
    content: "{#4d8dff-fg}Cocinero{/}  {white-fg}mock{/}  {gray-fg}Odrys{/}",
    style: {
      bg: "#1f1f1f"
    }
  })

  const sessionFooter = blessed.box({
    parent: screen,
    left: 1,
    right: 1,
    bottom: 0,
    height: 1,
    tags: true,
    hidden: true,
    content: "{gray-fg}esc{/} interrupt                               {bold}ctrl+t{/} variants   {bold}tab{/} agents   {bold}ctrl+p{/} commands",
    style: {
      fg: "#8a8a8a"
    }
  })

  function setMode(mode) {
    app.mode = mode
    const home = mode === "home"
    const help = mode === "help"
    const session = mode === "session"
    homeLogo.hidden = !home
    homeInputFrame.hidden = !home
    homeHelp.hidden = !home
    helpOverlay.hidden = !help
    sessionHeader.hidden = !session
    timeline.hidden = !session
    composerFrame.hidden = !session
    sessionFooter.hidden = !session
    if (home) homeInput.focus()
    else if (help) helpOverlay.focus()
    else composerInput.focus()
    render()
  }

  function addTranscript(lines, prefix = "") {
    const next = Array.isArray(lines) ? lines : [lines]
    if (prefix) app.transcript.push(prefix)
    app.transcript.push(...next)
    timeline.setContent(app.transcript.join("\n"))
    timeline.setScrollPerc(100)
  }

  function renderSessionHeader() {
    const title = app.sessionStartedAt
      ? `# New session - ${app.sessionStartedAt}`
      : "# New session"
    sessionHeader.setContent(` {bold}${title}{/}`)
  }

  function render() {
    renderSessionHeader()
    const layout = homeLayout(screen)
    const frameLeft = Math.max(0, layout.logoLeft - 2)
    homeLogo.left = layout.logoLeft
    homeLogo.width = layout.logoWidth
    homeLogo.setContent(layout.logoLines.join("\n"))
    homeInputFrame.left = frameLeft
    homeInputFrame.width = layout.frameWidth
    homeHelp.left = frameLeft
    homeHelp.width = layout.frameWidth
    const meta = `{#4d8dff-fg}Cocinero{/}  {white-fg}${state.provider.model}{/}  {gray-fg}${state.provider.name}{/}`
    homeMeta.setContent(meta)
    composerMeta.setContent(meta)
    screen.render()
  }

  async function handleCommand(text) {
    if (text === "/exit" || text === "/quit") {
      screen.destroy()
      process.exit(0)
    }

    if (text === "/help") {
      helpOverlay.setContent([
        "{bold}Odrys Help{/}",
        "",
        "Esta vista es temporal y luego la sustituiremos por una ayuda mas rica.",
        "",
        "{cyan-fg}Comandos{/}",
        "/help",
        "/doctor",
        "/scan",
        "/runs",
        "/show <n>",
        "/workspace <ruta>",
        "/provider <name>",
        "/model <id>",
        "/run <objetivo>",
        "/clear",
        "/exit",
        "",
        "{cyan-fg}Navegacion{/}",
        "Esc para cerrar esta ayuda",
        "PgUp/PgDown o j/k para mover el scroll"
      ].join("\n"))
      setMode("help")
      return
    }

    if (text === "/clear") {
      app.transcript = []
      timeline.setContent("")
      render()
      return
    }

    if (text === "/doctor") {
      setMode("session")
      addTranscript(formatDoctor(root, state), "{bold}Doctor{/}")
      return
    }

    if (text === "/scan") {
      const workspace = await resolveWorkspace(root, state.workspace)
      const snapshot = await scanWorkspace(workspace, state.permission)
      setMode("session")
      addTranscript(formatScan(snapshot), "{bold}Workspace{/}")
      return
    }

    if (text === "/runs") {
      const runs = await listRunLogs(root, 8)
      setMode("session")
      addTranscript(formatRunList(runs), "{bold}Runs recientes{/}")
      return
    }

    if (text.startsWith("/show ")) {
      const value = Number(text.slice("/show ".length).trim())
      if (!Number.isInteger(value) || value < 1) {
        setMode("session")
        addTranscript(["Uso: /show <n>"], "{red-fg}Error{/}")
        return
      }
      const runs = await listRunLogs(root, 8)
      const target = runs[value - 1]
      if (!target) {
        setMode("session")
        addTranscript(["No existe ese run reciente."], "{red-fg}Error{/}")
        return
      }
      const run = await readRunLog(target)
      setMode("session")
      addTranscript(formatStoredRun(run, `#${value}`))
      return
    }

    if (text.startsWith("/workspace ")) {
      state.workspace.path = text.slice("/workspace ".length).trim()
      setMode("session")
      addTranscript([`workspace actualizado a ${state.workspace.path}`], "{bold}Workspace{/}")
      render()
      return
    }

    if (text.startsWith("/provider ")) {
      state.provider.name = text.slice("/provider ".length).trim()
      setMode("session")
      addTranscript([`provider actualizado a ${state.provider.name}`], "{bold}Provider{/}")
      render()
      return
    }

    if (text.startsWith("/model ")) {
      state.provider.model = text.slice("/model ".length).trim()
      setMode("session")
      addTranscript([`model actualizado a ${state.provider.model}`], "{bold}Model{/}")
      render()
      return
    }
  }

  async function executeGoal(goal) {
    setMode("session")
    app.busy = true
    if (!app.sessionStartedAt) app.sessionStartedAt = stamp()

    addTranscript([goal], "{#4d8dff-fg}▎{/} {white-fg}Usuario{/}")
    addTranscript([`{#4d8dff-fg}◻{/} {white-fg}Cocinero{/} {gray-fg}· ${state.provider.model} ${state.provider.name}{/}`])
    render()

    try {
      const result = await runWorker({
        root,
        goal,
        config: state
      })
      addTranscript(formatRunResult(result))
    } catch (error) {
      addTranscript([error instanceof Error ? error.message : String(error)], "{red-fg}Error{/}")
    } finally {
      app.busy = false
      render()
    }
  }

  async function submitFrom(box) {
    if (app.busy) return
    const value = box.getValue().trim()
    box.clearValue()
    render()
    if (!value) return

    if (value.startsWith("/")) {
      await handleCommand(value)
      return
    }

    const goal = value.startsWith("/run ") ? value.slice("/run ".length).trim() : value
    await executeGoal(goal)
  }

  homeInput.on("submit", async () => {
    await submitFrom(homeInput)
    homeInput.focus()
    render()
  })

  composerInput.on("submit", async () => {
    await submitFrom(composerInput)
    composerInput.focus()
    render()
  })

  homeInput.key("enter", () => homeInput.submit())
  composerInput.key("enter", () => composerInput.submit())

  screen.key(["C-c"], () => {
    screen.destroy()
    process.exit(0)
  })

  screen.key(["escape"], () => {
    if (app.mode === "help") {
      setMode(app.sessionStartedAt ? "session" : "home")
      return
    }
    if (app.busy) {
      addTranscript(["Interrupcion aun no implementada."], "{yellow-fg}Aviso{/}")
      render()
    }
  })

  setMode("home")
  render()
}
