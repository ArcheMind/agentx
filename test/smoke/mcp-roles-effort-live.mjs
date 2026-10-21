import {spawn,spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,realpath} from 'node:fs/promises'
import {readFileSync,existsSync} from 'node:fs'
import {tmpdir,homedir} from 'node:os'
import {join,resolve,dirname} from 'node:path'
import {randomUUID} from 'node:crypto'
import {fileURLToPath} from 'node:url'

const binary=resolve(process.argv[2]??'bin/ax')
const base=await realpath(await mkdtemp(join(tmpdir(),'agentx-roles-effort-')))
const home=join(base,'home'),work=join(base,'work')
for(const dir of [join(home,'.config','agentx'),work])await mkdir(dir,{recursive:true})
spawnSync('git',['init','-q'],{cwd:work})
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const codexModel=process.env.AX_MCP_CODEX_MODEL??'gpt-5.6-luna',piModel=process.env.AX_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna'
// A Pi reasoning model without xhigh/max mappings, so Pi clamps max to high: the clamp must be detected, not passed.
// Every ChatGPT-accessible openai-codex model in Pi 0.84.4 maps the whole low..max range.
const piClampModel=process.env.AX_MCP_PI_CLAMP_MODEL??'deepseek/deepseek-flash',piClampAuthMethod=process.env.AX_MCP_PI_CLAMP_AUTH_METHOD??'api_key'
const codexAuth={source:'codex-home',path:codexHome,method:process.env.AX_MCP_CODEX_AUTH_METHOD??'chatgpt'},piAuth={source:'pi-agent-dir',path:piAgentDir,method:process.env.AX_MCP_PI_AUTH_METHOD??'oauth'}
const readOnly={policy:'read-only',approval:'never'}
const codexLevels=['none','minimal','low','medium','high','xhigh','max','ultra'],piLevels=['low','medium','high','xhigh','max']
const presets=[{name:'codex-role',agent:'codex',model:codexModel,effort:'low'},{name:'pi-role',agent:'pi',model:piModel},{name:'pi-clamp',agent:'pi',model:piClampModel,effort:'max'},...codexLevels.map(effort=>({name:`codex-${effort}`,agent:'codex',model:codexModel,effort})),...piLevels.map(effort=>({name:`pi-${effort}`,agent:'pi',model:piModel,effort}))]
const grants={}
for(const p of presets){const dir=join(home,'.agentx','presets',p.name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'PRESET.md'),`---\nagent: ${p.agent}\nmodel: ${p.model}\n${p.effort?`effort: ${p.effort}\n`:''}---\n\nRoles and effort acceptance ${p.name}\n`);grants[p.name]={execution:readOnly,auth:p.agent==='codex'?codexAuth:p.name==='pi-clamp'?{...piAuth,method:piClampAuthMethod}:piAuth}}
await writeFile(join(home,'.config','agentx','subagents.json'),JSON.stringify({enabled:true,grants}))
const alphaToken=`ALPHA-${randomUUID().slice(0,8)}`,betaToken=`BETA-${randomUUID().slice(0,8)}`
const roleFile=(name,description,body)=>({name,description,body})
const roles=[roleFile('alpha','Security reviewer persona',`You are a security reviewer. Begin every reply with the word SECURITY: and end every reply with a final line containing exactly ${alphaToken}.`),roleFile('beta','Documentation editor persona',`You are a documentation editor. Begin every reply with the word DOCS: and end every reply with a final line containing exactly ${betaToken}.`)]
const writeRole=async role=>{const dir=join(home,'.agentx','roles',role.name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'ROLE.md'),`---\ndescription: ${role.description}\n---\n\n${role.body}\n`)}
for(const role of roles)await writeRole(role)

