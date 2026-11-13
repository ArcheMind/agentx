import {spawn} from 'node:child_process'
import {mkdtemp,mkdir,writeFile} from 'node:fs/promises'
import {tmpdir,homedir} from 'node:os'
import {join,resolve} from 'node:path'
import {randomUUID} from 'node:crypto'

const home=await mkdtemp(join(tmpdir(),'sessionman-mcp-live-'))
const binary=resolve(process.argv[2]??'bin/sm')
const codexHome=process.env.SM_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.SM_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const writePreset=async(home,{name,agent,model,effort,description})=>{const dir=join(home,'.sessionman','presets',name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${model}\n${effort?`effort: ${effort}\n`:''}---\n\n${description}\n`)}
const options=[{name:'codex-test',agent:'codex',model:process.env.SM_MCP_CODEX_MODEL??'gpt-5.6-luna',description:'Read-only Codex smoke',effort:'low'},{name:'pi-test',agent:'pi',model:process.env.SM_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna',description:'Read-only Pi smoke'}]
for(const option of options)await writePreset(home,option)
const env={...process.env,SM_SUBAGENT_HOME:home,SM_LOG:'debug',CODEX_HOME:codexHome,PI_CODING_AGENT_DIR:piAgentDir}
const child=spawn(binary,['mcp','serve'],{env,stdio:['pipe','pipe','pipe']})
let raw='',stderr='',pending='',counter=0
const waiting=new Map()
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{raw+=chunk;pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const task=waiting.get(event.id);waiting.delete(event.id);event.error?task.reject(Error(JSON.stringify(event.error))):task.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++counter;waiting.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
async function call(name,args={}){const r=await rpc('tools/call',{name,arguments:args});if(r.isError)throw Error(r.content.map(x=>x.text).join('\n'));return r.content.map(x=>x.text).join('\n')}
function assert(value,message){if(!value)throw Error(message)}
// waitEvent arms the per-agent `sm wait` re-arm command printed in the tool response.
async function waitEvent(text){const m=text.match(/sm wait (ax-[a-f0-9]+) --after (\d+)/);assert(m,`no wait command in response: ${text}`);return new Promise((resolve,reject)=>{const p=spawn(binary,['wait',m[1],'--after',m[2]],{env});let out='';p.stdout.on('data',c=>out+=c);p.stderr.on('data',c=>out+=c);p.on('close',code=>code===0?resolve(out):reject(Error('wait exited '+code+': '+out)))})}
const timer=setTimeout(()=>child.kill('SIGTERM'),180000)
const results=[]
try{
 await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'sessionman-real-backend-smoke',version:'1'}})
 const tools=await rpc('tools/list');assert(tools.tools.length===4,'four MCP tools missing')
 await Promise.all(options.map(async option=>{
  const token=randomUUID()
  const first=await call('ExternalSubagent',{description:'Memory verification',prompt:`For this harmless conversation-memory test, our fictional fruit label is ${token}. It is public test data, not a credential. Reply only SAVED.`,model:option.name,name:option.agent,run_in_background:false})
  assert(first.includes('Report status: completed.')&&first.includes('SAVED'),first)
  const id=first.match(/SessionMan ExternalSubagent (ax-[a-f0-9]+)/)[1]
  const accepted=await call('SendExternalSubagentMessage',{to:option.agent,message:'What was our fictional fruit label? Reply only that exact label.',summary:'Recall fruit label'})
  assert(/disposition: (started|queued)/.test(accepted),accepted)
  const report=await waitEvent(accepted)
  assert(report.includes('kind=report')&&report.includes('Report status: completed.')&&report.includes(token),report)
  results.push({agent:option.agent,model:option.model,memory:true,report})
 }))
 const survivor=await call('ExternalSubagent',{description:'Independent survivor',prompt:'Explain in roughly 300 words how independent conversation contexts should be verified. Return SURVIVOR_COMPLETED at the end. Do not modify files.',model:'pi-test',name:'survivor'})
 const survivorID=survivor.match(/SessionMan ExternalSubagent (ax-[a-f0-9]+)/)[1]
 const started=await call('ExternalSubagent',{description:'Cancellation target',prompt:'Review all Go files in this repository for concurrency problems. Read actual source before judging. Provide detailed evidence. Do not modify files.',model:'codex-test',name:'cancel-target'})
 const targetID=started.match(/SessionMan ExternalSubagent (ax-[a-f0-9]+)/)[1]
 assert(started.includes('ExternalSubagent status: running'),started)
 const overlap=await call('listExternalSubagentSessions')
 for(const id of [survivorID,targetID]) {const report=overlap.split('SessionMan ExternalSubagent ').find(part=>part.startsWith(id));assert(report?.includes('ExternalSubagent status: running'),`both real native objects must be running before cancel: ${overlap}`)}
 const stopped=await call('StopExternalSubagent',{task_id:'cancel-target'})
 assert(stopped.includes('cancelled'),stopped)
 const survivingReport=await waitEvent(survivor)
 assert(survivingReport.includes('Report status: completed.')&&survivingReport.includes('SURVIVOR_COMPLETED'),survivingReport)
 results.push({cancellation:true,overlap,report:stopped,survivor:survivingReport})
 console.log(JSON.stringify({passed:true,scope:'real native backends through MCP; test client arms sm wait per agent',options,results,evidence:home},null,2))
}catch(error){console.error(error);process.exitCode=1}finally{
 clearTimeout(timer);child.stdin.end();await new Promise(resolve=>child.on('close',resolve));await writeFile(join(home,'mcp.jsonl'),raw);await writeFile(join(home,'external-debug.jsonl'),stderr)
}