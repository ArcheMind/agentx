import {spawn,spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,realpath} from 'node:fs/promises'
import {readFileSync,existsSync} from 'node:fs'
import {tmpdir,homedir} from 'node:os'
import {join,resolve,dirname} from 'node:path'
import {randomUUID} from 'node:crypto'
import {fileURLToPath} from 'node:url'

const binary=resolve(process.argv[2]??'bin/ax')
const base=await realpath(await mkdtemp(join(tmpdir(),'agentx-effort-')))
const home=join(base,'home'),work=join(base,'work')
for(const dir of [home,work])await mkdir(dir,{recursive:true})
spawnSync('git',['init','-q'],{cwd:work})
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const codexModel=process.env.AX_MCP_CODEX_MODEL??'gpt-5.6-luna',piModel=process.env.AX_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna'
// A Pi reasoning model without xhigh/max mappings, so Pi clamps max to high: the clamp must be detected, not passed.
// Every ChatGPT-accessible openai-codex model in Pi 0.84.4 maps the whole low..max range.
const piClampModel=process.env.AX_MCP_PI_CLAMP_MODEL??'deepseek/deepseek-flash'
const codexLevels=['none','minimal','low','medium','high','xhigh','max','ultra'],piLevels=['low','medium','high','xhigh','max']
const presets=[{name:'pi-clamp',agent:'pi',model:piClampModel,effort:'max'},...codexLevels.map(effort=>({name:`codex-${effort}`,agent:'codex',model:codexModel,effort})),...piLevels.map(effort=>({name:`pi-${effort}`,agent:'pi',model:piModel,effort}))]
for(const p of presets){const dir=join(home,'.agentx','presets',p.name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'PRESET.md'),`---\nagent: ${p.agent}\nmodel: ${p.model}\n${p.effort?`effort: ${p.effort}\n`:''}---\n\nEffort acceptance ${p.name}\n`)}

