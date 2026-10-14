import { expect, test } from 'claude-code/testing'

const id = '0199a213-81c0-7800-8aa1-bbab2a035a53'
const input = { turnId: 'turn', index: 0, model: 'claude-test', messageCount: 1, agentId: 'child' }

async function drain(stream) {
  let part = await stream.next()
  const chunks = []
  while (!part.done) {
    chunks.push(part.value)
    part = await stream.next()
  }
  return { result: part.value, chunks }
}

function setup(on, options = {}) {
  const saved = new Map()
  const requests = []
  let modelCalls = 0
  let messages = [{ role: 'user', text: 'Inspect the repository', toolUses: [] }]
  let closed = 0
  on('session.start', () => ({ cwd: '/project' }))
  on('agent.register', ($, e) => {
    return { value: { agent: `agentx:${e.name}` } }
  })
  on('classic.SubagentStart', () => ({}))
  on('store.keys', () => ({ value: [...saved.keys()] }))
  on('tool.register', ($, e) => ({ value: { tool: `mcp__agentx__${e.name}` } }))
  on('store.get', ($, e) => ({ value: saved.get(e.key) }))
  on('store.set', ($, e) => { saved.set(e.key, e.value); return { value: undefined } })
  on('env.get', () => ({ value: 'external-model' }))
  on('session.id', () => ({ value: 'session' }))
  on('session.messages', ($, e) => {
    expect(e.agentId).toBe('child')
    return { value: options.deny ? { deny: 'No transcript' } : messages }
  })
  on('agent.list', () => ({ value: [{ id: 'child', type: options.unrelated ? 'Explore' : 'agentx:codex' }] }))
  on('fs.read', () => ({ value: '{"executable":"/bin/ax"}' }))
  on('process.spawn', async function* ($, e) {
    requests.push(e)
    try {
      if (options.discovery) {
        yield { stream: 'stdout', text: JSON.stringify(options.discovery) }
        return { value: { code: 0, signal: null } }
      }
      if (options.malformed) {
        yield { stream: 'stdout', text: '{broken\n' }
        return { value: { code: 0, signal: null } }
      }
      const lines = (options.events ?? [
        { type: 'thread.started', thread_id: id },
        { type: 'turn.started' },
        { type: 'item.completed', item: { type: 'agent_message', text: 'External answer' } },
        options.fail ? { type: 'turn.failed', error: { message: 'External failure' } } : { type: 'turn.completed' },
      ]).map(value => JSON.stringify(value)).join('\n')
      // Pipe boundaries are not event boundaries, and the last event has no newline.
      yield { stream: 'stdout', text: lines.slice(0, 13) }
      yield { stream: 'stderr', text: 'diagnostic' }
      yield { stream: 'stdout', text: lines.slice(13) }
      return { value: { code: options.fail ? 1 : 0, signal: null } }
    } finally { closed++ }
  })
  on('turn.step', async function* ($, e) {
    modelCalls++
    yield { kind: 'text', index: 0, text: 'Native answer' }
    return { turnId: e.turnId, index: e.index, answer: 'Native answer', toolUses: [], stopReason: 'end_turn', usage: null }
  })
  return { saved, requests, modelCalls: () => modelCalls, closed: () => closed, setMessages: value => { messages = value } }
}

async function start($) {
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/project' })
  await $.classic.SubagentStart({ agent_id: 'child', agent_type: 'agentx:codex', session_id: 'session', cwd: '/isolated-worktree' })
}

test('native child executes externally, uses child cwd, and resumes its own thread', async ($, on) => {
  const fixture = setup(on)
  await start($)
  const first = await drain($.turn.step(input))
  expect(first.result.answer).toBe('External answer')
  expect(fixture.modelCalls()).toBe(0)
  expect(fixture.requests[0].cwd).toBe('/isolated-worktree')
  expect(fixture.requests[0].argv).toEqual(['/bin/ax', 'agent', 'exec', 'codex', '--cwd', '/isolated-worktree', '--model', 'external-model'])
  fixture.setMessages([
    { role: 'user', text: 'Inspect the repository', toolUses: [] },
    { role: 'assistant', text: 'External answer', toolUses: [] },
    { role: 'user', text: 'Continue with authentication', toolUses: [] },
  ])
  await drain($.turn.step({ ...input, turnId: 'second' }))
  expect(fixture.requests[1].argv.slice(-2)).toEqual(['--resume', id])
  expect(fixture.requests[1].input).toBe('Continue with authentication')
  expect(fixture.modelCalls()).toBe(0)
  expect(fixture.closed()).toBe(2)
})

test('main and unrelated agents retain native inference', async ($, on) => {
  const fixture = setup(on, { unrelated: true })
  await drain($.turn.step({ ...input, agentId: undefined }))
  await drain($.turn.step(input))
  expect(fixture.modelCalls()).toBe(2)
  expect(fixture.requests.length).toBe(0)
})

for (const [name, options, expected] of [
  ['provider failure', { fail: true }, 'External failure'],
  ['malformed events', { malformed: true }, 'AgentX execution failed'],
  ['unreadable child context', { deny: true }, 'No transcript'],
]) {
  test(`${name} never falls back to Claude`, async ($, on) => {
    const fixture = setup(on, options)
    await start($)
    const { result } = await drain($.turn.step(input))
    expect(result.answer).toContain(expected)
    expect(fixture.modelCalls()).toBe(0)
    if (!options.deny) expect(fixture.closed()).toBe(1)
  })
}

