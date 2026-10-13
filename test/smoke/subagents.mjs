import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join, delimiter } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';

const root = mkdtempSync(join(tmpdir(), 'agentx-subagents-'));
const ax = resolve(process.argv[2] || 'bin/ax');
const live = process.argv.includes('--live');
const bin = join(root, 'bin');
mkdirSync(bin);
if (!live) {
  writeFileSync(join(bin, 'codex'), `#!/usr/bin/env python3
import json, sys, os, subprocess, time
prompt = sys.stdin.read()
args = sys.argv[1:]
resume = 'resume' in args
if prompt.strip() == 'AX_CANCEL':
    child = subprocess.Popen(['sleep', '60'])
    with open(os.environ['AX_SUBAGENT_SMOKE_PID_FILE'], 'w') as file:
        json.dump([os.getpid(), child.pid], file)
    print(json.dumps({'type':'thread.started','thread_id':'0199a213-81c0-7800-8aa1-bbab2a035a53'}), flush=True)
    print(json.dumps({'type':'turn.started'}), flush=True)
    time.sleep(60)
    sys.exit(0)
assert 'sandbox_mode="read-only"' in args
assert 'approval_policy="never"' in args
if resume:
    assert '0199a213-81c0-7800-8aa1-bbab2a035a53' in args
    assert 'Return only the token' in prompt
    assert 'Remember the token' not in prompt
else:
    assert 'Remember the token' in prompt
for event in [
    {'type':'thread.started','thread_id':'0199a213-81c0-7800-8aa1-bbab2a035a53'},
    {'type':'turn.started'},
    {'type':'item.completed','item':{'type':'agent_message','text':'AX_NATIVE_731' if resume else 'AX_FIRST_OK'}},
    {'type':'turn.completed','usage':{'input_tokens':0,'output_tokens':0}},
]:
    print(json.dumps(event), flush=True)
`, { mode: 0o700 });
}
for (const plugin of ['internal/subagents/claude', 'test/smoke/fixtures/agentx-subagents-probe']) {
  const check = spawnSync('claude', ['plugin', 'validate', plugin, '--strict'], { encoding: 'utf8' });
  if (check.status !== 0) throw new Error(check.stdout + check.stderr);
}
const env = { ...process.env, AX_LOG: 'debug' };
if (!live) {
  env.PATH = bin + delimiter + env.PATH;
  env.AX_SUBAGENT_SMOKE_PID_FILE = join(root, 'cancelled-pids.json');
}
const child = spawn(ax, ['claude', '--subagents', '--', '--plugin-dir', resolve('test/smoke/fixtures/agentx-subagents-probe'), '--print', '--max-turns', '1', '--debug-file', join(root, 'claude-debug.log'), '--verbose', '--output-format', 'stream-json', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}', '--', 'Return only AX_PROBE_NOT_LOADED.'], {
  env, detached: process.platform !== 'win32', stdio: ['ignore', 'pipe', 'pipe'],
});
const result = { stdout: '', stderr: '', status: null, error: null };
function stop() {
  try {
    if (process.platform === 'win32') child.kill();
    else process.kill(-child.pid, 'SIGTERM');
  } catch (error) { if (error.code !== 'ESRCH') throw error; }
}
for (const stream of ['stdout', 'stderr']) {
  child[stream].setEncoding('utf8');
  child[stream].on('data', data => {
    result[stream] += data;
    if (result[stream].length > 16 * 1024 * 1024) { result.error = new Error('Smoke output limit exceeded'); stop(); }
  });
}
const timer = setTimeout(() => { result.error = new Error('Smoke timed out'); stop(); }, 120000);
await new Promise(resolve => {
  child.on('error', error => { result.error = error; resolve(); });
  child.on('close', status => { result.status = status; resolve(); });
});
clearTimeout(timer);
writeFileSync(join(root, 'stdout.jsonl'), result.stdout ?? '');
writeFileSync(join(root, 'stderr.log'), result.stderr ?? '');
console.log(`Evidence: ${root}`);
const records = (result.stdout ?? '').split('\n').filter(Boolean).flatMap(line => { try { return [JSON.parse(line)] } catch { return [] } });
const final = records.find(record => record.type === 'result' && record.result?.includes('AX_SMOKE_OK')) ?? records.findLast(record => record.type === 'result');
if (result.error || result.status !== 0 || !final?.result?.includes('AX_SMOKE_OK')) {
  console.error(result.error ?? final ?? result.stderr);
  process.exit(1);
}
if (records.some(record => record.type === 'result' && Object.keys(record.modelUsage ?? {}).length)) {
  throw new Error('Unexpected Claude inference usage in the smoke probe');
}
if (!live) {
  const pids = JSON.parse(readFileSync(env.AX_SUBAGENT_SMOKE_PID_FILE, 'utf8'));
  for (const pid of pids) {
    let alive = true;
    try { process.kill(pid, 0); } catch (error) { if (error.code === 'ESRCH') alive = false; else throw error; }
    if (alive) {
      process.kill(pid, 'SIGKILL');
      throw new Error(`Cancelled external descendant ${pid} was left running`);
    }
  }
}
console.log(final.result);
