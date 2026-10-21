import {spawn,spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,realpath} from 'node:fs/promises'
import {existsSync,readFileSync as readRaw,readdirSync,statSync} from 'node:fs'
import {tmpdir,homedir} from 'node:os'
import {join,resolve,relative} from 'node:path'
import {randomUUID} from 'node:crypto'

const binary=resolve(process.argv[2]??'bin/ax')
const base=await realpath(await mkdtemp(join(tmpdir(),'agentx-mcp-isolation-')))
const home=join(base,'home'),repo=join(base,'repo'),cwd=join(repo,'sub')
for(const dir of [home,cwd])await mkdir(dir,{recursive:true})
const git=(dir,...args)=>{const r=spawnSync('git',['-C',dir,...args],{encoding:'utf8',env:{...process.env,GIT_AUTHOR_NAME:'smoke',GIT_AUTHOR_EMAIL:'smoke@example.invalid',GIT_COMMITTER_NAME:'smoke',GIT_COMMITTER_EMAIL:'smoke@example.invalid'}});if(r.status!==0)throw Error(`git ${args.join(' ')}: ${r.stderr}`);return r.stdout.trim()}
git(repo,'init','-q')
await writeFile(join(cwd,'tracked.txt'),'committed\n')
git(repo,'add','.');git(repo,'commit','-qm','base')
// The original checkout starts dirty: a modified tracked file and an untracked file must survive.
await writeFile(join(cwd,'tracked.txt'),'uncommitted edit\n')
await writeFile(join(repo,'draft.txt'),'untracked draft\n')
function snapshot(){const files={};const walk=dir=>{for(const name of readdirSync(dir)){if(dir===repo&&name==='.git')continue;const path=join(dir,name);statSync(path).isDirectory()?walk(path):files[relative(repo,path)]=readRaw(path,'utf8')}};walk(repo);return JSON.stringify({status:git(repo,'status','--porcelain','--untracked-files=all'),files})}
const before=snapshot()
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const codexModel=process.env.AX_MCP_CODEX_MODEL??'gpt-5.6-luna',piModel=process.env.AX_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna'
// Under isolation the only writable root is the worktree AgentX creates: no caller path string, such
// as a case-variant spelling of the repository (same directory on default macOS APFS), reaches the sandbox.
const repoAlias=join(base,'REPO')
const presets={'iso':['codex',codexModel],'iso-pi':['pi',piModel],'iso-badmodel':['codex','agentx-no-such-model']}
for(const [name,[agent,model]] of Object.entries(presets)){const dir=join(home,'.agentx','presets',name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${model}\n${agent==='codex'?'effort: low\n':''}---\n\nIsolation acceptance ${name}\n`)}
const child=spawn(binary,['sub-agent-management','serve'],{cwd,env:{...process.env,AX_SUBAGENT_HOME:home,AX_LOG:'debug',CODEX_HOME:codexHome,PI_CODING_AGENT_DIR:piAgentDir},stdio:['pipe','pipe','pipe']})
let stderr='',pending='',counter=0
const waiting=new Map()
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const task=waiting.get(event.id);waiting.delete(event.id);event.error?task.reject(Error(JSON.stringify(event.error))):task.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++counter;waiting.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
async function call(name,args){const r=await rpc('tools/call',{name,arguments:args});return {failed:r.isError,text:r.content.map(x=>x.text).join('\n')}}
function assert(value,message){if(!value)throw Error(message)}
const read=path=>existsSync(path)?readRaw(path,'utf8'):null
const timer=setTimeout(()=>child.kill('SIGTERM'),900000)
const sleep=ms=>new Promise(r=>setTimeout(r,ms))
function nativeCalls(){return stderr.split('\n').filter(line=>line.startsWith('{')).map(line=>JSON.parse(line)).filter(entry=>entry.event==='external_call')}
function nativeRun(marker){return nativeCalls().find(e=>((e.input.executable==='codex'&&e.input.args[0]==='exec')||e.input.executable==='pi')&&e.stdin?.includes(marker))}
function threadFor(marker){const started=nativeRun(marker)?.output.split('\n').filter(Boolean).map(l=>JSON.parse(l)).find(e=>e.type==='thread.started');return started?.thread_id}
function rollout(thread){const found=spawnSync('find',[join(codexHome,'sessions'),'-name',`rollout-*-${thread}.jsonl`],{encoding:'utf8'}).stdout.trim().split('\n').filter(Boolean);assert(found.length===1,`rollout for ${thread}: ${found}`);return found[0]}
// Codex code mode runs shell calls inside an exec tool whose real I/O is only in the rollout, not in exec --json events.
function toolCalls(path){const calls=new Map(),outputs=new Map();for(const line of readRaw(path,'utf8').split('\n').filter(Boolean)){const p=JSON.parse(line).payload??{};if(p.type==='custom_tool_call'||p.type==='function_call')calls.set(p.call_id,p.input??p.arguments??'');if(p.type==='custom_tool_call_output'||p.type==='function_call_output'){const out=p.output;outputs.set(p.call_id,typeof out==='string'?out:Array.isArray(out)?out.map(x=>x.text??'').join(''):JSON.stringify(out))}}return [...calls].map(([id,input])=>({input,output:outputs.get(id)??''}))}
const worktreeOf=text=>text.match(/Isolation: worktree path="([^"]+)" branch=(\S+) base=(\S+)/)
const shell=(cmd,marker)=>`Authorized isolation acceptance test ${marker}. Using your shell tool, run exactly this one command once, unchanged, in your current working directory: ${cmd}\nReport its exit status and output. Do not retry, request escalation, or use any other tool.`
const results={}
try{
 const init=await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'agentx-isolation-live-smoke',version:'1'}})
 assert(init.instructions.includes('Isolation: worktree supported')&&init.instructions.includes('Remote: rejected'),'initialization does not expose isolation support')
 const tools=await rpc('tools/list')
 assert(tools.tools[0].inputSchema.properties.isolation.description.includes('remote: rejected'),'tool schema does not describe isolation support')
 const probe=async(id,model,cmd,write)=>{const marker=`[isolation ${id} ${randomUUID()}]`;const report=await call('ExternalSubagent',{description:`isolation ${id}`,prompt:shell(cmd,marker),model,isolation:'worktree',...(write?{write:true}:{}),run_in_background:false});const thread=threadFor(marker);assert(thread,`${id}: no native thread: ${report.text}`);const path=rollout(thread);const attempts=toolCalls(path).filter(c=>c.input.includes('printf'));assert(attempts.length>=1,`${id}: rollout ${path} shows no attempt`);return {report,thread,rollout:path,attempt:attempts[0].input.slice(0,300),output:attempts.map(a=>a.output).join('\n').slice(0,400),wt:worktreeOf(report.text),native:nativeRun(marker)}}
 const escapeTarget=join(cwd,'escape.txt'),variantTarget=join(repoAlias,'sub','variant-escape.txt')
 assert(existsSync(join(repoAlias,'sub','tracked.txt')),'case-variant spelling does not name the repository on this filesystem')
 const [write,escape,readonly,remote,failure,variant,dotgit]=await Promise.all([
  probe('write','iso','printf isolated > agent.txt',true),
  probe('escape','iso',`printf escaped > ${escapeTarget}`,true),
  probe('read-only','iso','printf readonly > ro.txt',false),
  call('ExternalSubagent',{description:'remote',prompt:'Reply ok',model:'iso',isolation:'remote',run_in_background:false}),
  call('ExternalSubagent',{description:'failure retention',prompt:'Reply ok',model:'iso-badmodel',isolation:'worktree',run_in_background:false}),
  probe('variant','iso',`printf escaped > ${variantTarget}`,true),
  probe('dotgit','iso','printf broken > ../.git',true),
 ])
 const rejectedRoots=await call('ExternalSubagent',{description:'roots with isolation',prompt:'Reply ok',model:'iso',isolation:'worktree',write:true,writable_roots:[repoAlias]})
 assert(rejectedRoots.failed&&rejectedRoots.text.includes('the worktree is the only writable root'),`caller roots accepted with isolation: ${rejectedRoots.text}`)
 results.callerRootsWithIsolation=rejectedRoots.text
 // Success: the write lands in the worktree, the Codex sandbox roots are the worktree, the original is untouched.
 assert(!write.report.failed&&write.wt,`write: ${write.report.text}`)
 const wtPath=write.wt[1]
 assert(read(join(wtPath,'sub','agent.txt'))==='isolated'&&read(join(cwd,'agent.txt'))===null,'write: file not in worktree only')
 assert(!write.output.includes('operation not permitted'),`write: denied ${write.output}`)
 assert(write.native.input.cwd===join(wtPath,'sub')&&write.native.input.args.join(' ').includes(`writable_roots=["${wtPath}"]`)&&!write.native.input.args.join(' ').includes(`"${repo}"`),`write: sandbox not bound to worktree: ${JSON.stringify(write.native.input)}`)
 assert(write.report.text.includes('?? sub/agent.txt')&&write.report.text.includes('kept after completion, failure and cancellation'),`write: report lacks remaining state: ${write.report.text}`)
 assert(git(wtPath,'branch','--show-current')===write.wt[2],'write: branch mismatch')
 results.success={worktree:wtPath,branch:write.wt[2],base:write.wt[3],thread:write.thread,rollout:write.rollout,attempt:write.attempt,runtimeOutput:write.output,nativeCwd:write.native.input.cwd,filesystem:read(join(wtPath,'sub','agent.txt'))}
 // The original checkout is outside the sandbox: an absolute write there is an OS denial.
 assert(!escape.report.failed&&escape.output.includes('operation not permitted')&&read(escapeTarget)===null,`escape: ${escape.output}`)
 results.escapeToOriginal={target:escapeTarget,thread:escape.thread,rollout:escape.rollout,attempt:escape.attempt,runtimeOutput:escape.output,filesystem:null}
 const variantArgs=variant.native.input.args.join(' ')
 const rootsArg=variantArgs.match(/sandbox_workspace_write\.writable_roots=(\[[^\]]*\])/)
 assert(!variant.report.failed&&rootsArg&&JSON.stringify(JSON.parse(rootsArg[1]))===JSON.stringify([variant.wt[1]])&&!variantArgs.toLowerCase().includes(repo.toLowerCase()),`variant: sandbox args carry more than the worktree: ${variantArgs}`)
 assert(variant.report.text.includes(`Write scope: worktree only ["${variant.wt[1]}"]`),`variant: report ${variant.report.text}`)
 assert(variant.output.includes('operation not permitted')&&read(join(cwd,'variant-escape.txt'))===null,`variant: ${variant.output}`)
 // The worktree's own .git file stays protected by Codex inside the writable root.
 const dotgitFile=join(dotgit.wt[1],'.git')
 assert(!dotgit.report.failed&&dotgit.output.includes('operation not permitted')&&read(dotgitFile).startsWith('gitdir: '),`dotgit: ${dotgit.output}`)
 results.worktreeDotGit={target:dotgitFile,thread:dotgit.thread,rollout:dotgit.rollout,attempt:dotgit.attempt,runtimeOutput:dotgit.output,filesystem:read(dotgitFile).trim()}
 results.caseVariantTarget={sandboxRoots:JSON.parse(rootsArg[1]),target:variantTarget,thread:variant.thread,rollout:variant.rollout,attempt:variant.attempt,runtimeOutput:variant.output,filesystem:null}
 assert(!readonly.report.failed&&readonly.output.includes('operation not permitted')&&read(join(readonly.wt[1],'sub','ro.txt'))===null,`read-only: ${readonly.output}`)
 results.readOnlyWorktree={worktree:readonly.wt[1],thread:readonly.thread,rollout:readonly.rollout,runtimeOutput:readonly.output}
 // Remote never runs, locally or otherwise.
 assert(remote.failed&&remote.text.includes('no cancel command')&&remote.text.includes('nothing runs locally'),remote.text)
 results.remote=remote.text
 // Native failure keeps the worktree and names it.
 const failed=worktreeOf(failure.text)
 assert(failure.failed&&failed&&existsSync(failed[1])&&failure.text.includes('kept after completion, failure and cancellation'),`failure: ${failure.text}`)
 results.failure={worktree:failed[1],report:failure.text.slice(0,1500)}
 // Cancellation after partial output keeps the worktree and its uncommitted file.
 const marker=`[isolation cancel ${randomUUID()}]`
 const started=await call('ExternalSubagent',{description:'cancel retention',prompt:shell('printf partial > partial.txt && sleep 240',marker),model:'iso',isolation:'worktree',write:true,name:'cancel-me'})
 const cancelWt=worktreeOf(started.text)
 assert(!started.failed&&cancelWt,started.text)
 const partial=join(cancelWt[1],'sub','partial.txt')
 for(let i=0;i<180&&read(partial)===null;i++)await sleep(1000)
 assert(read(partial)==='partial','cancel: partial output never appeared')
 const stopped=await call('StopExternalSubagent',{task_id:'cancel-me'})
 assert(!stopped.failed&&stopped.text.includes(': cancelled;')&&stopped.text.includes('?? sub/partial.txt')&&read(partial)==='partial',`cancel: ${stopped.text}`)
 results.cancellation={worktree:cancelWt[1],report:stopped.text.slice(0,1500),filesystem:read(partial)}
 // Pi read-only: only the working directory changes.
 const piMarker=`[isolation pi ${randomUUID()}]`
 const pi=await call('ExternalSubagent',{description:'pi isolation',prompt:`Isolation acceptance ${piMarker}. Use ls on your current directory and list the file names you see. Do not write anything.`,model:'iso-pi',isolation:'worktree',run_in_background:false})
 const piWt=worktreeOf(pi.text)
 assert(!pi.failed&&piWt&&nativeRun(piMarker).input.cwd===join(piWt[1],'sub')&&pi.text.includes('tracked.txt'),`pi: ${pi.text}`)
 results.pi={worktree:piWt[1],nativeCwd:nativeRun(piMarker).input.cwd}
 const after=snapshot()
 assert(after===before,`original checkout changed:\n${before}\n---\n${after}`)
 results.original={dirtyStatePreserved:true,status:git(repo,'status','--porcelain','--untracked-files=all'),trackedEdit:read(join(cwd,'tracked.txt'))}
 const gitCalls=nativeCalls().filter(e=>e.input.executable==='git')
 assert(gitCalls.length>0&&gitCalls.every(e=>'output' in e&&'error_output' in e),'git calls missing from debug log')
 results.gitCallsLogged=gitCalls.length
}finally{clearTimeout(timer);child.stdin.end();await new Promise(r=>child.on('close',r));await writeFile(join(base,'server-debug.log'),stderr)}
const list=git(repo,'worktree','list','--porcelain')
results.worktreesAfterServerExit=list.split('\n').filter(l=>l.startsWith('worktree ')).map(l=>l.slice(9))
assert(results.worktreesAfterServerExit.length===9,`worktrees not all retained: ${list}`)
console.log(JSON.stringify({passed:true,versions:{codex:spawnSync('codex',['--version'],{encoding:'utf8'}).stdout.trim(),pi:spawnSync('pi',['--version'],{encoding:'utf8'}).stdout.trim(),git:spawnSync('git',['--version'],{encoding:'utf8'}).stdout.trim()},scope:'real Codex/Pi through AgentX MCP with isolation=worktree; dirty original checkout snapshotted before/after; attempts and denials read from Codex rollouts; worktree filesystem state checked',results,evidence:base},null,2))