test('missing bridge state does not send an external agent to Claude', async ($, on) => {
  const fixture = setup(on)
  const { result } = await drain($.turn.step(input))
  expect(result.answer).toContain('missing subagent context')
  expect(fixture.modelCalls()).toBe(0)
})

async function defineRole($, agent, extra = {}) {
  return $.tool.call({ tool: 'mcp__agentx__define_agent', name: 'reviewer', agent, model: 'chosen-model', description: 'Review changes', instructions: 'Check correctness first.', ...extra })
}

for (const [agent, events] of [
  ['codex', undefined],
  ['pi', [
    { type: 'session', id }, { type: 'agent_start' },
    { type: 'message_end', message: { role: 'assistant', stopReason: 'stop', content: [{ type: 'text', text: 'External answer' }] } },
    { type: 'agent_end' },
  ]],
  ['gemini', [
    { type: 'init', session_id: id }, { type: 'message', role: 'user', content: 'task' },
    { type: 'message', role: 'assistant', content: 'External ', delta: true },
    { type: 'message', role: 'assistant', content: 'answer', delta: true },
    { type: 'result', status: 'success' },
  ]],
]) {
  test(`${agent} dynamic role binds model and instructions and resumes without repeating them`, async ($, on) => {
    const fixture = setup(on, { events })
    await start($)
    const registered = await defineRole($, agent)
    expect(registered.deny).toBeUndefined()
    expect(fixture.requests.length).toBe(0)
    await $.classic.SubagentStart({ agent_id: 'child', agent_type: 'agentx:reviewer', session_id: 'session', cwd: '/worktree' })
    // A different native child gets the new binding; the original child's binding is frozen.
    fixture.saved.delete('external:session:child')
    await $.classic.SubagentStart({ agent_id: 'child', agent_type: 'agentx:reviewer', session_id: 'session', cwd: '/worktree' })
    const first = await drain($.turn.step(input))
    expect(first.result.answer).toBe('External answer')
    expect(fixture.requests[0].argv).toEqual(['/bin/ax', 'agent', 'exec', agent, '--cwd', '/worktree', '--model', 'chosen-model'])
    expect(fixture.requests[0].input).toBe('Check correctness first.\n\nTask:\nInspect the repository')
    fixture.setMessages([{ role: 'user', text: 'Inspect the repository' }, { role: 'assistant', text: 'External answer' }, { role: 'user', text: 'Follow up' }])
    await drain($.turn.step({ ...input, turnId: 'followup' }))
    expect(fixture.requests[1].input).toBe('Follow up')
    expect(fixture.requests[1].argv.slice(-2)).toEqual(['--resume', id])
    expect(fixture.modelCalls()).toBe(0)
  })
}

test('role definitions survive session start, reject rebinding and allow parallel distinct names', async ($, on) => {
  const fixture = setup(on)
  await start($)
  const definitions = await Promise.all([defineRole($, 'pi'), defineRole($, 'codex', { name: 'second' })])
  expect(definitions.every(result => !result.deny)).toBe(true)
  const rebound = await defineRole($, 'gemini')
  expect(rebound.deny).toContain('already bound')
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/project' })
  expect(fixture.saved.get('definition:session:reviewer').agent).toBe('pi')
  expect(fixture.saved.get('definition:session:second').agent).toBe('codex')
  expect(fixture.requests.length).toBe(0)
})

for (const [name, events, expected] of [
  ['pi', [{ type: 'session', id }, { type: 'agent_start' }, { type: 'message_end', message: { role: 'assistant', stopReason: 'error', errorMessage: 'Pi provider refused', content: [] } }, { type: 'agent_end' }], 'Pi provider refused'],
  ['gemini', [{ type: 'init', session_id: id }, { type: 'message', role: 'user' }, { type: 'result', status: 'error', error: { message: 'Gemini provider refused' } }], 'Gemini provider refused'],
]) {
  test(`${name} reports native errors even when CLI exits zero`, async ($, on) => {
    const fixture = setup(on, { events })
    await start($)
    await defineRole($, name)
    fixture.saved.delete('external:session:child')
    await $.classic.SubagentStart({ agent_id: 'child', agent_type: 'agentx:reviewer', session_id: 'session', cwd: '/worktree' })
    expect((await drain($.turn.step(input))).result.answer).toContain(expected)
    expect(fixture.modelCalls()).toBe(0)
  })
}


test('discovery reuses native model catalog without inference', async ($, on) => {
  const fixture = setup(on, { discovery: [{ id: 'provider/model', source: 'pi' }] })
  await start($)
  const result = await $.tool.call({ tool: 'mcp__agentx__discover', agent: 'pi' })
  expect(JSON.parse(result.result).models[0].id).toBe('provider/model')
  expect(fixture.requests[0].argv).toEqual(['/bin/ax', '--json', 'agent', 'models', 'pi'])
  expect(fixture.modelCalls()).toBe(0)
})

test('agent discovery distinguishes launch support from subagent support', async ($, on) => {
  setup(on, { discovery: [{ id: 'pi', installed: true }, { id: 'dsh', installed: true }, { id: 'claude', installed: true }] })
  await start($)
  const result = JSON.parse((await $.tool.call({ tool: 'mcp__agentx__discover' })).result)
  expect(result[0].subagent).toBe(true)
  expect(result[1].subagent).toBe(false)
  expect(result[2].execution).toContain('native subagents directly')
})
