import {spawn} from 'node:child_process'
import {mkdtemp, mkdir, writeFile, readFile, stat} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join, resolve} from 'node:path'

const home=await mkdtemp(join(tmpdir(),'sessionman-duplex-'))
const binary=resolve(process.env.SM_MCP_BINARY??process.argv[2]??'bin/sm')
const fakeLog=join(home,'fake-claude.jsonl')
await mkdir(join(home,'bin'))
// Fake streaming claude: stream-json on stdin/stdout, scripted by prompt keywords.
await writeFile(join(home,'bin','claude'),`#!/usr/bin/env node
import {createInterface} from 'node:readline'
import {appendFileSync} from 'node:fs'
import {spawn} from 'node:child_process'
const args=process.argv.slice(2)
appendFileSync(${JSON.stringify(fakeLog)},JSON.stringify({pid:process.pid,argv:args})+'\\n')
const session='dddddddd-dddd-dddd-dddd-dddddddddddd'
const emit=o=>console.log(JSON.stringify(o))
const lines=[]
let wake=null
createInterface({input:process.stdin}).on('line',l=>{lines.push(JSON.parse(l).message.content[0].text);wake?.()})
const next=async(ms)=>{if(lines.length)return lines.shift();if(ms===0)return null;await Promise.race([new Promise(r=>{wake=r}),new Promise(r=>setTimeout(r,ms))]);wake=null;return lines.shift()??null}
const result=(text)=>emit({type:'result',subtype:'success',session_id:session,is_error:false,result:text,usage:{input_tokens:3}})
async function callHost(){
 const i=args.indexOf('--mcp-config')
 const cfg=JSON.parse(args[i+1]).mcpServers.sessionman
 const child=spawn(cfg.command,cfg.args,{env:{...process.env,...cfg.env},stdio:['pipe','pipe','inherit']})
 let id=0,buf=''
 const pend=new Map()
 child.stdout.on('data',c=>{buf+=c;let e;while((e=buf.indexOf('\\n'))>=0){const line=buf.slice(0,e);buf=buf.slice(e+1);const m=JSON.parse(line);if(m.id!==undefined)pend.get(m.id)?.(m)}})
 const rpc=(method,params)=>new Promise(r=>{const n=++id;pend.set(n,r);child.stdin.write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\\n')})
 await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'fake-claude',version:'1'}})
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\\n')
 const reply=await rpc('tools/call',{name:'MessageHost',arguments:{message:'hello-from-child'}})
 child.stdin.end()
 return JSON.stringify(reply.result??reply.error)
}
for(;;){
 const prompt=await next(3600000)
 if(prompt===null)process.exit(0)
 emit({type:'system',subtype:'init',session_id:session})
 emit({type:'assistant'})
 if(prompt.includes('LONGTASK')){
  const steer=await next(8000)
  result(steer===null?'TIMEOUT-NO-STEER':'STEERED:'+steer)
 }else if(prompt.includes('CALLHOST')){
  const ack=await callHost()
  await new Promise(r=>setTimeout(r,2000))
  result('MESSAGED:'+ack)
 }else if(prompt.includes('SLEEPFOREVER')){
  await new Promise(()=>{})
 }else{
  result('ECHO:'+prompt)
 }
}
`,{mode:0o700})
const dir=join(home,'.sessionman','presets','duplex-test');await mkdir(dir,{recursive:true})
await writeFile(join(dir,'PRESET.md'),'---\nagent: claude\nmodel: opus\n---\n\nDuplex daemon integration fixture.\n')
const env={...process.env,SM_SUBAGENT_HOME:home,SM_LOG:'debug',PATH:join(home,'bin')+':'+process.env.PATH}
const child=spawn(binary,['mcp','serve'],{env,stdio:['pipe','pipe','pipe']})
let next=0,pending='',stderr=''
const requests=new Map()
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const request=requests.get(event.id);requests.delete(event.id);event.error?request.reject(Error(JSON.stringify(event.error))):request.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++next;requests.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
async function call(name,args={}){const r=await rpc('tools/call',{name,arguments:args});const text=r.content.map(x=>x.text).join('\n');if(r.isError)throw Error(text);return text}
function assert(value,message){if(!value)throw Error(message)}
function armWait(id,seq){return new Promise((resolve,reject)=>{const p=spawn(binary,['wait',id,'--after',String(seq)],{env});let out='';p.stdout.on('data',c=>out+=c);p.stderr.on('data',c=>out+=c);p.on('close',code=>code===0?resolve(out):reject(Error('wait exited '+code+': '+out)))})}
const rearm=text=>{const m=text.match(/sm wait (ax-[a-f0-9]+) --after (\d+)/);assert(m,'no wait command in: '+text);return {id:m[1],seq:Number(m[2])}}
const pidAlive=pid=>{try{process.kill(pid,0);return true}catch{return false}}
const timer=setTimeout(()=>child.kill('SIGTERM'),120000)
try{
 await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'duplex-smoke',version:'1'}})
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n')
 const tools=await rpc('tools/list')
 assert(tools.tools.length===4,'expected four MCP tools')
 assert(tools.tools[0].description.includes('sm wait'),'start tool description lacks wait contract')
 assert(tools.tools[1].inputSchema.properties.delivery,'send tool lacks delivery param')

 // (a) steer delivered mid-turn, same turn, single report
 const started=await call('StartExternalSubagent',{description:'long task',prompt:'LONGTASK step one',name:'dux',model:'duplex-test',run_in_background:true})
 const arm1=rearm(started)
 const wait1=armWait(arm1.id,arm1.seq)
 const steered=await call('SendExternalSubagentMessage',{to:'dux',message:'please-add-marker-42'})
 assert(steered.includes('disposition: steered'),steered)
 const woke1=await wait1
 assert(woke1.includes('kind=report')&&woke1.includes('STEERED:')&&woke1.includes('Message from your coordinator (via sessionman Send):')&&woke1.includes('please-add-marker-42'),'steer not consumed in-turn: '+woke1)

 // (b) MessageHost wakes the armed wait while the task still runs
 const arm2=rearm(woke1)
 const wait2=armWait(arm2.id,arm2.seq)
 const second=await call('SendExternalSubagentMessage',{to:'dux',message:'CALLHOST now'})
 assert(second.includes('disposition: started'),second)
 const woke2=await wait2
 assert(woke2.includes('kind=message')&&woke2.includes('hello-from-child'),'MessageHost event missing: '+woke2)
 const during=await call('ListExternalSubagentSessions')
 assert(during.includes('ExternalSubagent status: running'),'task not running while message arrived: '+during)

 // (c) report event after settle; re-armed wait survives into the second round
 const arm3=rearm(woke2)
 const woke3=await armWait(arm3.id,arm3.seq)
 assert(woke3.includes('kind=report')&&woke3.includes('MESSAGED:'),'second-round report missing: '+woke3)

 // (d) stop mid-run: cancelled report, daemon gone, no orphan native process
 const third=await call('SendExternalSubagentMessage',{to:'dux',message:'SLEEPFOREVER'})
 assert(third.includes('disposition: started'),third)
 await new Promise(r=>setTimeout(r,500))
 const fakes=(await readFile(fakeLog,'utf8')).trim().split('\n').map(JSON.parse)
 const daemonPid=Number((await readFile(join(home,'.local','state','sessionman','agents',arm1.id,'daemon.pid'),'utf8')).trim())
 const stopped=await call('StopExternalSubagent',{task_id:'dux'})
 assert(stopped.includes('Report status: cancelled.'),stopped)
 const deadline=Date.now()+5000
 while(Date.now()<deadline&&(fakes.some(f=>pidAlive(f.pid))||pidAlive(daemonPid)))await new Promise(r=>setTimeout(r,100))
 assert(!fakes.some(f=>pidAlive(f.pid)),'orphan fake claude process survived stop')
 assert(!pidAlive(daemonPid),'daemon survived stop')

 // connection EOF disposes agent state
 child.stdin.end()
 await new Promise(resolve=>child.once('close',resolve))
 const disposed=await stat(join(home,'.local','state','sessionman','agents',arm1.id)).then(()=>false,()=>true)
 assert(disposed,'agent dir not disposed on connection EOF')
 console.log(JSON.stringify({passed:true,binary,agent:arm1.id,evidence:home}))
}catch(error){
 console.error(error)
 console.error('stderr tail:',stderr.slice(-4000))
 process.exitCode=1
}finally{
 clearTimeout(timer)
 if(child.exitCode===null){child.stdin.end();await new Promise(resolve=>child.once('close',resolve))}
}