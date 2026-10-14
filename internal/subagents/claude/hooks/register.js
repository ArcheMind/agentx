const backends = ['codex', 'pi', 'gemini']
const prefix = 'agentx:'

function key(sessionId, agentId) {
  return `external:${sessionId}:${agentId}`
}

function definitionKey(sessionId, name = '') {
  return `definition:${sessionId}:${name}`
}

let registration = Promise.resolve()

async function define($, sessionId, definition) {
  const previous = registration
  let release
  registration = new Promise(resolve => { release = resolve })
  await previous
  try {
    const storageKey = definitionKey(sessionId, definition.name)
    const existing = await $.store.get(storageKey)
    if (existing && JSON.stringify(existing) !== JSON.stringify(definition)) throw new Error('This role name is already bound. Choose a new name for a different configuration.')
    await $.store.set(storageKey, definition)
    await registerAgent($, definition)
  } finally { release() }
}

async function* answer(e, text) {
  yield { kind: 'text', index: 0, text }
  yield { kind: 'stop', stopReason: 'end_turn', usage: null }
  return { turnId: e.turnId, index: e.index, answer: text, toolUses: [], stopReason: 'end_turn', usage: null }
}

async function registerAgent($, definition) {
  return $.agent.register({
    name: definition.name,
    description: `${definition.description} Executes read-only with ${definition.agent}${definition.model ? ` / ${definition.model}` : ''} through AgentX. Resume with SendMessage.`,
    prompt: 'AgentX executes this task externally. Report an unavailable bridge rather than performing the task yourself.',
    tools: ['Read'],
    model: 'inherit',
  })
}

async function command($, args) {
  const { executable } = JSON.parse(await $.fs.read(`${$.plugin.root}/bridge.json`))
  if (!executable) throw new Error('Missing AgentX executable.')
  const debug = await $.env.get('AX_LOG') === 'debug'
  const process = $.process.spawn({ argv: [executable, ...args] })
  let stdout = ''
  let stderr = ''
  try {
    let chunk = await process.next()
    while (!chunk.done) {
      if (debug) await $.ui.log(chunk.value.text, { to: 'debug' })
      if (chunk.value.stream === 'stderr') stderr = (stderr + chunk.value.text).slice(-16384)
      else stdout += chunk.value.text
      if (stdout.length > 8 * 1024 * 1024) throw new Error('AgentX discovery output exceeded the bridge limit.')
      chunk = await process.next()
    }
    if (chunk.value.code !== 0) throw new Error(stderr || `AgentX exited with ${chunk.value.code ?? chunk.value.signal}.`)
    return JSON.parse(stdout)
  } finally {
    await process.return()
  }
}

