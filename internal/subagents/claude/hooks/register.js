const agentType = 'agentx:codex'

function key(sessionId, agentId) {
  return `codex:${sessionId}:${agentId}`
}

async function* answer(e, text) {
  yield { kind: 'text', index: 0, text }
  yield { kind: 'stop', stopReason: 'end_turn', usage: null }
  return { turnId: e.turnId, index: e.index, answer: text, toolUses: [], stopReason: 'end_turn', usage: null }
}

export function register(on) {
  on('session.start', async ($, e, next) => {
    const result = await next(e)
    // Register only when the bridge is loaded: a disabled mod must offer no Claude fallback.
    await $.agent.register({
      name: 'codex',
      description: 'Read-only research and code review using Codex through AgentX. Uses Codex native tools and credentials. Resume with SendMessage.',
      prompt: 'AgentX executes this task in Codex. Report an unavailable bridge rather than performing the task yourself.',
      tools: ['Read'],
      model: 'inherit',
    })
    return result
  })

  on('classic.SubagentStart', async ($, e, next) => {
    if (e.agent_type === agentType) {
      const stateKey = key(e.session_id, e.agent_id)
      const previous = await $.store.get(stateKey)
      await $.store.set(stateKey, {
        model: await $.env.get('AX_SUBAGENT_CODEX_MODEL') ?? '',
        delivered: 0,
        ...previous,
        cwd: e.cwd,
      })
    }
    return next(e)
  })

  on('turn.step', async function* ($, e, next) {
    if (!e.agentId) return yield* next(e)
    const stateKey = key(await $.session.id(), e.agentId)
    const state = await $.store.get(stateKey)
    if (!state) {
      const agents = await $.agent.list()
      if (!agents.some(agent => agent.id === e.agentId && agent.type === agentType)) return yield* next(e)
      return yield* answer(e, 'AgentX execution failed: missing subagent context. Start a new agentx:codex subagent.')
    }
    try {
      const messages = await $.session.messages({ agentId: e.agentId })
      if (!Array.isArray(messages)) throw new Error(messages.deny)
      if (messages.length < state.delivered) throw new Error('Subagent transcript was shortened. Start a new agentx:codex subagent.')
      const prompt = messages.slice(state.delivered).filter(message => message.role === 'user' && message.text).map(message => message.text).join('\n\n')
      if (!prompt.trim()) throw new Error('No new task message to deliver to Codex.')
      const { executable } = JSON.parse(await $.fs.read(`${$.plugin.root}/bridge.json`))
      if (!executable) throw new Error('Missing AgentX executable.')
      const argv = [executable, 'agent', 'exec', 'codex', '--cwd', state.cwd]
      if (state.model) argv.push('--model', state.model)
      if (state.threadId) argv.push('--resume', state.threadId)
      const debug = await $.env.get('AX_LOG') === 'debug'
      const process = $.process.spawn({ argv, cwd: state.cwd, input: prompt })
      let pending = ''
      let stderr = ''
      let completed = false
      let failure = ''
      let text = ''
      async function event(line) {
        if (!line.trim()) return
        const item = JSON.parse(line)
        if (item.type === 'thread.started') {
          if (!item.thread_id || (state.threadId && state.threadId !== item.thread_id)) throw new Error('Codex returned an unexpected session ID.')
          state.threadId = item.thread_id
          await $.store.set(stateKey, state)
        } else if (item.type === 'turn.started') {
          if (!state.threadId) throw new Error('Codex started a turn without a session ID.')
          state.delivered = messages.length
          await $.store.set(stateKey, state)
        } else if (item.type === 'item.completed' && item.item?.type === 'agent_message') {
          text = item.item.text
        } else if (item.type === 'turn.completed') {
          completed = true
        } else if (item.type === 'turn.failed') {
          failure = item.error?.message ?? 'Codex turn failed.'
        }
      }
      try {
        let chunk = await process.next()
        while (!chunk.done) {
          if (chunk.value.stream === 'stderr') {
            stderr = (stderr + chunk.value.text).slice(-16384)
            if (debug) await $.ui.log(chunk.value.text, { to: 'debug' })
          } else {
            pending += chunk.value.text
            if (pending.length > 8 * 1024 * 1024) throw new Error('Codex event exceeded the bridge limit.')
            let end
            while ((end = pending.indexOf('\n')) >= 0) {
              await event(pending.slice(0, end))
              pending = pending.slice(end + 1)
            }
          }
          chunk = await process.next()
        }
        await event(pending)
        if (chunk.value.code !== 0) throw new Error(failure || stderr || `Codex exited with ${chunk.value.code ?? chunk.value.signal}.`)
        if (failure) throw new Error(failure)
        if (!completed || !text || !state.threadId) throw new Error('Codex exited without a complete answer.')
      } finally {
        // Ending the native stream also cancels the child on parsing or storage errors.
        await process.return()
      }
      return yield* answer(e, text)
    } catch (error) {
      return yield* answer(e, `AgentX execution failed: ${error.message ?? String(error)}`)
    }
  }).catch(async function* ($, e, next) {
    if (!e.agentId || next.called) return yield* next(e)
    return yield* answer(e, `AgentX bridge failed: ${next.error.message ?? next.error.kind}. No Claude request was made.`)
  })
}
