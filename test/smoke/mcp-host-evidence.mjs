import {readFile} from 'node:fs/promises'
import {join} from 'node:path'

export function inspectHost(history,labels,live,{historical=false}={}){
 const tool=historical?{create:'Agent',send:'SendMessage',list:'ListAgents'}:{create:'ExternalSubagent',send:'SendExternalSubagentMessage',list:'listExternalSubagentSessions'}
 const reportPrefix=historical?'AgentX subagent ':'AgentX ExternalSubagent '
 const runningLabel=historical?'subagent status: running':'ExternalSubagent status: running'
 const events=history.trim().split('\n').flatMap(line=>{try{return [JSON.parse(line)]}catch{return []}})
 const calls=new Map()
 for(const event of events)if(event.type==='assistant')for(const block of event.message?.content??[])if(block.type==='tool_use')calls.set(block.id,block)
 const results=events.flatMap(event=>event.type==='user'&&Array.isArray(event.message?.content)?event.message.content.filter(b=>b.type==='tool_result').map(b=>({call:calls.get(b.tool_use_id),text:typeof b.content==='string'?b.content:b.content?.map(p=>p.text??'').join('\n')??''})):[])
 const assistant=events.filter(e=>e.type==='assistant').flatMap(e=>e.message?.content??[]).filter(b=>b.type==='text').map(b=>b.text)
 const result={sawCall:[...calls.values()].some(c=>c.name===`mcp__agentx__${tool.create}`),sawCompletion:assistant.some(text=>labels.every(label=>text.includes(label))),noParentRead:![...calls.values()].some(c=>c.name==='Read'||c.name==='Bash')}
 if(!live)return result
 const channels=events.filter(e=>e.attachment?.type==='queued_command').map(e=>e.attachment.prompt??'').filter(text=>text.startsWith('<channel source="agentx"'))
 result.workers=[]
 for(const [i,name] of ['codex-worker','pi-worker'].entries()){
  const model=i===0?'codex-test':'pi-test'
  const creations=[...calls.values()].filter(c=>c.name===`mcp__agentx__${tool.create}`&&c.input.name===name&&c.input.model===model)
  const created=results.find(r=>r.call?.id===creations[0]?.id)
  const id=created?.text.split(reportPrefix)[1]?.match(/^(ax-[a-f0-9]+)/)?.[1]
  const sends=results.filter(r=>r.call?.name===`mcp__agentx__${tool.send}`&&r.call.input.to===name)
  result.workers.push({name,model,id,oneCreation:creations.length===1,busyMessage:sends.length===1&&sends[0].text.includes(id??'INVALID')&&sends[0].text.includes('accepted and queued; not yet processed.')&&sends[0].text.includes(runningLabel),completedRecall:channels.some(text=>text.includes(`agent_id="${id}"`)&&text.includes('status="completed"')&&text.includes(labels[i]))})
 }
 result.overlap=results.some(r=>r.call?.name===`mcp__agentx__${tool.list}`&&result.workers.every(w=>r.text.split(reportPrefix).some(part=>part.startsWith(w.id??'INVALID')&&part.includes(runningLabel))))
 return result
}

if(process.argv[1]===new URL(import.meta.url).pathname){
 const [home,transcript]=process.argv.slice(2)
 const labels=await Promise.all([0,1].map(i=>readFile(join(home,`fruit-${i}.txt`),'utf8').then(x=>x.trim())))
 const history=await readFile(transcript,'utf8')
 const debug=await readFile(join(home,'claude-debug.log'),'utf8')
 const historical=process.argv.includes('--historical')
 const result={...inspectHost(history,labels,true,{historical}),format:historical?'historical pre-rename':'ExternalSubagent',registered:debug.includes('MCP server "agentx": Channel notifications registered'),evidence:home,transcript}
 console.log(JSON.stringify(result,null,2))
 if(!result.sawCall||!result.sawCompletion||!result.noParentRead||!result.overlap||!result.registered||result.workers.some(w=>!w.oneCreation||!w.busyMessage||!w.completedRecall))process.exitCode=1
}
