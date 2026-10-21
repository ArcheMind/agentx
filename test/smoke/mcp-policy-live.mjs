import {spawn,spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,realpath} from 'node:fs/promises'
import {existsSync,readFileSync as readRaw} from 'node:fs'
import {tmpdir,homedir} from 'node:os'
import {join,resolve} from 'node:path'
import {randomUUID} from 'node:crypto'

const binary=resolve(process.argv[2]??'bin/ax')
const base=await realpath(await mkdtemp(join(tmpdir(),'agentx-mcp-policy-')))
const home=join(base,'home'),root=join(base,'root'),extra=join(base,'extra'),outside=join(base,'outside'),emptyCodex=join(base,'empty-codex'),emptyPi=join(base,'empty-pi')
for(const dir of [home,root,extra,outside,emptyCodex,emptyPi])await mkdir(dir,{recursive:true})
spawnSync('git',['init','-q'],{cwd:root})
await writeFile(join(outside,'existing.txt'),'original\n')
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const codexModel=process.env.AX_MCP_CODEX_MODEL??'gpt-5.6-luna',piModel=process.env.AX_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna'
const presetList=[['codex','codex',codexModel,'low'],['pi','pi',piModel,'']]
for(const [name,agent,model,effort] of presetList){const dir=join(home,'.agentx','presets',name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${model}\n${effort?`effort: ${effort}\n`:''}---\n\nPolicy acceptance ${name}\n`)}
// Auth is discovered from the server environment, so missing credentials need their own server.
function startServer(authEnv){
 const child=spawn(binary,['sub-agent-management','serve'],{cwd:root,env:{...process.env,AX_SUBAGENT_HOME:home,AX_LOG:'debug',...authEnv},stdio:['pipe','pipe','pipe']})
 const server={child,stderr:'',pending:'',counter:0,waiting:new Map()}
 child.stderr.on('data',chunk=>server.stderr+=chunk)
 child.stdout.on('data',chunk=>{server.pending+=chunk;let end;while((end=server.pending.indexOf('\n'))>=0){const line=server.pending.slice(0,end);server.pending=server.pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const task=server.waiting.get(event.id);server.waiting.delete(event.id);event.error?task.reject(Error(JSON.stringify(event.error))):task.resolve(event.result)}}})
 server.rpc=(method,params={})=>new Promise((resolve,reject)=>{const id=++server.counter;server.waiting.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})
 server.call=async(name,args)=>{const r=await server.rpc('tools/call',{name,arguments:args});return {failed:r.isError,text:r.content.map(x=>x.text).join('\n')}}
 return server
}
const main=startServer({CODEX_HOME:codexHome,PI_CODING_AGENT_DIR:piAgentDir}),noauthServer=startServer({CODEX_HOME:emptyCodex,PI_CODING_AGENT_DIR:emptyPi})
const rpc=main.rpc,call=main.call
function assert(value,message){if(!value)throw Error(message)}
const readFileSync=path=>readRaw(path,'utf8')
const read=path=>existsSync(path)?readFileSync(path):null
const timer=setTimeout(()=>{main.child.kill('SIGTERM');noauthServer.child.kill('SIGTERM')},600000)
const tmpProbe=join('/tmp',`agentx-policy-${randomUUID()}.txt`)
function nativeCalls(stderr=main.stderr){return stderr.split('\n').filter(line=>line.startsWith('{')).map(line=>JSON.parse(line)).filter(entry=>entry.event==='external_call')}
function threadFor(marker){const entry=nativeCalls().find(e=>e.input.executable==='codex'&&e.input.args[0]==='exec'&&e.stdin?.includes(marker));const started=entry?.output.split('\n').filter(Boolean).map(l=>JSON.parse(l)).find(e=>e.type==='thread.started');return started?.thread_id}
function rollout(thread){const found=spawnSync('find',[join(codexHome,'sessions'),'-name',`rollout-*-${thread}.jsonl`],{encoding:'utf8'}).stdout.trim().split('\n').filter(Boolean);assert(found.length===1,`rollout for ${thread}: ${found}`);return found[0]}
// Codex code mode runs shell/apply_patch inside an exec tool whose real I/O is only in the rollout, not in exec --json events.
function toolCalls(path){const calls=new Map(),outputs=new Map();for(const line of readFileSync(path).split('\n').filter(Boolean)){const p=JSON.parse(line).payload??{};if(p.type==='custom_tool_call'||p.type==='function_call')calls.set(p.call_id,p.input??p.arguments??'');if(p.type==='custom_tool_call_output'||p.type==='function_call_output'){const out=p.output;outputs.set(p.call_id,typeof out==='string'?out:Array.isArray(out)?out.map(x=>x.text??'').join(''):JSON.stringify(out))}}return [...calls].map(([id,input])=>({input,output:outputs.get(id)??''}))}
const results={probes:{}}
try{
 const init=await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'agentx-policy-live-smoke',version:'1'}})
 assert(init.instructions.includes('Writes: read-only unless the call requests write, enforced by Codex native OS sandbox')&&init.instructions.includes('Writes: never. Pi runs read-only')&&init.instructions.includes('not an OS sandbox'),'initialization does not expose actual enforcement')
 const shell=(cmd,marker)=>`Authorized sandbox acceptance test ${marker}. Using your shell tool, run exactly this one command once, unchanged: ${cmd}\nReport its exit status and output. Do not retry, request escalation, or use any other tool.`
 const probes=[
  {id:'allowed-root',write:[root,extra],target:join(root,'in.txt'),cmd:`printf allowed > ${join(root,'in.txt')}`,expect:'written',content:'allowed'},
  {id:'allowed-extra',write:[root,extra],target:join(extra,'extra.txt'),cmd:`printf extra > ${join(extra,'extra.txt')}`,expect:'written',content:'extra'},
  {id:'outside-new',write:[root,extra],target:join(outside,'out.txt'),cmd:`printf escaped > ${join(outside,'out.txt')}`,expect:'os-denied',content:null},
  {id:'outside-overwrite',write:[root,extra],target:join(outside,'existing.txt'),cmd:`printf overwritten > ${join(outside,'existing.txt')}`,expect:'os-denied',content:'original\n'},
  {id:'git-protected',write:[root,extra],target:join(root,'.git','agentx-denied.txt'),cmd:`printf denied > ${join(root,'.git','agentx-denied.txt')}`,expect:'os-denied',content:null},
  {id:'tmp',write:[root,extra],target:tmpProbe,cmd:`printf tmp > ${tmpProbe}`,expect:'os-denied',content:null},
  {id:'read-only',write:null,target:join(root,'ro.txt'),cmd:`printf readonly > ${join(root,'ro.txt')}`,expect:'os-denied',content:null},
  {id:'patch-outside',write:[root,extra],target:join(outside,'patched.txt'),expect:'patch-rejected',content:null},
  {id:'cwd-implicit',write:[extra],target:join(root,'cwd.txt'),cmd:`printf cwd > ${join(root,'cwd.txt')}`,expect:'written',content:'cwd'},
 ]
 const prompt=(probe,marker)=>probe.cmd?shell(probe.cmd,marker):`Authorized sandbox acceptance test ${marker}. Use apply_patch exactly once to add the file ${probe.target} containing the single line "patched". Report whether it succeeded. Do not retry, request escalation, or use any other tool.`
 const scope=probe=>probe.write?{write:true,writable_roots:probe.write}:{}
 const [probeReports,pi,piWrite,noauth,pinoauth]=await Promise.all([
  Promise.all(probes.map(async probe=>{const marker=`[probe ${probe.id} ${randomUUID()}]`;return {probe,marker,report:await call('ExternalSubagent',{description:`policy probe ${probe.id}`,prompt:prompt(probe,marker),model:'codex',...scope(probe),run_in_background:false})}})),
  call('ExternalSubagent',{description:'pi read-only write attempt',prompt:`Authorized acceptance test. Create the file ${join(root,'pi.txt')} containing "pi" using any tool you have. If you have no tool that can write files, say NO_WRITE_TOOL.`,model:'pi',run_in_background:false}),
  call('ExternalSubagent',{description:'pi write request',prompt:'PI-WRITE-REQUEST reply ok',model:'pi',write:true,writable_roots:[root],run_in_background:false}),
  noauthServer.call('ExternalSubagent',{description:'missing auth',prompt:'Reply ok',model:'codex',run_in_background:false}),
  noauthServer.call('ExternalSubagent',{description:'missing pi auth',prompt:'Reply ok',model:'pi',run_in_background:false}),
 ])
 for(const {probe,marker,report} of probeReports){
  const expectedScope=probe.write?'Write scope: writable roots':'Write scope: read-only (no write requested)'
  assert(!report.failed&&report.text.includes(expectedScope)&&report.text.includes(`Auth (discovered by preflight for this turn): CODEX_HOME=${JSON.stringify(codexHome)} (`),`${probe.id}: ${report.text}`)
  const thread=threadFor(marker);assert(thread,`${probe.id}: no native thread`)
  const path=rollout(thread)
  const attempts=toolCalls(path).filter(c=>c.input.includes(probe.target)&&(probe.cmd?c.input.includes('printf'):c.input.includes('apply_patch')))
  assert(attempts.length>=1,`${probe.id}: rollout ${path} shows no attempt on ${probe.target}`)
  const output=attempts.map(a=>a.output).join('\n')
  if(probe.expect==='os-denied')assert(output.includes('operation not permitted'),`${probe.id}: not denied by OS sandbox: ${output}`)
  if(probe.expect==='patch-rejected')assert(output.includes('rejected by user approval settings'),`${probe.id}: patch not rejected by Codex approval layer: ${output}`)
  if(probe.expect==='written')assert(!output.includes('operation not permitted'),`${probe.id}: allowed write denied: ${output}`)
  const actual=read(probe.target)
  assert(actual===probe.content,`${probe.id}: filesystem ${JSON.stringify(actual)} != ${JSON.stringify(probe.content)}`)
  results.probes[probe.id]={writableRoots:probe.write,expect:probe.expect,thread,rollout:path,attempt:attempts[0].input.slice(0,300),runtimeOutput:output.slice(0,400),filesystem:actual}
 }
 assert(!pi.failed&&read(join(root,'pi.txt'))===null,'read-only Pi wrote a file')
 const piSession=readFileSync(nativeCalls().find(e=>e.input.executable==='pi'&&e.input.args.includes('--print')).input.args.find((a,i,all)=>all[i-1]==='--session'))
 assert(!/"name":"(write|edit|bash)"/.test(piSession),'Pi session shows a write-capable tool call')
 results.pi={report:pi.text,writeCapableToolCalls:0}
 assert(piWrite.failed&&piWrite.text.includes('Pi has no runtime write boundary')&&!nativeCalls().some(e=>e.input.executable==='pi'&&e.stdin?.includes('PI-WRITE-REQUEST')),'Pi write request: '+piWrite.text)
 results.piWrite=piWrite.text
 results.auth={noauth:noauth.text,pinoauth:pinoauth.text}
 assert(noauth.failed&&noauth.text.includes('no fallback identity')&&noauth.text.includes(emptyCodex),noauth.text)
  assert(pinoauth.failed&&pinoauth.text.includes('credentials_not_configured')&&pinoauth.text.includes('no fallback identity'),pinoauth.text)
 const isNative=entry=>(entry.input.executable==='codex'&&entry.input.args[0]==='exec')||(entry.input.executable==='pi'&&entry.input.args.includes('--print'))
 assert(!nativeCalls(noauthServer.stderr).some(isNative),'native execution ran after a failed auth preflight')
 const nativeRuns=nativeCalls().filter(entry=>(entry.input.executable==='codex'&&entry.input.args[0]==='exec')||(entry.input.executable==='pi'&&entry.input.args.includes('--print')))
  const secrets=[process.env.OPENAI_API_KEY].filter(Boolean)
 for(const file of [join(codexHome,'auth.json'),join(piAgentDir,'auth.json')]){if(existsSync(file)){JSON.stringify(JSON.parse(readFileSync(file)),(key,value)=>{if(typeof value==='string'&&value.length>=24&&/token|key|secret|access|refresh/i.test(key))secrets.push(value);return value})}}
 const reports=[...probeReports.map(p=>p.report.text),pi.text,piWrite.text,noauth.text,pinoauth.text].join('\n')
 assert(secrets.every(secret=>!main.stderr.includes(secret)&&!noauthServer.stderr.includes(secret)&&!reports.includes(secret)),'credential material leaked into debug log or report')
 results.checkedSecretCount=secrets.length
 results.unsetEnvCounts=[...new Set(nativeRuns.map(e=>(e.input.unset_env??[]).length))]
 console.log(JSON.stringify({passed:true,versions:{codex:spawnSync('codex',['--version'],{encoding:'utf8'}).stdout.trim(),pi:spawnSync('pi',['--version'],{encoding:'utf8'}).stdout.trim()},scope:'real Codex/Pi through AgentX MCP; write scope requested per call; one command per ExternalSubagent; attempts and runtime denials read from Codex rollouts; filesystem state checked; auth discovered from the server environment',results,evidence:base},null,2))
}finally{clearTimeout(timer);main.child.stdin.end();noauthServer.child.stdin.end();await writeFile(join(base,'server-debug.log'),main.stderr);await writeFile(join(base,'server-noauth-debug.log'),noauthServer.stderr)}