export function register(on) {
  on('session.start', async ($, e, next) => {
    const result = await next(e)
    const sessionId = await $.session.id()
    const storageKey = definitionKey(sessionId, 'codex')
    const defaultAgent = await $.store.get(storageKey) ?? { name: 'codex', agent: 'codex', model: await $.env.get('AX_SUBAGENT_CODEX_MODEL') ?? '', description: 'Research and code review with Codex.', instructions: '' }
    await $.store.set(storageKey, defaultAgent)
    // Register only when the bridge is loaded: a disabled mod must offer no Claude fallback.
    for (const storedKey of await $.store.keys()) {
      if (storedKey.startsWith(definitionKey(sessionId))) await registerAgent($, await $.store.get(storedKey))
    }
    await $.tool.register({
      name: 'discover',
      description: 'List installed agents and external subagent support, or list native models for one agent. Model discovery does not run inference. Claude decides which agents/models to combine using native Agent calls.',
      inputSchema: { type: 'object', properties: { agent: { type: 'string' } }, additionalProperties: false },
      isDeferred: false,
    })
    await $.tool.register({
      name: 'define_agent',
      description: 'Register a read-only native Claude subagent backed by an external agent/model. Then use the native Agent tool with the returned subagent_type; use SendMessage to resume and TaskStop to cancel. Define any number of roles and combine them with native tools. This registration makes no inference call. External tools and credentials are native to the selected backend; Claude tool policies are not forwarded.',
      inputSchema: {
        type: 'object',
        properties: {
          name: { type: 'string', pattern: '^[a-zA-Z0-9_-]{1,64}$' },
          agent: { type: 'string', enum: backends },
          model: { type: 'string', description: 'Exact native model ID, or omit to use the native default.' },
          description: { type: 'string', description: 'When Claude should delegate to this role.' },
          instructions: { type: 'string', description: 'Role instructions delivered with the first external task. Not a replacement for the backend system prompt.' },
        },
        required: ['name', 'agent', 'description'],
        additionalProperties: false,
      },
      isDeferred: false,
    })
    return result
  })

  on('tool.call', { tool: /^mcp__agentx__(discover|define_agent)$/ }, async ($, e) => {
    try {
      const input = e
      let result
      if (e.tool === 'mcp__agentx__discover') {
        if (input.agent) result = { agent: input.agent, models: await command($, ['--json', 'agent', 'models', input.agent]) }
        else result = (await command($, ['--json', 'agent', 'list'])).map(agent => ({ ...agent, subagent: backends.includes(agent.id), execution: backends.includes(agent.id) ? 'read-only; native resume and cancellation' : agent.id === 'claude' ? 'Use Claude native subagents directly.' : 'No execution adapter; available for ordinary AgentX launch only.' }))
      } else {
        if (!/^[a-zA-Z0-9_-]{1,64}$/.test(input.name ?? '') || !backends.includes(input.agent) || typeof input.description !== 'string' || !input.description.trim()) throw new Error('A valid name, supported agent and nonempty description are required.')
        if (input.name === 'codex') throw new Error('The name codex is reserved. Choose a role name.')
        for (const field of ['model', 'instructions']) if (input[field] !== undefined && typeof input[field] !== 'string') throw new Error(`${field} must be a string.`)
        const definition = { name: input.name, agent: input.agent, model: input.model ?? '', description: input.description, instructions: input.instructions ?? '' }
        await define($, await $.session.id(), definition)
        result = { subagent_type: `${prefix}${input.name}`, agent: definition.agent, model: definition.model || 'native default', access: 'read-only' }
      }
      return { result: JSON.stringify(result) }
    } catch (error) {
      return { deny: `AgentX: ${error.message ?? String(error)}` }
    }
  })

  on('classic.SubagentStart', async ($, e, next) => {
    if (e.agent_type.startsWith(prefix)) {
      const definition = await $.store.get(definitionKey(e.session_id, e.agent_type.slice(prefix.length)))
      const stateKey = key(e.session_id, e.agent_id)
      const previous = await $.store.get(stateKey)
      if (previous || definition) await $.store.set(stateKey, { ...definition, delivered: 0, ...previous, cwd: e.cwd })
    }
    return next(e)
  })

  on('turn.step', async function* ($, e, next) {
    if (!e.agentId) return yield* next(e)
    const stateKey = key(await $.session.id(), e.agentId)
    const state = await $.store.get(stateKey)
    if (!state) {
      const agents = await $.agent.list()
      if (!agents.some(agent => agent.id === e.agentId && agent.type.startsWith(prefix))) return yield* next(e)
      return yield* answer(e, 'AgentX execution failed: missing subagent context. Start a new AgentX subagent.')
    }
    try {
      const messages = await $.session.messages({ agentId: e.agentId })
      if (!Array.isArray(messages)) throw new Error(messages.deny)
      if (messages.length < state.delivered) throw new Error('Subagent transcript was shortened. Start a new AgentX subagent.')
      let prompt = messages.slice(state.delivered).filter(message => message.role === 'user' && message.text).map(message => message.text).join('\n\n')
      if (!prompt.trim()) throw new Error('No new task message to deliver.')
      if (!state.delivered && state.instructions) prompt = `${state.instructions}\n\nTask:\n${prompt}`
      const { executable } = JSON.parse(await $.fs.read(`${$.plugin.root}/bridge.json`))
      if (!executable) throw new Error('Missing AgentX executable.')
      const argv = [executable, 'agent', 'exec', state.agent, '--cwd', state.cwd]
      if (state.model) argv.push('--model', state.model)
      if (state.threadId) argv.push('--resume', state.threadId)
      const debug = await $.env.get('AX_LOG') === 'debug'
      const process = $.process.spawn({ argv, cwd: state.cwd, input: prompt })
      let pending = ''
      let stderr = ''
      let completed = false
      let failure = ''
      let text = ''
      async function session(id) {
        if (typeof id !== 'string' || !/^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$/.test(id) || (state.threadId && state.threadId !== id)) throw new Error('External agent returned an unexpected session ID.')
        state.threadId = id
        await $.store.set(stateKey, state)
      }
      async function submitted() {
        if (!state.threadId) throw new Error('External agent started a turn without a session ID.')
        state.delivered = messages.length
        await $.store.set(stateKey, state)
      }
      async function event(line) {
        if (!line.trim()) return
        const item = JSON.parse(line)
        if (state.agent === 'codex') {
          if (item.type === 'thread.started') await session(item.thread_id)
          else if (item.type === 'turn.started') await submitted()
          else if (item.type === 'item.completed' && item.item?.type === 'agent_message') text = item.item.text
          else if (item.type === 'turn.completed') completed = true
          else if (item.type === 'turn.failed') failure = item.error?.message ?? 'Codex turn failed.'
        } else if (state.agent === 'pi') {
          if (item.type === 'session') await session(item.id)
          else if (item.type === 'agent_start') await submitted()
          else if (item.type === 'message_end' && item.message?.role === 'assistant') {
            failure = ['error', 'aborted'].includes(item.message.stopReason) ? item.message.errorMessage || `Pi request ${item.message.stopReason}.` : ''
            text = item.message.content.filter(part => part.type === 'text').map(part => part.text).join('\n')
          } else if (item.type === 'agent_end') completed = true
        } else if (state.agent === 'gemini') {
          if (item.type === 'init') await session(item.session_id)
          else if (item.type === 'message' && item.role === 'user') await submitted()
          else if (item.type === 'message' && item.role === 'assistant') text = item.delta ? text + item.content : item.content
          else if (item.type === 'result') {
            completed = item.status === 'success'
            if (!completed) failure = item.error?.message || 'Gemini request failed.'
          } else if (item.type === 'error' && item.severity === 'error') failure = item.message
        } else throw new Error('Unsupported external agent.')
      }
      try {
        let chunk = await process.next()
        while (!chunk.done) {
          if (debug) await $.ui.log(chunk.value.text, { to: 'debug' })
          if (chunk.value.stream === 'stderr') stderr = (stderr + chunk.value.text).slice(-16384)
          else {
            pending += chunk.value.text
            if (pending.length > 8 * 1024 * 1024) throw new Error('External event exceeded the bridge limit.')
            let end
            while ((end = pending.indexOf('\n')) >= 0) {
              await event(pending.slice(0, end))
              pending = pending.slice(end + 1)
            }
          }
          chunk = await process.next()
        }
        await event(pending)
        if (chunk.value.code !== 0) throw new Error(failure || stderr || `External agent exited with ${chunk.value.code ?? chunk.value.signal}.`)
        if (failure) throw new Error(failure)
        if (!completed || !text || !state.threadId) throw new Error('External agent exited without a complete answer.')
        await submitted()
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
