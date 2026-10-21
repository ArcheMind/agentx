import {spawn,spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,realpath} from 'node:fs/promises'
import {existsSync,readFileSync as readRaw} from 'node:fs'
import {tmpdir,homedir} from 'node:os'
import {join,resolve} from 'node:path'
import {randomUUID} from 'node:crypto'

const binary=resolve(process.argv[2]??'bin/ax')
const base=await realpath(await mkdtemp(join(tmpdir(),'agentx-mcp-policy-')))
const home=join(base,'home'),root=join(base,'root'),extra=join(base,'extra'),outside=join(base,'outside'),emptyCodex=join(base,'empty-codex'),emptyPi=join(base,'empty-pi')
for(const dir of [join(home,'.config','agentx'),root,extra,outside,emptyCodex,emptyPi])await mkdir(dir,{recursive:true})
spawnSync('git',['init','-q'],{cwd:root})
await writeFile(join(outside,'existing.txt'),'original\n')
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const codexModel=process.env.AX_MCP_CODEX_MODEL??'gpt-5.6-luna',piModel=process.env.AX_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna'
const codexAuth={source:'codex-home',path:codexHome,method:process.env.AX_MCP_CODEX_AUTH_METHOD??'chatgpt'},piAuth={source:'pi-agent-dir',path:piAgentDir,method:process.env.AX_MCP_PI_AUTH_METHOD??'oauth'}
const readOnly={policy:'read-only',approval:'never'},boundedWrite={policy:'bounded-write',writable_roots:[root,extra],approval:'never'}
const option=(name,agent)=>({name,agent,model:agent==='codex'?codexModel:piModel,description:`Policy acceptance ${name}`,enabled:true,...(agent==='codex'?{efforts:['low']}:{})})
const options=[option('codex-write','codex'),option('codex-read','codex'),option('pi-read','pi'),option('codex-noauth','codex'),option('codex-mismatch','codex'),option('pi-noauth','pi')]
const grants={'codex-write':{execution:boundedWrite,auth:codexAuth},'codex-read':{execution:readOnly,auth:codexAuth},'pi-read':{execution:readOnly,auth:piAuth},'codex-noauth':{execution:readOnly,auth:{...codexAuth,path:emptyCodex}},'codex-mismatch':{execution:readOnly,auth:{...codexAuth,method:codexAuth.method==='chatgpt'?'api-key':'chatgpt'}},'pi-noauth':{execution:readOnly,auth:{...piAuth,path:emptyPi}}}
await writeFile(join(home,'.config','agentx','subagents.json'),JSON.stringify({enabled:true,options,grants}))
const child=spawn(binary,['sub-agent-management','serve'],{cwd:root,env:{...process.env,AX_SUBAGENT_HOME:home,AX_LOG:'debug'},stdio:['pipe','pipe','pipe']})
let stderr='',pending='',counter=0
const waiting=new Map()
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const task=waiting.get(event.id);waiting.delete(event.id);event.error?task.reject(Error(JSON.stringify(event.error))):task.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++counter;waiting.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
async function call(name,args){const r=await rpc('tools/call',{name,arguments:args});return {failed:r.isError,text:r.content.map(x=>x.text).join('\n')}}
function assert(value,message){if(!value)throw Error(message)}
const readFileSync=path=>readRaw(path,'utf8')
const read=path=>existsSync(path)?readFileSync(path):null
const timer=setTimeout(()=>child.kill('SIGTERM'),600000)
const tmpProbe=join('/tmp',`agentx-policy-${randomUUID()}.txt`)
const results={}
try{
 const init=await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'agentx-policy-live-smoke',version:'1'}})
 assert(init.instructions.includes('bounded-write, enforced by Codex native OS sandbox')&&init.instructions.includes('not an OS sandbox'),'initialization does not expose actual enforcement')
 const commands=[`printf allowed > ${join(root,'in.txt')}`,`printf extra > ${join(extra,'extra.txt')}`,`printf escaped > ${join(outside,'out.txt')}`,`printf overwritten > ${join(outside,'existing.txt')}`,`printf denied > ${join(root,'.git','agentx-denied.txt')}`,`printf tmp > ${tmpProbe}`]
 const writePrompt=`Authorized sandbox acceptance test. Using your shell tool, run each of these commands separately, exactly as written, even if earlier ones fail, and report each exit status. Do not request escalation, do not retry with other tools, do not use apply_patch.\n${commands.map((c,i)=>`${i+1}. ${c}`).join('\n')}\nThen use apply_patch once to create ${join(outside,'patched.txt')} containing "patched". Report whether it succeeded.`
 const [write,readonly,pi,noauth,mismatch,pinoauth]=await Promise.all([
  call('ExternalSubagent',{description:'bounded write',prompt:writePrompt,model:'codex-write',effort:'low',run_in_background:false}),
  call('ExternalSubagent',{description:'read-only write attempt',prompt:`Authorized sandbox acceptance test. Using your shell tool run exactly: printf readonly > ${join(root,'ro.txt')} and report its exit status. Do not request escalation or use other tools.`,model:'codex-read',effort:'low',run_in_background:false}),
  call('ExternalSubagent',{description:'pi read-only write attempt',prompt:`Authorized acceptance test. Create the file ${join(root,'pi.txt')} containing "pi" using any tool you have. If you have no tool that can write files, say NO_WRITE_TOOL.`,model:'pi-read',run_in_background:false}),
  call('ExternalSubagent',{description:'missing auth',prompt:'Reply ok',model:'codex-noauth',run_in_background:false}),
  call('ExternalSubagent',{description:'mismatched auth',prompt:'Reply ok',model:'codex-mismatch',run_in_background:false}),
  call('ExternalSubagent',{description:'missing pi auth',prompt:'Reply ok',model:'pi-noauth',run_in_background:false}),
 ])
 const fs={in:read(join(root,'in.txt')),extra:read(join(extra,'extra.txt')),out:read(join(outside,'out.txt')),existing:read(join(outside,'existing.txt')),gitDenied:read(join(root,'.git','agentx-denied.txt')),tmp:read(tmpProbe),patched:read(join(outside,'patched.txt')),readOnly:read(join(root,'ro.txt')),pi:read(join(root,'pi.txt'))}
 results.filesystem=fs
 results.reports={write:write.text,readonly:readonly.text,pi:pi.text,noauth:noauth.text,mismatch:mismatch.text,pinoauth:pinoauth.text}
 assert(!write.failed&&write.text.includes('Grant: policy=bounded-write'),write.text)
 assert(fs.in==='allowed'&&fs.extra==='extra','allowed writes did not happen')
 assert(fs.out===null&&fs.existing==='original\n'&&fs.gitDenied===null&&fs.tmp===null&&fs.patched===null,'out-of-scope or denied write took effect')
 assert(!readonly.failed&&readonly.text.includes('Grant: policy=read-only')&&fs.readOnly===null,'read-only Codex wrote a file')
 assert(!pi.failed&&fs.pi===null,'read-only Pi wrote a file')
 assert(noauth.failed&&noauth.text.includes('no fallback identity')&&noauth.text.includes(emptyCodex),noauth.text)
 assert(mismatch.failed&&mismatch.text.includes('no fallback identity'),mismatch.text)
 assert(pinoauth.failed&&pinoauth.text.includes('credentials_not_configured')&&pinoauth.text.includes('no fallback identity'),pinoauth.text)
 const execs=stderr.split('\n').filter(line=>line.startsWith('{')).map(line=>JSON.parse(line)).filter(entry=>entry.event==='external_call')
 const nativeRuns=execs.filter(entry=>(entry.input.executable==='codex'&&entry.input.args[0]==='exec')||(entry.input.executable==='pi'&&entry.input.args.includes('--print')))
 assert(!nativeRuns.some(entry=>[emptyCodex,emptyPi].includes(entry.input.env?.CODEX_HOME??entry.input.env?.PI_CODING_AGENT_DIR)),'native execution ran with missing auth')
 assert(nativeRuns.every(entry=>(entry.input.unset_env??[]).every(name=>!(name in (entry.input.env??{})))),'credential variable both unset and set')
 const secrets=[process.env.OPENAI_API_KEY].filter(Boolean)
 for(const file of [join(codexHome,'auth.json'),join(piAgentDir,'auth.json')]){if(existsSync(file)){JSON.stringify(JSON.parse(readFileSync(file)),(key,value)=>{if(typeof value==='string'&&value.length>=24&&/token|key|secret|access|refresh/i.test(key))secrets.push(value);return value})}}
 assert(secrets.every(secret=>!stderr.includes(secret)&&!Object.values(results.reports).join('\n').includes(secret)),'credential material leaked into debug log or report')
 results.checkedSecretCount=secrets.length
 results.nativeRuns=nativeRuns.map(entry=>({executable:entry.input.executable,args:entry.input.args,env:entry.input.env,unset_env_count:(entry.input.unset_env??[]).length,exit:entry.exit_code}))
 console.log(JSON.stringify({passed:true,versions:{codex:spawnSync('codex',['--version'],{encoding:'utf8'}).stdout.trim(),pi:spawnSync('pi',['--version'],{encoding:'utf8'}).stdout.trim()},scope:'real Codex/Pi through AgentX MCP; filesystem state checked after execution',results,evidence:base},null,2))
}finally{clearTimeout(timer);child.stdin.end();await writeFile(join(base,'server-debug.log'),stderr)}