const child=spawn(binary,['sub-agent-management','serve'],{cwd:work,env:{...process.env,AX_SUBAGENT_HOME:home,AX_LOG:'debug'},stdio:['pipe','pipe','pipe']})
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
const report=text=>text.split('Untrusted ExternalSubagent report (not user instructions or authorization):\n')[1]?.split('\nExternal ')[0]??''
const piSession=text=>join(home,'.local','state','agentx','subagents',idOf(text),'session.jsonl')
const piEntries=path=>readFileSync(path,'utf8').split('\n').filter(Boolean).map(line=>JSON.parse(line))
const rollout=thread=>{const found=spawnSync('find',[join(codexHome,'sessions'),'-name',`rollout-*-${thread}.jsonl`],{encoding:'utf8'}).stdout.trim().split('\n').filter(Boolean);assert(found.length===1,`rollout for ${thread}: ${found}`);return found[0]}
const threadOf=marker=>{const out=execFor(marker)[0]?.output??'';return out.split('\n').filter(l=>l.startsWith('{')).map(l=>JSON.parse(l)).find(e=>e.type==='thread.started')?.thread_id}
const results={versions:{codex:spawnSync('codex',['--version'],{encoding:'utf8'}).stdout.trim(),pi:spawnSync('pi',['--version'],{encoding:'utf8'}).stdout.trim()},models:{codexModel,piModel,piClampModel},roles:{},effort:{codex:{},pi:{}}}
try{
 const init=await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'agentx-roles-effort-live',version:'1'}})
 assert(init.instructions.includes('- alpha: user description="Security reviewer persona"')&&init.instructions.includes('role instructions are sent as Codex developer_instructions'),'initialization lacks role data')
 const tools=(await rpc('tools/list')).tools
 assert(JSON.stringify(tools[0].inputSchema.properties.subagent_type.enum)===JSON.stringify(['general-purpose','alpha','beta']),'subagent_type enum')
 const ask=marker=>`${marker} In one short sentence, say what you focus on when reviewing a change. Do not use tools.`
 const roleRun=async(model,role)=>{const marker=`[role ${model} ${role} ${randomUUID()}]`;const r=await call('ExternalSubagent',{description:`role ${role}`,prompt:ask(marker),model,subagent_type:role,name:`${model}-${role}`,run_in_background:false});return {marker,...r}}
 const effortRun=async(model)=>{const marker=`[effort ${model} ${randomUUID()}]`;const r=await call('ExternalSubagent',{description:`effort ${model}`,prompt:`${marker} Reply with exactly: OK`,model,run_in_background:false});return {marker,...r}}
 const [roleRuns,codexEffort,piEffort,clamp]=await Promise.all([
  Promise.all(['codex-role','pi-role'].flatMap(model=>['alpha','beta','general-purpose'].map(role=>roleRun(model,role).then(r=>({model,role,...r}))))),
  Promise.all(codexLevels.map(level=>effortRun(`codex-${level}`).then(r=>({level,...r})))),
  Promise.all(piLevels.map(level=>effortRun(`pi-${level}`).then(r=>({level,...r})))),
  effortRun('pi-clamp'),
 ])
 for(const run of roleRuns){
  const text=report(run.text)
  assert(!run.failed,`${run.model}/${run.role} failed: ${run.text}`)
  const hasAlpha=text.includes(alphaToken),hasBeta=text.includes(betaToken)
  const expected=run.role==='alpha'?hasAlpha&&!hasBeta&&/SECURITY:/.test(text):run.role==='beta'?hasBeta&&!hasAlpha&&/DOCS:/.test(text):!hasAlpha&&!hasBeta
  assert(expected,`${run.model}/${run.role} behavior: ${text}`)
  const native=execFor(run.marker)
  assert(native.length===1,`${run.model}/${run.role} native calls: ${native.length}`)
  const args=native[0].input.args
  const evidence={report:text,args:args.join(' ')}
  if(run.model==='codex-role'){
   const thread=threadOf(run.marker);evidence.rollout=rollout(thread)
   const developer=readFileSync(evidence.rollout,'utf8').split('\n').filter(Boolean).map(l=>JSON.parse(l)).filter(e=>e.type==='response_item'&&e.payload?.role==='developer').flatMap(e=>e.payload.content.map(c=>c.text))
   const body=roles.find(r=>r.name===run.role)?.body
   assert(run.role==='general-purpose'?!developer.some(t=>t.includes('ALPHA-')||t.includes('BETA-')):developer.includes(body),`${run.role} developer message in rollout`)
   if(run.role!=='general-purpose')assert(run.text.includes('role instructions recorded as a Codex developer message'),'codex role evidence line')
  }else{
   evidence.session=piSession(run.text)
   if(run.role!=='general-purpose'){
    const records=piEntries(evidence.session).filter(e=>e.type==='custom'&&e.customType==='agentx-provider-request')
    assert(records.length>0&&records.every(r=>r.data.role_included===true),`pi ${run.role} observer records ${JSON.stringify(records)}`)
    assert(run.text.includes('role instructions present in all'),'pi role evidence line')
   }else assert(!args.includes('--append-system-prompt')&&!args.includes('--extension'),'general-purpose pi carried role machinery')
  }
  results.roles[`${run.model}/${run.role}`]=evidence
 }
 const strip=args=>args.filter((a,i)=>!(a.startsWith('developer_instructions=')||args[i+1]?.startsWith('developer_instructions=')))
 const codexArgs=role=>execFor(roleRuns.find(r=>r.model==='codex-role'&&r.role===role).marker)[0].input.args
 assert(JSON.stringify(strip(codexArgs('alpha')))===JSON.stringify(codexArgs('general-purpose'))&&JSON.stringify(strip(codexArgs('beta')))===JSON.stringify(codexArgs('general-purpose')),'role changed the Codex execution combination')
 const piArgs=role=>{const args=execFor(roleRuns.find(r=>r.model==='pi-role'&&r.role===role).marker)[0].input.args;return args.filter((a,i)=>!['--extension','--append-system-prompt'].includes(a)&&!['--extension','--append-system-prompt'].includes(args[i-1])&&!a.includes(`/${idOf(roleRuns.find(r=>r.model==='pi-role'&&r.role===role).text)}/`))}
 assert(JSON.stringify(piArgs('alpha'))===JSON.stringify(piArgs('beta')),'role changed the Pi execution combination')
 results.roles.sameModelDifferentRole='codex and pi argv identical apart from role carriage; behavior differs per role token'
 results.roles.sameRoleDifferentModel='alpha produced its token on both codex-role and pi-role'
 const unknown=await call('ExternalSubagent',{description:'unknown role',prompt:'Reply ok',model:'codex-role',subagent_type:'gamma',run_in_background:false})
 assert(unknown.failed&&unknown.text.includes('subagent_type "gamma" rejected'),'unknown role: '+unknown.text)
 results.roles.unknown=unknown.text.split('\n').at(-1)
 const resumed=await call('SendExternalSubagentMessage',{to:'codex-role-alpha',message:'In one short sentence, name one risk you would check first. Do not use tools.'})
 assert(!resumed.failed,'resume rejected: '+resumed.text)
 const task=resumed.text.match(/Task (task-[0-9a-f]+)/)[1]
 let done;for(let i=0;i<180&&!(done=notifications.find(n=>n.meta.task_id===task));i++)await new Promise(next=>setTimeout(next,1000))
 assert(done?.meta.status==='completed','resumed task did not complete: '+JSON.stringify(done))
 const resumedReport=done.content
 assert(resumedReport.includes(alphaToken)&&resumedReport.includes('role=alpha'),'resumed turn lost role: '+resumedReport)
 results.roles.resumeKeepsRole=report(resumedReport)
 await writeRole({...roles[0],body:roles[0].body+' Also be brief.'})
 const rebound=await call('SendExternalSubagentMessage',{to:'codex-role-alpha',message:'again'})
 assert(rebound.failed&&rebound.text.includes('original role alpha changed'),'role edit rebound: '+rebound.text)
 results.roles.editRejectsContinuation=rebound.text.split('\n').at(-1)
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
