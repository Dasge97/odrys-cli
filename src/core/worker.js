import { buildContext, formatContext, shouldUsePlanner } from "./context-engine.js"
import { validateAgentOutput } from "./contract.js"
import { createProvider } from "../llm/create-provider.js"
import { readAgentPrompt, readPrompt, readProjectState, updateProjectState, writeRunLog } from "./state-manager.js"
import { resolveWorkspace } from "./workspace.js"
import { getAgent } from "./agents.js"
import { applyExecutorOperations } from "./operations.js"

function plannerTaskList(plan) {
  const tasks = []
  for (const phase of plan.phases ?? []) {
    for (const task of phase.tasks ?? []) {
      tasks.push(task.description ?? task.id ?? "tarea sin descripcion")
    }
  }
  return tasks.length > 0 ? tasks : [plan.summary]
}

function reviewHasBlockingErrors(review) {
  return (review.errors ?? []).some((item) => item?.severity === "critical" || item?.severity === "major")
}

async function callAgent({ root, provider, name, goal, task, context, feedback }) {
  const agent = getAgent(name)
  const base = await readPrompt(root, "system/base_system_prompt.md")
  const agentPrompt = await readAgentPrompt(root, name)

  const response = await provider.run({
    agent: name,
    agent_name: agent.name,
    agent_slug: agent.slug,
    systemPrompt: `${base.trim()}\n\n${agentPrompt.trim()}`,
    context,
    goal,
    task,
    feedback
  })

  return validateAgentOutput(name, response)
}

export async function runWorker({ root, goal, config }) {
  const provider = createProvider(config.provider)
  const workspace = await resolveWorkspace(root, config.workspace)
  const run = {
    started_at: new Date().toISOString(),
    goal,
    workspace,
    provider: config.provider,
    steps: []
  }

  const executeContext = formatContext(await buildContext(root, "execute", { workspace, permission: config.permission }))
  const reviewContext = formatContext(await buildContext(root, "review", { workspace, permission: config.permission }))

  let plan = null
  const mustPlan = config.worker.planner.auto && shouldUsePlanner(goal)

  if (mustPlan) {
    plan = await callAgent({
      root,
      provider,
      name: "planner",
      goal,
      task: goal,
      context: executeContext
    })
    run.steps.push({ agent: getAgent("planner").slug, role: "planner", output: plan })
  }

  const tasks = plan ? plannerTaskList(plan) : [goal]
  const taskResults = []

  for (const task of tasks) {
    let executorOutput = null
    let reviewerOutput = null
    let feedback = null
    let appliedOperations = []

    for (let attempt = 0; attempt <= config.worker.maxReviewLoops; attempt += 1) {
      executorOutput = await callAgent({
        root,
        provider,
        name: "executor",
        goal,
        task,
        context: executeContext,
        feedback
      })
      appliedOperations = await applyExecutorOperations(workspace, config.permission, executorOutput.operations)
      run.steps.push({ agent: getAgent("executor").slug, role: "executor", task, attempt, output: executorOutput })
      run.steps.push({ agent: "workspace", role: "operations", task, attempt, output: appliedOperations })

      reviewerOutput = await callAgent({
        root,
        provider,
        name: "reviewer",
        goal,
        task,
        context: reviewContext,
        feedback: {
          executor: executorOutput,
          applied_operations: appliedOperations
        }
      })
      run.steps.push({ agent: getAgent("reviewer").slug, role: "reviewer", task, attempt, output: reviewerOutput })

      if (reviewerOutput.status === "approved" && !reviewHasBlockingErrors(reviewerOutput)) {
        break
      }

      feedback = {
        executor: executorOutput,
        reviewer: reviewerOutput,
        applied_operations: appliedOperations
      }
    }

    taskResults.push({
      task,
      executor: executorOutput,
      applied_operations: appliedOperations,
      reviewer: reviewerOutput
    })
  }

  let summary = null
  if (config.worker.summarizeOnSuccess) {
    const currentState = await readProjectState(root)
    summary = await callAgent({
      root,
      provider,
      name: "summarizer",
      goal,
      task: "actualizar estado del proyecto",
      context: `${executeContext}\n\n## current_project_state\n\n${currentState}`,
      feedback: taskResults
    })
    run.steps.push({ agent: getAgent("summarizer").slug, role: "summarizer", output: summary })
    await updateProjectState(root, summary.project_state)
  }

  run.completed_at = new Date().toISOString()
  run.result = {
    plan,
    tasks: taskResults,
    summary
  }

  const logPath = await writeRunLog(root, run)

  return {
    status: "completed",
    goal,
    planned: mustPlan,
    tasks: taskResults,
    summary,
    log_path: logPath
  }
}
