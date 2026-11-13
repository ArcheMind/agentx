import {spawn, execFileSync} from 'node:child_process'
import {mkdtemp, mkdir, writeFile, readFile, appendFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join, resolve} from 'node:path'

for (const agent of ['agy']) {
const evidenceDir=await mkdtemp(join(tmpdir(),`sessionman-duplex-live-${agent}-`))
console.log(JSON.stringify({agent,evidence:evidenceDir,phase:'starting'}))
await mkdir(evidenceDir,{recursive:true})
const log=async(name,data)=>appendFile(join(evidenceDir,name),(typeof data==='string'?data:JSON.stringify(data,null,2))+'\n')
const binary=resolve(process.argv[2])
const home=await mkdtemp(join(tmpdir(),'agentx-live-duplex-'))
const work=await mkdtemp(join(tmpdir(),'agentx-live-cwd-'))
const dir=join(home,'.sessionman','presets','duplex-live');await mkdir(dir,{recursive:true})
await writeFile(join(dir,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${process.env.SM_AGY_MODEL || 'gemini-3.8-flash-low'}\n---\n\nDuplex live acceptance. Follow instructions exactly; keep replies to one short line.\n`)
const env={...process.env,SM_SUBAGENT_HOME:home,SM_LOG:'debug'}
const child=spawn(binary,['mcp','serve'],{cwd:work,env,stdio:['pipe','pipe','pipe']})
let pending='',counter=0
const requests=new Map()
child.stderr.on('data',c=>log('serve-stderr.log',c.toString()).catch(()=>{}))
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);log('mcp.jsonl',line).catch(()=>{});const event=JSON.parse(line);if(event.id!==undefined){const r=requests.get(event.id);requests.delete(event.id);event.error?r.reject(Error(JSON.stringify(event.error))):r.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++counter;requests.set(id,{resolve,reject});const line=JSON.stringify({jsonrpc:'2.0',id,method,params});log('mcp.jsonl','>> '+line).catch(()=>{});child.stdin.write(line+'\n')})}
async function call(name,args={}){const r=await rpc('tools/call',{name,arguments:args});const text=r.content.map(x=>x.text).join('\n');if(r.isError)throw Error(text);return text}
function assert(value,message){if(!value)throw Error('ASSERT FAILED: '+message)}
function armWait(id,seq){return new Promise((resolve,reject)=>{const p=spawn(binary,['wait',id,'--after',String(seq)],{env});let out='';p.stdout.on('data',c=>out+=c);p.stderr.on('data',c=>out+=c);p.on('close',code=>{log('waits.log',`--- wait --after ${seq} (exit ${code}) ---\n`+out).catch(()=>{});code===0?resolve(out):reject(Error('wait exited '+code+': '+out))})})}
const rearm=text=>{const m=text.match(/sm wait (ax-[a-f0-9]+) --after (\d+)/);assert(m,'no wait command: '+text);return {id:m[1],seq:Number(m[2])}}
const seqOf=text=>Number(text.match(/Re-arm: sm wait ax-[a-f0-9]+ --after (\d+)/)[1])
const pidAlive=pid=>{try{process.kill(pid,0);return true}catch{return false}}
const sleep=ms=>new Promise(r=>setTimeout(r,ms))
const verdict={}
const timer=setTimeout(()=>{console.error('GLOBAL TIMEOUT');child.kill('SIGTERM');process.exit(1)},900000)
try{
 await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'duplex-live-acceptance',version:'1'}})
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n')

 // (a) long task, steer mid-run, consumed within the same turn
 const started=await call('ExternalSubagent',{description:'live steer test',prompt:"Use the Bash tool to run exactly this foreground command once: python3 -c 'import time; time.sleep(25); print(\"slept\")' . Do NOT run it in background. When it finishes, reply with a single line that starts with DONE and also contains every extra marker you were asked for during this turn.",name:'live',model:'duplex-live',run_in_background:true})
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

 // (b)+(c) second round: MessageHost wakes the wait while the task still runs, then report
 const arm2={id:arm1.id,seq:seqOf(woke1)}
 const wait2=armWait(arm2.id,arm2.seq)
 const round2=await call('SendExternalSubagentMessage',{to:'live',message:`New task. First call the MCP tool MessageHost (the tool whose name ends in MessageHost) with the message 'starting-sleep-now'. Then use Bash to run in foreground: python3 -c 'import time; time.sleep(20)' . When it finishes reply with the single line ROUND2-DONE.`})
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

 const queuedRound=await call('SendExternalSubagentMessage',{to:'live',message:'Use your terminal tool to run a foreground sleep for 8 seconds, then reply QUEUE-FIRST.'})
 const queuedArm={id:arm1.id,seq:seqOf(woke3)}
 const queuedWait=armWait(queuedArm.id,queuedArm.seq)
 const queuedAck=await call('SendExternalSubagentMessage',{to:'live',message:'Reply exactly QUEUE-SECOND. Do not use tools.',delivery:'queued'})
 assert(queuedAck.includes('disposition: queued'),'queued message not held: '+queuedAck)
 const queuedFirst=await queuedWait
 assert(queuedFirst.includes('QUEUE-FIRST')&&!queuedFirst.includes('QUEUE-SECOND'),'queued message leaked into first round: '+queuedFirst)
 const queuedSecond=await armWait(queuedArm.id,seqOf(queuedFirst))
 assert(queuedSecond.includes('QUEUE-SECOND'),'queued second round missing: '+queuedSecond)
 verdict.queue='PASS: next round starts only after the active round settles'


 // (d) stop mid-run: cancelled report, daemon gone, no orphan native processes
 const psBefore=()=>execFileSync('ps',['-axo','pid=,ppid=,command='],{encoding:'utf8'})
 const round3=await call('SendExternalSubagentMessage',{to:'live',message:"New task: use Bash to run in foreground: python3 -c 'import time; time.sleep(120)' . Reply NEVER when done."})
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
 child.stdin.end()
 await new Promise(resolve=>child.once('close',resolve))
}
}
