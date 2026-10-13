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
    expect(e.name).toBe('codex')
    return { value: { agent: 'agentx:codex' } }
  })
  on('classic.SubagentStart', () => ({}))
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
      if (options.malformed) {
        yield { stream: 'stdout', text: '{broken\n' }
        return { value: { code: 0, signal: null } }
      }
      const lines = [
        { type: 'thread.started', thread_id: id },
        { type: 'turn.started' },
        { type: 'item.completed', item: { type: 'agent_message', text: 'External answer' } },
        options.fail ? { type: 'turn.failed', error: { message: 'External failure' } } : { type: 'turn.completed' },
      ].map(value => JSON.stringify(value)).join('\n')
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
