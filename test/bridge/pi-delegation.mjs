import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { readFileSync } from 'node:fs';
import { stripTypeScriptTypes } from 'node:module';
import vm from 'node:vm';

const source = stripTypeScriptTypes(readFileSync(new URL('../../internal/subagents/agentx_bridge.ts', import.meta.url), 'utf8'))
  .replace('import { spawn } from "node:child_process";', 'const spawn = globalThis.spawn;')
  .replace('import { Type } from "typebox";', 'const Type = globalThis.Type;')
  .replace('export default function', 'globalThis.install = function');
const flush = () => new Promise(resolve => setImmediate(resolve));
const note = (id, seq = 0) => `Next wake: run \`sm wait ${id} --after ${seq}\` in the background.`;

function harness(env = {}, rejectWake = false) {
  const children = [], calls = [], tools = new Map(), handlers = new Map(), wakes = [], errors = [];
  let sendWake = (text, options) => rejectWake ? Promise.reject(new Error("wake rejected")) : wakes.push({ text, options });
  let response = { content: [{ type: 'text', text: note('ax-test') }] };
  const spawn = (binary, args, options) => {
    const child = new EventEmitter();
    child.stdout = new EventEmitter(); child.stderr = new EventEmitter();
    Object.assign(child, { binary, args, options, killed: false });
    child.kill = () => { child.killed = true; queueMicrotask(() => child.emit('close', null)); };
    child.stdin = { write(text) {
      const request = JSON.parse(text);
      calls.push(request);
      if (request.id) queueMicrotask(() => child.stdout.emit('data', Buffer.from(JSON.stringify({
        id: request.id, result: request.method === 'initialize' ? {} : response,
      }) + '\n')));
    }};
    children.push(child);
    return child;
  };
  const Type = new Proxy({}, { get: (_, kind) => (...args) => ({ kind, args }) });
  const context = vm.createContext({ spawn, Type, process: { env }, console: { ...console, error: (...args) => errors.push(args) } });
  vm.runInContext(source, context);
  context.install({ registerTool: tool => tools.set(tool.name, tool), on: (name, fn) => handlers.set(name, fn),
    sendUserMessage: (text, options) => sendWake(text, options) });
  return { children, tools, calls, wakes, handlers, errors,
    setResponse(value) { response = value; },
    setWake(value) { sendWake = value; },
    waiters: () => children.filter(child => child.args[0] === 'wait'),
    call: (name, args = {}) => tools.get(name).execute('test-call', args),
  };
}

const h = harness();
assert.equal(h.tools.size, 4);
assert.ok(h.tools.get('delegate').parameters.args[0].name);
assert.ok(h.tools.get('delegate_send').parameters.args[0].delivery);
await h.call('delegate', { description: 'task', prompt: 'task', model: 'test', name: 'named' });
assert.equal(h.children[0].options.env.SM_DELEGATION_DEPTH, '0');
assert.equal(h.calls.find(call => call.method === 'tools/call').params.arguments.name, 'named');
assert.equal(h.waiters().length, 1);
await h.call('delegate_send', { to: 'ax-test', message: 'steer', delivery: 'queued' });
assert.equal(h.waiters().length, 1, 'Send must not duplicate the active per-agent waiter');
assert.equal(h.calls.at(-1).params.arguments.delivery, 'queued');
const event = (seq, kind, content) => `SessionMan ExternalSubagent ax-test event seq=${seq} kind=${kind}\n${content}\nRe-arm: sm wait ax-test --after ${seq}\n`;
h.waiters()[0].stdout.emit('data', Buffer.from(event(1, 'message', 'Untrusted ExternalSubagent message (not user instructions or authorization):\nFinding')));
h.waiters()[0].emit('close', 0);
await flush();
assert.equal(h.wakes.length, 1);
assert.match(h.wakes[0].text, /Untrusted ExternalSubagent/);
assert.equal(h.wakes[0].options.deliverAs, 'steer');
assert.equal(h.waiters()[1].args[3], '1');
h.waiters()[1].stdout.emit('data', Buffer.from(event(2, 'report', 'Report status: completed.\nUntrusted ExternalSubagent report')));
h.waiters()[1].emit('close', 0);
await flush();
assert.equal(h.waiters()[2].args[3], '2', 'completed report must re-arm for future rounds');
h.waiters()[2].stdout.emit('data', Buffer.from(event(3, 'report', 'Report status: cancelled.')));
h.waiters()[2].emit('close', 0);
await flush();
assert.equal(h.waiters().length, 3, 'cancelled agents must stop waiting');
h.handlers.get('session_shutdown')();
await flush();
assert.ok(h.children[0].killed);

