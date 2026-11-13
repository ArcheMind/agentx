import {spawn, spawnSync, execFileSync} from 'node:child_process'
import {mkdtemp, mkdir, writeFile, readFile, appendFile, readdir, access, copyFile, realpath, cp, rm} from 'node:fs/promises'
import {tmpdir, homedir} from 'node:os'
import {randomUUID} from 'node:crypto'
import {join, resolve} from 'node:path'

for (const agent of ['agy']) {
const evidenceDir=await mkdtemp(join(tmpdir(),`sessionman-duplex-live-${agent}-`))
console.log(JSON.stringify({agent,evidence:evidenceDir,phase:'starting'}))
await mkdir(evidenceDir,{recursive:true})
const log=async(name,data)=>appendFile(join(evidenceDir,name),(typeof data==='string'?data:JSON.stringify(data,null,2))+'\n')
const binary=resolve(process.argv[2])
const home=await mkdtemp(join(tmpdir(),'agentx-live-duplex-'))
const work=await realpath(await mkdtemp(join(tmpdir(),'agentx-live-cwd-')))
const dir=join(home,'.sessionman','presets','duplex-live');await mkdir(dir,{recursive:true})
await writeFile(join(dir,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${process.env.SM_AGY_MODEL || 'gemini-3.8-flash-low'}\n---\n\nDuplex live acceptance. Follow instructions exactly; keep replies to one short line.\n`)
const normalDir=join(home,'.sessionman','presets','duplex-normal');await mkdir(normalDir,{recursive:true})
await writeFile(join(normalDir,'PRESET.md'),`---\nagent: agy\nmodel: ${process.env.SM_AGY_MODEL || 'gemini-3.8-flash-low'}\nmode: normal\n---\n\nNative normal permission acceptance.\n`)
const env={...process.env,SM_SUBAGENT_HOME:home,SM_LOG:'debug'}
const agentDir=id=>join(home,'.local','state','sessionman','agents',id)
const meta=async id=>JSON.parse(await readFile(join(agentDir(id),'meta.json'),'utf8'))
const events=async id=>(await Promise.all((await readdir(join(agentDir(id),'events'))).filter(x=>x.endsWith('.json')).map(async file=>JSON.parse(await readFile(join(agentDir(id),'events',file),'utf8'))))).sort((a,b)=>a.seq-b.seq)
const nativeLog=async id=>(await readFile(join(agentDir(id),'daemon.log'),'utf8')).split('\n').flatMap(line=>{try{return [JSON.parse(line)]}catch{return []}})
const nativeMessages=async id=>(await nativeLog(id)).filter(x=>x.event==='stream_line').map(x=>({direction:x.direction,...JSON.parse(x.line)}))
const checkpoint=async(phase)=>{console.log(JSON.stringify({agent,phase,evidence:evidenceDir}));await log('progress.jsonl',{phase})}
const child=spawn(binary,['mcp','serve'],{cwd:work,env,stdio:['pipe','pipe','pipe']})
let pending='',counter=0
const requests=new Map()
const waiters=new Set()
child.stderr.on('data',c=>log('serve-stderr.log',c.toString()).catch(()=>{}))
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);log('mcp.jsonl',line).catch(()=>{});const event=JSON.parse(line);if(event.id!==undefined){const r=requests.get(event.id);requests.delete(event.id);event.error?r.reject(Error(JSON.stringify(event.error))):r.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++counter;requests.set(id,{resolve,reject});const line=JSON.stringify({jsonrpc:'2.0',id,method,params});log('mcp.jsonl','>> '+line).catch(()=>{});child.stdin.write(line+'\n')})}
async function call(name,args={}){const r=await rpc('tools/call',{name,arguments:args});const text=r.content.map(x=>x.text).join('\n');if(r.isError)throw Error(text);return text}
function assert(value,message){if(!value)throw Error('ASSERT FAILED: '+message)}
function armWait(id,seq){return new Promise((resolve,reject)=>{const p=spawn(binary,['wait',id,'--after',String(seq)],{env});waiters.add(p);let out='';p.stdout.on('data',c=>out+=c);p.stderr.on('data',c=>out+=c);p.on('close',code=>{waiters.delete(p);log('waits.log',`--- wait --after ${seq} (exit ${code}) ---\n`+out).catch(()=>{});code===0?resolve(out):reject(Error('wait exited '+code+': '+out))})})}
const rearm=text=>{const m=text.match(/sm wait (ax-[a-f0-9]+) --after (\d+)/);assert(m,'no wait command: '+text);return {id:m[1],seq:Number(m[2])}}
const seqOf=text=>Number(text.match(/Re-arm: sm wait ax-[a-f0-9]+ --after (\d+)/)[1])
const pidAlive=pid=>{try{process.kill(pid,0);return true}catch{return false}}
const sleep=ms=>new Promise(r=>setTimeout(r,ms))
const verdict={}
const ownedPlugin=join(homedir(),'.gemini','config','plugins','sessionman')
const ownership='sessionman native agy plugin\n'
let pluginSnapshot=null
let pluginTouched=false
let installerProcess=null
let coexistNativeIDs=[]
async function productExec(){
 const result=await new Promise((resolve,reject)=>{
  const process=spawn(binary,['agent','exec','agy','--cwd',work,'--model',globalThis.process.env.SM_AGY_MODEL||'gemini-3.8-flash-low'],{cwd:work,env,stdio:['pipe','pipe','pipe']})
  installerProcess=process
  let stdout='',stderr=''
  process.stdout.on('data',chunk=>stdout+=chunk)
  process.stderr.on('data',chunk=>stderr+=chunk)
  process.on('error',reject)
  process.on('close',code=>resolve({code,stdout,stderr}))
  process.stdin.end('Do not use tools. Reply exactly PLUGIN-INSTALLED.')
 })
 await log('coexist-product-exec.json',result)
 assert(result.code===0&&result.stdout.includes('PLUGIN-INSTALLED'),'product agent exec did not install plugin successfully: '+JSON.stringify(result))
 assert(result.stdout.trim().split('\n').map(JSON.parse).some(x=>x.event==='result'&&x.result?.status==='SUCCESS'),'product exec lacked native NDJSON success')
}

const terminalWait='If the terminal tool returns an asynchronous/running command ID, keep calling its status/wait tool until the command exits. Do not finish your turn or describe waiting while the command is still running.'

const timer=setTimeout(()=>{console.error('GLOBAL TIMEOUT');for(const request of requests.values())request.reject(Error('GLOBAL TIMEOUT'));requests.clear();for(const waiter of waiters)waiter.kill('SIGTERM');installerProcess?.kill('SIGTERM');child.kill('SIGTERM')},900000)
try{
 await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'duplex-live-acceptance',version:'1'}})
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n')

 if(process.env.SM_AGY_COEXIST==='1'){
  let existed=true
  try{await access(ownedPlugin)}catch(error){if(error.code==='ENOENT')existed=false;else throw error}
  if(existed){
   assert(await readFile(join(ownedPlugin,'.sessionman-owned'),'utf8')===ownership,'refusing foreign plugin')
   pluginSnapshot=join(evidenceDir,'original-owned-plugin')
   await cp(ownedPlugin,pluginSnapshot,{recursive:true})
  }
  pluginTouched=true
  await productExec()
  assert(await readFile(join(ownedPlugin,'.sessionman-owned'),'utf8')===ownership,'installed plugin has no owned marker')
  await log('coexist-installed-plugin.json',{manifest:JSON.parse(await readFile(join(ownedPlugin,'mcp_config.json'),'utf8'))})
  const parentToken='PARENT-'+randomUUID(),childToken='CHILD-'+randomUUID(),answerToken='NESTED-'+randomUUID()
  const prompt=`First call EACH distinct available SessionMan MessageHost tool once with message ${parentToken}. Both server namespaces may be offered; exercise both if offered. Then call the SessionMan ExternalSubagent tool whose full name contains sessionman twice if offered (otherwise choose the offered one), with model "duplex-live", name "inner", description "coexist nested acceptance", run_in_background false, prompt "Call EACH distinct available MessageHost tool exactly once with message ${childToken}, including both server namespaces if offered. Then reply exactly ${answerToken}. Do not use other tools." Include its completed report verbatim in your final answer.`
  const accepted=await call('ExternalSubagent',{description:'coexisting native MCP servers',name:'coexist',model:'duplex-live',prompt,run_in_background:true})
  const arm=rearm(accepted)
  let seq=arm.seq,report
  while(!report){const wake=await armWait(arm.id,seq);seq=seqOf(wake);if(wake.includes('kind=report'))report=wake}
  assert(report.includes('Report status: completed.')&&report.includes(answerToken),'coexist parent did not complete native nested turn: '+report)
  const identities=await Promise.all((await readdir(join(home,'.local','state','sessionman','agents'))).map(async id=>({id,...await meta(id)})))
  const parent=identities.find(x=>x.id===arm.id),nested=identities.find(x=>x.name==='inner')
  assert(parent?.cwd===work&&parent.delegation_depth===0&&nested?.cwd===work&&nested.delegation_depth===1,'coexist identity/cwd/depth mismatch: '+JSON.stringify(identities))
  const parentEvents=await events(parent.id),childEvents=await events(nested.id)
  assert(parentEvents.filter(x=>x.kind==='message'&&x.message===parentToken).length>=2,'both parent server paths did not deliver MessageHost')
  assert(childEvents.filter(x=>x.kind==='message'&&x.message===childToken).length>=2,'both nested server paths did not deliver MessageHost')
  assert(!parentEvents.some(x=>x.message===childToken)&&!childEvents.some(x=>x.message===parentToken),'coexistent native server routed MessageHost to another layer')
  const observed=[]
  for(const identity of [parent,nested]){
   const messages=await nativeMessages(identity.id)
   const names=[...new Set(messages.filter(x=>x.direction==='out'&&x.method==='session/update').map(x=>x.params?.update?.name).filter(x=>x?.includes('MessageHost')))]
   const start=(await nativeLog(identity.id)).find(x=>x.event==='stream_start')
   assert(start?.input.env.SM_MCP_CWD===work&&start.input.env.SM_HOST_AGENT_DIR===agentDir(identity.id)&&start.input.env.SM_DELEGATION_DEPTH===String(identity.delegation_depth+1),'coexist native plugin inherited wrong child environment')
   assert(names.some(x=>(x.match(/sessionman/g)||[]).length>=2)&&names.some(x=>(x.match(/sessionman/g)||[]).length===1),'did not observe calls through both actually exposed SessionMan namespaces: '+JSON.stringify(names))
   observed.push({id:identity.id,names,start})
  }
  await log('coexist.json',{identities,parentEvents,childEvents,observed,report})
  verdict.coexist='PASS: product exec installs owned plugin; global and temporary native server namespaces both route parent/nested MessageHost to correct layer/cwd/depth'
  coexistNativeIDs=[parent.session,nested.session]
  continue
 }


 // (a) long task, steer mid-run, consumed within the same turn
 const started=await call('ExternalSubagent',{description:'live steer test',prompt:terminalWait+" Use your terminal tool to run exactly this command once: python3 -c 'import time; time.sleep(25); print(\"slept\")' . Do NOT run it in background. When it finishes, reply with a single line that starts with DONE and also contains every extra marker you were asked for during this turn.",name:'live',model:'duplex-live',run_in_background:true})
 await log('a-start.txt',started)
 const arm1=rearm(started)
 const wait1=armWait(arm1.id,arm1.seq)
 await sleep(8000) // the 25s sleep is running now
 const steered=await call('SendExternalSubagentMessage',{to:'live',message:'Additional instruction for the task you are running right now: include the marker MARKER-ALPHA-42 in your final DONE line.'})
 await log('a-send.txt',steered)
 assert(steered.includes('disposition: steered'),'(a) not steered: '+steered)
 const woke1=await wait1
 assert(woke1.includes('kind=report')&&woke1.includes('Report status: completed.'),'(a) no completed report: '+woke1)
 assert(woke1.includes('MARKER-ALPHA-42')&&woke1.includes('DONE'),'(a) steer not consumed in the same turn: '+woke1)
 verdict.a='PASS: steer delivered mid-turn, single report contains DONE + MARKER-ALPHA-42'
 const firstReport=(await events(arm1.id)).find(x=>x.kind==='report')
 const prompts=firstReport.usage?.prompts
 assert(Array.isArray(prompts)&&prompts.length===2,'steered usage must retain both native RPCs: '+JSON.stringify(firstReport))
 assert(new Set(prompts.map(x=>x.id)).size===2,'usage RPC identifiers are not distinct')
 const messages=await nativeMessages(arm1.id)
 for(const prompt of prompts){
  const native=messages.find(x=>x.direction==='out'&&x.id===prompt.id&&x.result?.usage)
  assert(native,'native usage response missing for RPC '+prompt.id)
  assert(JSON.stringify(native.result.usage)===JSON.stringify(prompt.usage),'raw native usage changed for RPC '+prompt.id)
  assert(prompt.stopReason===native.result.stopReason,'native terminal reason changed')
  assert(Object.values(prompt.usage).some(x=>typeof x==='number'&&x>0),'native RPC usage contained no positive counters')
 }
 await log('a-native-usage.json',{report:firstReport.usage,native:messages.filter(x=>prompts.some(p=>p.id===x.id))})
 verdict.usage='PASS: both prompt RPCs retain exact native usage and terminal reason'
 await checkpoint('steer-and-native-usage-passed')

 // (b)+(c) second round: MessageHost wakes the wait while the task still runs, then report
 const arm2={id:arm1.id,seq:seqOf(woke1)}
 const wait2=armWait(arm2.id,arm2.seq)
 const round2=await call('SendExternalSubagentMessage',{to:'live',message:`${terminalWait} New task. First call the MCP tool MessageHost (the tool whose name ends in MessageHost) with the message 'starting-sleep-now'. Then use Bash to run in foreground: python3 -c 'import time; time.sleep(20)' . When it finishes reply with the single line ROUND2-DONE.`})
 await log('b-send.txt',round2)
 assert(round2.includes('disposition: started'),'(b) second round not started: '+round2)
 const woke2=await wait2
 assert(woke2.includes('kind=message')&&woke2.includes('starting-sleep-now'),'(b) MessageHost event did not wake wait: '+woke2)
 const during=await call('ListExternalSubagentSessions')
 await log('b-list-during.txt',during)
 assert(during.includes('ExternalSubagent status: running'),'(b) agent not running when message arrived: '+during)
 verdict.b='PASS: message event woke sm wait while run state was running'
 const wait3=armWait(arm2.id,seqOf(woke2))
 const woke3=await wait3
 assert(woke3.includes('kind=report')&&woke3.includes('ROUND2-DONE'),'(c) re-armed wait missed second-round report: '+woke3)
 verdict.c='PASS: report event delivered; re-armed wait survived the second round'

 const queuedRound=await call('SendExternalSubagentMessage',{to:'live',message:terminalWait+' Use your terminal tool to run python3 -c \"import time; time.sleep(8)\" and wait until it exits, then reply exactly QUEUE-FIRST.'})
 const queuedArm={id:arm1.id,seq:seqOf(woke3)}
 const queuedWait=armWait(queuedArm.id,queuedArm.seq)
 const queuedAck=await call('SendExternalSubagentMessage',{to:'live',message:'Reply exactly QUEUE-SECOND. Do not use tools.',delivery:'queued'})
 assert(queuedAck.includes('disposition: queued'),'queued message not held: '+queuedAck)
 const queuedFirst=await queuedWait
 assert(queuedFirst.includes('QUEUE-FIRST')&&!queuedFirst.includes('QUEUE-SECOND'),'queued message leaked into first round: '+queuedFirst)
 const queuedSecond=await armWait(queuedArm.id,seqOf(queuedFirst))
 assert(queuedSecond.includes('QUEUE-SECOND'),'queued second round missing: '+queuedSecond)
 verdict.queue='PASS: next round starts only after the active round settles'

 await checkpoint('checking-native-nested-delegation')
 const nestedToken='NESTED-'+randomUUID()
 const nestedHostToken='HOST-'+randomUUID()
 const nestedStarted=await call('SendExternalSubagentMessage',{to:'live',message:`Call the SessionMan MCP tool ExternalSubagent exactly once with model "duplex-live", name "inner", description "native nested acceptance", run_in_background false, and prompt "First call the SessionMan MCP tool MessageHost with message ${nestedHostToken}. Then reply exactly ${nestedToken}. Do not use other tools." Wait for its report and include its exact report in your answer. Do not produce the token yourself without calling ExternalSubagent.`})
 const nestedReport=await armWait(arm1.id,seqOf(queuedSecond))
 assert(nestedReport.includes(nestedToken),'native nested report missing: '+nestedReport)
 const metas=await Promise.all((await readdir(join(home,'.local','state','sessionman','agents'))).map(async id=>({id,...await meta(id)})))
 const nested=metas.find(x=>x.id!==arm1.id&&x.name==='inner')
 assert(nested&&nested.delegation_depth===1&&nested.cwd===work,'nested native task has wrong depth/cwd: '+JSON.stringify(metas))
 const nestedEvents=await events(nested.id)
 assert(nestedEvents.some(x=>x.kind==='message'&&x.message===nestedHostToken),'nested MessageHost did not reach its own agent event stream: '+JSON.stringify(nestedEvents))
 assert(nestedEvents.some(x=>x.kind==='report'&&x.status==='completed'&&x.report.includes(nestedToken)),'nested agent did not produce native completed report')
 const outerEvents=await events(arm1.id)
 assert(!outerEvents.some(x=>x.kind==='message'&&x.message===nestedHostToken),'nested MessageHost leaked to parent identity')
 const nestedStart=(await nativeLog(nested.id)).find(x=>x.event==='stream_start')
 assert(nestedStart?.input?.env?.SM_MCP_CWD===work&&nestedStart.input.env.SM_HOST_AGENT_DIR===agentDir(nested.id)&&nestedStart.input.env.SM_DELEGATION_DEPTH==='2','nested plugin environment did not match child identity: '+JSON.stringify(nestedStart))
 await log('nested.json',{nested,nestedEvents,nestedStart})
 verdict.nested='PASS: real nested native turn and MessageHost use child identity/cwd/depth'
 const nativeToolNames=[...new Set((await nativeMessages(arm1.id)).filter(x=>x.direction==='out'&&x.method==='session/update').map(x=>x.params?.update?.name).filter(x=>x?.includes('sessionman')))]
 await log('observed-native-mcp-names.json',nativeToolNames)

 await checkpoint('checking-native-normal-permissions')
 const normalStarted=await call('ExternalSubagent',{description:'native normal acceptance',prompt:'Use your terminal tool to write the text NORMAL-WRITE into native-normal.txt in the current working directory. If native permissions deny it, do not retry or bypass them. Finish with a short description of the native outcome.',name:'normal',model:'duplex-normal',run_in_background:true})
 const normalArm=rearm(normalStarted)
 const normalReport=await armWait(normalArm.id,normalArm.seq)
 const normalEvent=(await events(normalArm.id)).filter(x=>x.kind==='report').at(-1)
 const normalLog=await nativeLog(normalArm.id)
 const normalStart=normalLog.find(x=>x.event==='stream_start')
 const yoloStart=(await nativeLog(arm1.id)).find(x=>x.event==='stream_start')
 assert(normalStart&&!normalStart.input.args.includes('--dangerously-skip-permissions'),'normal missing native start or enabled bypass')
 assert(yoloStart?.input?.args.includes('--dangerously-skip-permissions'),'yolo omitted native bypass')
 assert(!(await nativeMessages(normalArm.id)).some(x=>x.method==='session/set_mode'),'normal overwrote native permission mode')
 const normalMessages=await nativeMessages(normalArm.id)
 const permissionRequests=normalMessages.filter(x=>x.direction==='out'&&x.method==='session/request_permission')
 let normalFile=null
 try{normalFile=await readFile(join(work,'native-normal.txt'),'utf8')}catch(error){if(error.code!=='ENOENT')throw error}
 if(permissionRequests.length){
  for(const request of permissionRequests)assert(normalMessages.some(x=>x.direction==='in'&&x.id===request.id&&x.result?.outcome?.outcome==='cancelled'),'normal permission request did not receive native cancelled response')
  assert(normalFile===null,'headless denied native write was executed')
  if(normalEvent?.status==='failed'){
   assert(normalEvent.failure==='ExternalSubagent exited without a complete report; no implicit retry','normal failure is not the exact native no-report outcome: '+normalReport)
   assert(normalMessages.some(x=>x.direction==='out'&&x.result?.stopReason==='end_turn'),'no-report denied turn lacked native end_turn')
   assert(!normalMessages.some(x=>x.direction==='out'&&x.params?.update?.sessionUpdate==='agent_message_chunk'&&x.params.update.content?.text),'denied no-report turn unexpectedly had assistant text')
  }else assert(['completed','cancelled'].includes(normalEvent?.status),'denied normal turn has unexpected status: '+normalReport)
 }else{
  assert(normalEvent?.status==='completed'&&normalFile==='NORMAL-WRITE','native normal configuration neither requested permission nor completed the write: '+normalReport)
 }
 await log('normal-yolo.json',{normalEvent,normalStart,yoloStart,permissionRequests,normalFile})
 verdict.modes='PASS: normal retains native policy; requested headless approvals cancel without executing the write; denied native end_turn without text remains an explicit failed report; yolo uses native bypass'


 await checkpoint('recording-native-cold-recovery-token')
 const coldToken='COLD-'+randomUUID()
 await call('SendExternalSubagentMessage',{to:'live',message:`Remember this exact cold recovery token for a later task: ${coldToken}. Reply exactly STORED, without using tools.`})
 const stored=await armWait(arm1.id,seqOf(nestedReport))
 assert(stored.includes('STORED'),'cold token store failed: '+stored)
 const beforeCold=await meta(arm1.id)
 assert(beforeCold.session,'native session ID missing before cold recovery')
 await checkpoint('waiting-for-idle-daemon-to-exit')
 const coldDeadline=Date.now()+80000
 let daemonGone=false
 while(Date.now()<coldDeadline){try{await access(join(agentDir(arm1.id),'daemon.pid'))}catch{daemonGone=true;break}await sleep(500)}
 assert(daemonGone,'idle daemon did not exit before cold recovery')
 const coldSent=await call('SendExternalSubagentMessage',{to:'live',message:'Return exactly the cold recovery token I asked you to remember in the previous task. Do not use tools.'})
 assert(coldSent.includes('disposition: started'),'cold recovery did not start')
 const recalled=await armWait(arm1.id,seqOf(stored))
 assert(recalled.includes(coldToken),'same-session cold memory retrieval failed: '+recalled)
 const afterCold=await meta(arm1.id)
 assert(afterCold.session===beforeCold.session&&afterCold.native_id===beforeCold.native_id,'cold recovery replaced native session ID')
 const coldMessages=await nativeMessages(arm1.id)
 const loads=coldMessages.filter(x=>x.direction==='in'&&x.method==='session/load')
 assert(loads.some(x=>x.params.sessionId===beforeCold.session),'product cold recovery did not issue native session/load with original ID')
 assert(coldMessages.filter(x=>x.direction==='in'&&x.method==='session/new').length===1,'cold recovery created a replacement session')
 await log('cold-recovery.json',{beforeCold,afterCold,loads,recalled})
 verdict.cold='PASS: exited daemon cold-loads original native ID and retrieves unrepeated token'
 await checkpoint('checking-native-stop-process-tree')

 // (d) stop mid-run: cancelled report, daemon gone, no orphan native processes
 const psBefore=()=>execFileSync('ps',['-axo','pid=,ppid='],{encoding:'utf8'})
 const round3=await call('SendExternalSubagentMessage',{to:'live',message:terminalWait+" New task: use your terminal tool to run: python3 -c 'import time; time.sleep(120)' . Reply NEVER when done."})
 await log('d-send.txt',round3)
 await sleep(10000)
 const before=psBefore()
 await log('d-ps-before.txt',before)
 const daemonPid=Number((await readFile(join(home,'.local','state','sessionman','agents',arm1.id,'daemon.pid'),'utf8')).trim())
 const rows=before.trim().split('\n').map(l=>{const [pid,ppid]=l.trim().split(/\s+/);return {pid:Number(pid),ppid:Number(ppid)}})
 const nativePids=[]
 const parents=new Set([daemonPid])
 for(let changed=true;changed;){changed=false;for(const r of rows)if(parents.has(r.ppid)&&!parents.has(r.pid)){parents.add(r.pid);nativePids.push(r.pid);changed=true}}
 assert(nativePids.length>0,'(d) no native child process before stop')
 const stopped=await call('StopExternalSubagent',{task_id:'live'})
 await log('d-stop.txt',stopped)
 await log('d-pids.json',{daemonPid,nativePids})
 assert(stopped.includes('Report status: cancelled.'),'(d) stop did not confirm cancellation: '+stopped)
 const deadline=Date.now()+10000
 while(Date.now()<deadline&&(nativePids.some(pidAlive)||pidAlive(daemonPid)))await sleep(200)
 const after=psBefore()
 await log('d-ps-after.txt',after)
 assert(!nativePids.some(pidAlive),'(d) orphan native agent process after stop')
 assert(!pidAlive(daemonPid),'(d) daemon still alive after stop')
 verdict.d='PASS: cancelled report, daemon exited, native process tree dead'

 await log('verdict.json',verdict)
 console.log(JSON.stringify({passed:true,agent,verdict,evidence:evidenceDir,home},null,2))
}catch(error){
 verdict.error=String(error)
 await log('verdict.json',verdict)
 console.error(error)
 process.exitCode=1
}finally{
 clearTimeout(timer)
 try {
  const ids=await readdir(join(home,'.local','state','sessionman','agents'))
  for(const id of ids){
   const target=join(evidenceDir,'agents',id)
   await mkdir(target,{recursive:true})
   for(const file of ['meta.json','daemon.log']){try{await copyFile(join(agentDir(id),file),join(target,file))}catch{}}
   try{await writeFile(join(target,'events.json'),JSON.stringify(await events(id),null,2))}catch{}
  }
 }catch{}
 child.stdin.end()
 if(child.exitCode===null&&child.signalCode===null)await new Promise(resolve=>child.once('close',resolve))
 if(pluginTouched){
  let present=true
  try{await access(ownedPlugin)}catch(error){if(error.code==='ENOENT')present=false;else throw error}
  if(present){
   assert(await readFile(join(ownedPlugin,'.sessionman-owned'),'utf8')===ownership,'refusing to restore over a foreign plugin marker')
   await rm(ownedPlugin,{recursive:true})
  }
  if(pluginSnapshot)await cp(pluginSnapshot,ownedPlugin,{recursive:true})
  await log('coexist-restored.json',{existedBefore:!!pluginSnapshot,restored:true})
 }
 if(coexistNativeIDs.length){
  assert((await readdir(join(home,'.local','state','sessionman','agents'))).length===0,'MCP sidecar agent directories remain after shutdown')
  const listed=include=>{
   const args=['--json','session','list','--source','agy','--all',...(include?['--include-subagents']:[])]
   const result=spawnSync(binary,args,{cwd:work,env,encoding:'utf8',timeout:30000,maxBuffer:32*1024*1024})
   assert(!result.error&&result.status===0,'native catalog command failed: '+result.stderr)
   return JSON.parse(result.stdout)
  }
  const hidden=listed(false),included=listed(true)
  for(const id of coexistNativeIDs){
   assert(!JSON.stringify(hidden).includes(id),'native child session remains visible without sidecar by default: '+id)
   assert(JSON.stringify(included).includes(id),'native marked session missing with --include-subagents: '+id)
  }
  await log('coexist-native-visibility.json',{ids:coexistNativeIDs,defaultHidden:true,explicitlyIncluded:true,sidecarsRemoved:true})
  verdict.visibility='PASS: native parent/nested sessions retain first-line marker and hide by default after MCP sidecar cleanup; --include-subagents reveals both'
  await log('verdict.json',verdict)
  console.log(JSON.stringify({passed:true,agent,verdict,evidence:evidenceDir},null,2))
 }
}
}
