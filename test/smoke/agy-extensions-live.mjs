import {spawnSync} from 'node:child_process'
import {mkdtempSync, mkdirSync, writeFileSync, readFileSync, readdirSync, realpathSync, existsSync, rmSync} from 'node:fs'
import {tmpdir, homedir} from 'node:os'
import {join, resolve} from 'node:path'
import {randomBytes} from 'node:crypto'

const binary = resolve(process.argv[2] ?? 'bin/sm')
const root = realpathSync(mkdtempSync(join(tmpdir(), 'sm-agy-extensions-live-')))
const plugin = join(homedir(), '.gemini', 'config', 'plugins', 'sessionman')
const ownership = 'sessionman native agy plugin\n'
const snapshot = new Map()
const existed = existsSync(plugin)
if (existed) {
  if (readFileSync(join(plugin, '.sessionman-owned'), 'utf8') !== ownership) throw new Error('Refusing to touch a foreign sessionman plugin')
  for (const name of readdirSync(plugin)) snapshot.set(name, readFileSync(join(plugin, name)))
}
const token = `AGY_HOOK_${randomBytes(10).toString('hex')}`
const events = join(root, 'hook-events.jsonl')
const recorder = join(root, 'record-hook.mjs')
writeFileSync(recorder, `import{appendFileSync,readdirSync,readFileSync}from'node:fs';import{join}from'node:path';let input='';for await(const chunk of process.stdin)input+=chunk;appendFileSync(${JSON.stringify(events)},input+'\\n');try{const dir=join(process.env.SM_SUBAGENT_HOME,'.local','state','sessionman','connections');for(const name of readdirSync(dir))appendFileSync(${JSON.stringify(join(root,'mcp-connections.jsonl'))},readFileSync(join(dir,name),'utf8')+'\\n')}catch{};console.log(JSON.stringify({hookSpecificOutput:{additionalContext:${JSON.stringify(token)}}}));`)
mkdirSync(join(root, '.agents'), {recursive:true})
const command = `${JSON.stringify(process.execPath)} ${JSON.stringify(recorder)}`
writeFileSync(join(root, '.agents', 'hooks.json'), JSON.stringify({hooks:Object.fromEntries(['SessionStart','Stop','SessionEnd','UserPromptSubmit'].map(event=>[event,[{hooks:[{type:'command',command}]}]]))}))
const env = {...process.env, SM_SUBAGENT_HOME:join(root,'subagent-home')}
let result
try {
  const prompt = 'Call the sessionman MCP ListExternalSubagentSessions tool exactly once. Do not use any other tools. Report the result and the AGY_HOOK code injected at session start.'
  const args = ['agy','--cwd',root,'--model',process.env.AX_CROSS_AGY_MODEL ?? 'gemini-3.8-flash-low','--','--output-format','stream-json','--dangerously-skip-permissions',`--print=${prompt}`]
  result = spawnSync(binary,args,{cwd:root,env,input:prompt,encoding:'utf8',timeout:120_000,maxBuffer:16*1024*1024})
  writeFileSync(join(root,'native.json'),JSON.stringify({args,input:prompt,status:result.status,stdout:result.stdout,stderr:result.stderr,error:result.error?.message},null,2))
  if(result.error || result.status!==0) throw new Error(result.error?.message ?? result.stderr)
  const payloads = readFileSync(events,'utf8').trim().split('\n').map(JSON.parse)
  for(const event of ['SessionStart','Stop']) if(payloads.filter(p=>p.hook_event_name===event && p.cwd===root).length!==1) throw new Error(`${event} not dispatched in actual workspace`)
  for(const event of ['SessionEnd','UserPromptSubmit']) if(payloads.some(p=>p.hook_event_name===event)) throw new Error(`${event} was incorrectly projected`)
  if(!result.stderr.includes('does not support portable SessionEnd') || !result.stderr.includes('does not support portable UserPromptSubmit')) throw new Error('Unsupported portable event warnings missing')
  if(!result.stdout.includes(token)) throw new Error('SessionStart context was not natively injected')
  const connections = readFileSync(join(root,'mcp-connections.jsonl'),'utf8').trim().split('\n').map(JSON.parse)
  if(!connections.some(c=>c.cwd===root && c.status==='active MCP connection: host sent notifications/initialized')) throw new Error('Native MCP did not use the launch workspace')
  const stream = result.stdout.trim().split('\n').map(JSON.parse)
  if(!stream.some(e=>e.step_update?.state==='DONE' && e.step_update?.tool_info?.parameters?.ToolName==='ListExternalSubagentSessions' && e.step_update?.tool_info?.output?.includes('SessionMan MCP connection'))) throw new Error('Native stream did not record successful SessionMan MCP tool use')
  console.log(JSON.stringify({passed:true,checks:['owned-native-plugin','native-MCP-tool','runtime-workspace-hooks','SessionStart-injectSteps','unsupported-event-warnings'],evidence:root},null,2))
} finally {
  if(existed) {
    for(const name of ['plugin.json','hooks.json','mcp_config.json','.sessionman-owned']) {
      if(snapshot.has(name)) writeFileSync(join(plugin,name),snapshot.get(name))
      else rmSync(join(plugin,name),{force:true})
    }
  } else if(existsSync(join(plugin,'.sessionman-owned')) && readFileSync(join(plugin,'.sessionman-owned'),'utf8')===ownership) rmSync(plugin,{recursive:true})
}
