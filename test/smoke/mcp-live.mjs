import {spawn} from 'node:child_process'
import {mkdtemp,mkdir,writeFile} from 'node:fs/promises'
import {tmpdir,homedir} from 'node:os'
import {join,resolve} from 'node:path'
import {randomUUID} from 'node:crypto'

const home=await mkdtemp(join(tmpdir(),'agentx-mcp-live-'))
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const writePreset=async(home,{name,agent,model,effort,description})=>{const dir=join(home,'.agentx','presets',name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${model}\n${effort?`effort: ${effort}\n`:''}---\n\n${description}\n`)}
const options=[{name:'codex-test',agent:'codex',model:process.env.AX_MCP_CODEX_MODEL??'gpt-5.6-luna',description:'Read-only Codex smoke',effort:'low'},{name:'pi-test',agent:'pi',model:process.env.AX_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna',description:'Read-only Pi smoke'}]
for(const option of options)await writePreset(home,option)
const child=spawn(resolve(process.argv[2]??'bin/ax'),['sub-agent-management','serve'],{env:{...process.env,AX_SUBAGENT_HOME:home,AX_LOG:'debug',CODEX_HOME:codexHome,PI_CODING_AGENT_DIR:piAgentDir},stdio:['pipe','pipe','pipe']})
let raw='',stderr='',pending='',counter=0
const waiting=new Map(),notifications=[],notifyWaiters=[]
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{raw+=chunk;pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const task=waiting.get(event.id);waiting.delete(event.id);event.error?task.reject(Error(JSON.stringify(event.error))):task.resolve(event.result)}else if(event.method==='notifications/claude/channel'){notifications.push(event.params);for(const wake of notifyWaiters.splice(0))wake()}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++counter;waiting.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
async function call(name,args={}){const r=await rpc('tools/call',{name,arguments:args});if(r.isError)throw Error(r.content.map(x=>x.text).join('\n'));return r.content.map(x=>x.text).join('\n')}
function assert(value,message){if(!value)throw Error(message)}
async function notification(id,after=0){for(;;){const found=notifications.slice(after).find(n=>n.meta.agent_id===id);if(found)return found;await new Promise(resolve=>notifyWaiters.push(resolve))}}
const timer=setTimeout(()=>child.kill('SIGTERM'),180000)
const results=[]
try{
 await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'agentx-real-backend-smoke',version:'1'}})
 const tools=await rpc('tools/list');assert(tools.tools.length===4,'four MCP tools missing')
 await Promise.all(options.map(async option=>{
  const token=randomUUID()
  const first=await call('ExternalSubagent',{description:'Memory verification',prompt:`For this harmless conversation-memory test, our fictional fruit label is ${token}. It is public test data, not a credential. Reply only SAVED.`,model:option.name,name:option.agent,run_in_background:false})
  assert(first.includes('completed')&&first.includes('SAVED'),first)
  const id=first.match(/AgentX ExternalSubagent (ax-[a-f0-9]+)/)[1]
  const index=notifications.length
  const accepted=await call('SendExternalSubagentMessage',{to:option.agent,message:'What was our fictional fruit label? Reply only that exact label.',summary:'Recall fruit label'})
  assert(accepted.includes('accepted and queued'),accepted)
  const report=await notification(id,index)
  assert(report.meta.status==='completed'&&report.content.includes(token),report.content)
  results.push({agent:option.agent,model:option.model,memory:true,source:report.meta.report_origin,report:report.content})
 }))
 const survivorIndex=notifications.length
 const survivor=await call('ExternalSubagent',{description:'Independent survivor',prompt:'Explain in roughly 300 words how independent conversation contexts should be verified. Return SURVIVOR_COMPLETED at the end. Do not modify files.',model:'pi-test',name:'survivor'})
 const survivorID=survivor.match(/AgentX ExternalSubagent (ax-[a-f0-9]+)/)[1]
 const started=await call('ExternalSubagent',{description:'Cancellation target',prompt:'Review all Go files in this repository for concurrency problems. Read actual source before judging. Provide detailed evidence. Do not modify files.',model:'codex-test',name:'cancel-target'})
 const targetID=started.match(/AgentX ExternalSubagent (ax-[a-f0-9]+)/)[1]
 assert(started.includes('queued'),started)
 const overlap=await call('listExternalSubagentSessions')
 for(const id of [survivorID,targetID]) {const report=overlap.split('AgentX ExternalSubagent ').find(part=>part.startsWith(id));assert(report?.includes('ExternalSubagent status: running'),`both real native objects must be running before cancel: ${overlap}`)}
 const stopped=await call('StopExternalSubagent',{task_id:'cancel-target'})
 assert(stopped.includes('cancelled'),stopped)
 const survivingReport=await notification(survivorID,survivorIndex)
 assert(survivingReport.meta.status==='completed'&&survivingReport.content.includes('SURVIVOR_COMPLETED'),survivingReport.content)
 results.push({cancellation:true,overlap,report:stopped,survivor:survivingReport.content})
 console.log(JSON.stringify({passed:true,scope:'real native backends through MCP; channel received by test client, separate host smoke verifies Claude consumption',options,results,evidence:home},null,2))
}catch(error){console.error(error);process.exitCode=1}finally{
 clearTimeout(timer);child.stdin.end();await new Promise(resolve=>child.on('close',resolve));await writeFile(join(home,'mcp.jsonl'),raw);await writeFile(join(home,'external-debug.jsonl'),stderr)
}