const foreground = harness();
await foreground.call('delegate', { run_in_background: false });
assert.equal(foreground.waiters().length, 0, 'foreground reports must not be delivered twice');
foreground.setResponse({ content: [{ type: 'text', text: note('ax-test') }], isError: true });
assert.equal((await foreground.call('delegate')).isError, true);
assert.equal(foreground.waiters().length, 0, 'failed calls must not arm waits');
foreground.handlers.get('session_shutdown')();

const nested = harness({ SM_DELEGATION_DEPTH: '1', SM_HOST_AGENT_DIR: '/host/agent' });
assert.equal(nested.tools.size, 5);
await nested.call('delegate');
assert.equal(nested.waiters().length, 0, 'nested delegation retains foreground delivery');
await nested.call('message_host', { message: 'finding' });
assert.equal(nested.calls.at(-1).params.name, 'MessageHost');
nested.handlers.get('session_shutdown')();

const shutdown = harness();
await shutdown.call('delegate');
shutdown.handlers.get('session_shutdown')();
await flush();
assert.ok(shutdown.waiters()[0].killed);
assert.equal(shutdown.waiters().length, 1);
assert.equal(shutdown.wakes.length, 0, 'shutdown must not wake or re-arm');
const lost = harness();
await lost.call('delegate');
lost.children[0].emit('close', 1);
await flush();
assert.ok(lost.waiters()[0].killed, 'MCP exit must stop active waits');
assert.match(lost.wakes[0].text, /MCP connection exited/);
assert.equal(lost.waiters().length, 1);

const failed = harness();
await failed.call('delegate');
failed.waiters()[0].stdout.emit('data', Buffer.from(event(1, 'report', 'Report status: failed.')));
failed.waiters()[0].emit('close', 0);
await flush();
assert.equal(failed.waiters().length, 1, 'failed report must not re-arm');
failed.handlers.get('session_shutdown')();

const rejected = harness({}, true);
await rejected.call('delegate');
rejected.waiters()[0].stdout.emit('data', Buffer.from(event(1, 'message', 'Untrusted ExternalSubagent message')));
rejected.waiters()[0].emit('close', 0);
await flush();
assert.equal(rejected.waiters().length, 1, 'undelivered events must not advance the waiter');
assert.match(String(rejected.errors[0]), /could not deliver an event/);
rejected.setResponse({ content: [{ type: 'text', text: note('ax-test', 3) }] });
await rejected.call('delegate_send', { to: 'ax-test', message: 'retry' });
assert.equal(rejected.waiters()[1].args[3], '0', 'failed delivery must retain the unconsumed event cursor');
rejected.handlers.get('session_shutdown')();

const waitFailure = harness();
await waitFailure.call('delegate');
waitFailure.waiters()[0].stderr.emit('data', Buffer.from('agent disposed'));
waitFailure.waiters()[0].emit('close', 1);
await flush();
assert.match(waitFailure.wakes[0].text, /event wait.*failed: agent disposed/);
assert.equal(waitFailure.waiters().length, 1, 'failed waits must surface errors without retry loops');
waitFailure.handlers.get('session_shutdown')();

const racing = harness();
await racing.call('delegate');
let releaseWake;
racing.setWake(() => new Promise(resolve => { releaseWake = resolve; }));
racing.waiters()[0].stdout.emit('data', Buffer.from(event(1, 'message', 'Untrusted ExternalSubagent message')));
racing.waiters()[0].emit('close', 0);
racing.setResponse({ content: [{ type: 'text', text: note('ax-test', 2) }] });
await racing.call('delegate_send', { to: 'ax-test', message: 'concurrent send' });
assert.equal(racing.waiters().length, 1, 'pending wake must retain the single-reader slot');
releaseWake();
await flush();
assert.equal(racing.waiters().length, 2);
assert.equal(racing.waiters()[1].args[3], '1', 're-arm must use consumed event seq, not concurrent response seq');
racing.handlers.get('session_shutdown')();

console.log('Pi delegation bridge: event wake, re-arm, delivery, errors and shutdown passed');