const child=spawn(binary,['preset','serve'],{cwd:work,env:{...process.env,AX_SUBAGENT_HOME:home,AX_LOG:'debug',CODEX_HOME:codexHome,PI_CODING_AGENT_DIR:piAgentDir},stdio:['pipe','pipe','pipe']})
let stderr='',pending='',counter=0
const waiting=new Map(),notifications=[]
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.method==='notifications/claude/channel')notifications.push(event.params);if(event.id!==undefined){const task=waiting.get(event.id);waiting.delete(event.id);event.error?task.reject(Error(JSON.stringify(event.error))):task.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++counter;waiting.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
async function call(name,args){const r=await rpc('tools/call',{name,arguments:args});return {failed:r.isError,text:r.content.map(x=>x.text).join('\n')}}
function assert(value,message){if(!value)throw Error(message)}
const timer=setTimeout(()=>child.kill('SIGTERM'),900000)
const nativeCalls=()=>stderr.split('\n').filter(line=>line.startsWith('{')).map(line=>JSON.parse(line)).filter(entry=>entry.event==='external_call')
const execFor=marker=>nativeCalls().filter(e=>(e.input.args[0]==='exec'||e.input.args.includes('--print'))&&e.stdin?.includes(marker))
const idOf=text=>text.match(/AgentX ExternalSubagent (ax-[0-9a-f]+)/)?.[1]
const piSession=text=>join(home,'.local','state','agentx','subagents',idOf(text),'session.jsonl')
const piEntries=path=>readFileSync(path,'utf8').split('\n').filter(Boolean).map(line=>JSON.parse(line))
const rollout=thread=>{const found=spawnSync('find',[join(codexHome,'sessions'),'-name',`rollout-*-${thread}.jsonl`],{encoding:'utf8'}).stdout.trim().split('\n').filter(Boolean);assert(found.length===1,`rollout for ${thread}: ${found}`);return found[0]}
const threadOf=marker=>{const out=execFor(marker)[0]?.output??'';return out.split('\n').filter(l=>l.startsWith('{')).map(l=>JSON.parse(l)).find(e=>e.type==='thread.started')?.thread_id}
const results={versions:{codex:spawnSync('codex',['--version'],{encoding:'utf8'}).stdout.trim(),pi:spawnSync('pi',['--version'],{encoding:'utf8'}).stdout.trim()},models:{codexModel,piModel,piClampModel},effort:{codex:{},pi:{}}}
try{
 const init=await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'agentx-effort-live',version:'1'}})
 const effortRun=async(model)=>{const marker=`[effort ${model} ${randomUUID()}]`;const r=await call('ExternalSubagent',{description:`effort ${model}`,prompt:`${marker} Reply with exactly: OK`,model,run_in_background:false});return {marker,...r}}
 const [codexEffort,piEffort,clamp]=await Promise.all([
  Promise.all(codexLevels.map(level=>effortRun(`codex-${level}`).then(r=>({level,...r})))),
  Promise.all(piLevels.map(level=>effortRun(`pi-${level}`).then(r=>({level,...r})))),
  effortRun('pi-clamp'),
 ])
 for(const run of codexEffort){
  const otel=execFor(run.marker)[0]?.error_output??''
  const executed=[...otel.matchAll(/event\.kind=response\.completed .*model_reasoning_effort="?([a-z]+)/g)].map(m=>m[1])
  results.effort.codex[run.level]={failed:run.failed,executed,rollout:threadOf(run.marker)?rollout(threadOf(run.marker)):null,reason:run.text.match(/Reason: (.*)/)?.[1]??null}
  if(run.level==='ultra')assert(run.failed&&run.text.includes('native effort mismatch: preset effort ultra')&&run.text.includes('max executed'),'ultra clamp not detected: '+run.text)
  else if(run.level==='minimal')assert(run.failed&&run.text.includes("Unsupported value: 'minimal'"),'minimal not rejected natively: '+run.text)
  else assert(!run.failed&&run.text.includes(`Native evidence: executed effort ${run.level} (`)&&executed.length>0&&executed.every(e=>e===run.level),`codex ${run.level}: ${run.text}`)
 }
 for(const run of piEffort){
  assert(!run.failed&&run.text.includes(`Native evidence: executed effort ${run.level} (`),`pi ${run.level}: ${run.text}`)
  const entries=piEntries(piSession(run.text))
  const thinking=entries.filter(e=>e.type==='thinking_level_change').at(-1)?.thinkingLevel
  const sent=entries.filter(e=>e.type==='custom'&&e.customType==='agentx-provider-request').map(e=>e.data.reasoning_effort)
  assert(thinking===run.level&&sent.length>0&&sent.every(e=>e===run.level),`pi ${run.level} session evidence ${thinking} ${sent}`)
  results.effort.pi[run.level]={session:piSession(run.text),thinking,sent}
 }
 const clampEntries=piEntries(piSession(clamp.text))
 assert(clamp.failed&&clamp.text.includes('Pi session records thinking level "high"'),'pi clamp not detected: '+clamp.text)
 results.effort.pi.clamp={model:piClampModel,requested:'max',session:piSession(clamp.text),thinking:clampEntries.filter(e=>e.type==='thinking_level_change').at(-1)?.thinkingLevel,sent:clampEntries.filter(e=>e.type==='custom').map(e=>e.data.reasoning_effort),reason:clamp.text.match(/Reason: (.*)/)?.[1]}
 // Direct Pi runs with the same observer show why off and minimal stay rejected for presets.
 const observer=join(dirname(fileURLToPath(import.meta.url)),'..','..','internal','subagents','pi_observer.js')
 for(const level of ['off','minimal']){
  const session=join(base,`pi-direct-${level}.jsonl`)
  const env={...process.env,PI_CODING_AGENT_DIR:piAgentDir};delete env.OPENAI_API_KEY
  const run=spawnSync('pi',['--print','--mode','json','--tools','read,grep,find,ls','--no-extensions','--extension',observer,'--no-skills','--no-prompt-templates','--no-approve','--session',session,'--model',piModel,'--thinking',level,'Reply with exactly: OK'],{cwd:work,env,encoding:'utf8',input:''})
  assert(run.status===0,`direct pi ${level}: ${run.stderr}`)
  const entries=piEntries(session)
  results.effort.pi[`direct-${level}`]={session,thinking:entries.filter(e=>e.type==='thinking_level_change').at(-1)?.thinkingLevel,sent:entries.filter(e=>e.type==='custom').map(e=>e.data.reasoning_effort)}
 }
 assert(results.effort.pi['direct-minimal'].thinking==='minimal'&&results.effort.pi['direct-minimal'].sent.every(e=>e==='low'),'minimal remap not observed')
 assert(results.effort.pi['direct-off'].thinking==='off'&&results.effort.pi['direct-off'].sent.every(e=>e===null),'off omission not observed')
 await writeFile(join(base,'evidence.json'),JSON.stringify(results,null,2))
 await writeFile(join(base,'server-debug.log'),stderr)
 console.log(JSON.stringify({passed:true,evidence:base,codex:Object.fromEntries(Object.entries(results.effort.codex).map(([k,v])=>[k,v.failed?'rejected: '+v.reason:'executed '+v.executed.join(',')])),pi:Object.fromEntries(Object.entries(results.effort.pi).map(([k,v])=>[k,`thinking=${v.thinking} sent=${v.sent}`]))},null,2))
}catch(error){await writeFile(join(base,'server-debug.log'),stderr);console.error('evidence:',base);throw error}
finally{clearTimeout(timer);child.stdin.end();await new Promise(done=>child.once('close',done))}
