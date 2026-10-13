const results = new Map()
const waiting = new Map()
let finished
let latestId
function resultOf(id) {
  if (results.has(id)) return Promise.resolve(results.get(id))
  return new Promise(resolve => waiting.set(id, resolve))
}
async function* answer(e, text) {
  yield { kind: 'text', index: 0, text }
  yield { kind: 'stop', stopReason: 'end_turn', usage: null }
  return { turnId: e.turnId, index: e.index, answer: text, toolUses: [], stopReason: 'end_turn', usage: null }
}
export function register(on) {
  on('classic.SubagentStart', async ($, e, next) => {
    if (e.agent_type === 'agentx:codex') latestId = e.agent_id
    return next(e)
  })
  on('turn.complete', async ($, e, next) => {
    if (e.agentId) {
      results.set(e.agentId, e.answer)
      waiting.get(e.agentId)?.(e.answer)
      waiting.delete(e.agentId)
    }
    return next(e)
  })
  on('turn.step', async function* ($, e, next) {
    if (e.agentId) return yield* next(e)
    if (finished) return yield* answer(e, finished)
    try {
      const { agentId, deny } = await $.agent.spawn({ subagentType: 'agentx:codex', prompt: 'Remember the token AX_NATIVE_731. Reply with only AX_FIRST_OK.', description: 'AgentX smoke' })
      if (deny || !agentId) throw new Error(deny ?? 'Missing native agent ID')
      const first = await resultOf(agentId)
      if (!first.includes('AX_FIRST_OK')) throw new Error(`First task: ${first}`)
      results.delete(agentId)
      const sent = await $.session.send({ to: { agentId }, text: 'Return only the token I asked you to remember.' })
      if (!sent.isDelivered) throw new Error(sent.reason)
      const second = await resultOf(agentId)
      if (!second.includes('AX_NATIVE_731')) throw new Error(`Resume: ${second}`)
      const pidFile = await $.env.get('AX_SUBAGENT_SMOKE_PID_FILE')
      let cancelled
      if (pidFile) {
        const spawning = $.agent.spawn({ subagentType: 'agentx:codex', prompt: 'AX_CANCEL', description: 'AgentX cancellation smoke' }).catch(error => ({ error: error.message }))
        let ready = false
        for (let attempt = 0; attempt < 100; attempt++) {
          if (await $.fs.exists(pidFile)) { ready = true; break }
          await $.clock.sleep(50)
        }
        if (!ready) throw new Error('Cancellation child did not start')
        cancelled = latestId
        await $.tool.call({ tool: 'TaskStop', task_id: cancelled })
        await spawning
      }
      const agents = await $.agent.list()
      if (cancelled && !agents.some(agent => agent.id === cancelled && agent.status === 'killed')) throw new Error('Cancelled child is not marked killed')
      finished = JSON.stringify({ status: 'AX_SMOKE_OK', agentId, first, second, agents })
      return yield* answer(e, finished)
    } catch (error) {
      finished = `AX_SMOKE_FAILED: ${error.message}`
      return yield* answer(e, finished)
    }
  })
  // If the bridge passes a child request through, fail without making any model request.
  on('turn.step', { agentId: /.+/ }, async function* ($, e) {
    return yield* answer(e, 'AX_UNEXPECTED_CLAUDE_INFERENCE')
  })
}
