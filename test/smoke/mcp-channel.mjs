import {spawn} from 'node:child_process'
import {mkdtemp, mkdir, writeFile, readFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join, resolve} from 'node:path'
import {randomUUID} from 'node:crypto'
import {homedir} from 'node:os'
import {inspectHost} from './mcp-host-evidence.mjs'

const executable=resolve(process.argv[2]??'bin/ax')
const live=process.argv.includes('--live')
const home=await mkdtemp(join(tmpdir(),'agentx-channel-'))
await mkdir(join(home,'bin'))
await writeFile(join(home,'bin','pi'),`#!/usr/bin/env node
(async()=>{
if(process.argv.includes('auth')){console.log(JSON.stringify({status:'ready',provider:'test',authType:'api_key'}));return}
let input='';for await (const chunk of process.stdin) input+=chunk;
await new Promise(r=>setTimeout(r,4000));
console.log(JSON.stringify({type:'session',id:'test-session'}));
console.log(JSON.stringify({type:'message_end',message:{role:'assistant',provider:'test',model:'channel',stopReason:'stop',content:[{type:'text',text:'CHANNEL_RECEIVED_8f326c'}]}}));
console.log(JSON.stringify({type:'agent_end'}));
})().catch(error=>{console.error(error);process.exitCode=1})
`,{mode:0o700})
const labels=live?[randomUUID(),randomUUID()]:['CHANNEL_RECEIVED_8f326c']
if(live)for(let i=0;i<2;i++)await writeFile(join(home,`fruit-${i}.txt`),labels[i]+'\n')
const writePreset=async(home,{name,agent,model,effort,description})=>{const dir=join(home,'.agentx','presets',name);await mkdir(dir,{recursive:true});await writeFile(join(dir,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${model}\n${effort?`effort: ${effort}\n`:''}---\n\n${description}\n`)}
const options=live?[
 {name:'codex-test',agent:'codex',model:process.env.AX_MCP_CODEX_MODEL??'gpt-5.6-luna',description:'Actual Codex native read-only runtime'},
 {name:'pi-test',agent:'pi',model:process.env.AX_MCP_PI_MODEL??'openai-codex/gpt-5.6-luna',description:'Actual Pi native read-only runtime'}
]:[{name:'channel-test',agent:'pi',model:'test/channel',description:'Channel integration test fixture, deterministic four second answer.'}]
await mkdir(join(home,'pi-agent'))
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=live?(process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')):join(home,'pi-agent')
for(const option of options)await writePreset(home,option)
const mcp={mcpServers:{agentx:{type:'stdio',command:executable,args:['preset','serve'],env:{AX_SUBAGENT_HOME:home,AX_LOG:'debug',CODEX_HOME:codexHome,PI_CODING_AGENT_DIR:piAgentDir,PATH:live?process.env.PATH:join(home,'bin')+':'+process.env.PATH}}}}
const prompt=live?`This is an authorized integration acceptance test, not a code change. Use only AgentX MCP tools (ToolSearch discovery allowed). Do not read any files yourself. Start exactly two independent background AgentX ExternalSubagents with descriptions and these exact names: codex-worker (model codex-test), pi-worker (model pi-test). Create both in parallel. For codex-worker, its prompt must ask it to read ${join(home,'fruit-0.txt')}, remember the public fictional fruit label for later, then write about 1000 words explaining conversation-memory test methodology WITHOUT revealing the label and end with SAVED. For pi-worker, do the same using ${join(home,'fruit-1.txt')}. Immediately after creation, SendExternalSubagentMessage once to each worker while still working, asking it to recall its label, reply ONLY the label, and not reread the file. Do not repeat labels in follow-up messages: you do not know them. listExternalSubagentSessions once to establish both workers are running. Wait for channel completion notifications for both recalled labels and report both labels exactly. No polling, no additional ExternalSubagent creation, no cancellation, no native Agent, no direct Read/Bash. If an acceptance precondition fails, report it honestly.`:'Use mcp__agentx__ExternalSubagent exactly once to start a background task using model channel-test, description "channel test", prompt "Return the test token", run_in_background true. The backend will send its result asynchronously via AgentX channel. Do not use listExternalSubagentSessions or other tools. After the completion notification arrives, repeat the backend token verbatim. Do not invent the token. You may say waiting before the notification arrives.'
const session=randomUUID()
const child=spawn('python3',[new URL('./pty-host.py',import.meta.url).pathname,'claude','--session-id',session,'--debug-file',join(home,'claude-debug.log'),'--strict-mcp-config','--mcp-config',JSON.stringify(mcp),'--dangerously-load-development-channels','server:agentx','--allowedTools','mcp__agentx__ExternalSubagent','mcp__agentx__SendExternalSubagentMessage','mcp__agentx__StopExternalSubagent','mcp__agentx__listExternalSubagentSessions','--',prompt],{stdio:['pipe','pipe','pipe'],env:{...process.env,TERM:'xterm-256color'}})
let output='',stderr='',pending='',sawCall=false,sawCompletion=false,confirmed=false
child.stderr.on('data',chunk=>{stderr+=chunk})
const timer=setTimeout(()=>{child.kill('SIGTERM')},live?240000:90000)
const transcript=join(homedir(),'.claude','projects',process.cwd().replace(/[^A-Za-z0-9]/g,'-'),session+'.jsonl')
const interval=setInterval(async()=>{
 let data;try{data=await readFile(transcript,'utf8')}catch{return}
 for(const line of data.trim().split('\n')){
  let event;try{event=JSON.parse(line)}catch{continue}
  if(event.type==='assistant'){
   const blocks=event.message?.content??[]
   if(blocks.some(b=>b.type==='tool_use'&&b.name.endsWith('__ExternalSubagent'))) sawCall=true
   if(blocks.some(b=>b.type==='text'&&labels.every(label=>b.text.includes(label)))) {sawCompletion=true;child.kill('SIGTERM')}
  }
 }
},500)
child.stdout.on('data',chunk=>{
 output+=chunk;pending+=chunk
 if(!confirmed&&output.includes('development')&&output.includes('confirm')) {confirmed=true;child.stdin.write('\r')}
 let end
 while((end=pending.indexOf('\n'))>=0){
  const line=pending.slice(0,end);pending=pending.slice(end+1)
  let event;try{event=JSON.parse(line)}catch{continue}
  if(event.type==='assistant'){
   const blocks=event.message?.content??[]
   if(blocks.some(b=>b.type==='tool_use'&&b.name.endsWith('__ExternalSubagent'))) sawCall=true
   if(blocks.some(b=>b.type==='text'&&labels.every(label=>b.text.includes(label)))) {sawCompletion=true;child.stdin.end()}
  }
 }
})
const code=await new Promise(resolve=>child.on('close',resolve));clearTimeout(timer);clearInterval(interval)
await writeFile(join(home,'claude-output.jsonl'),output)
await writeFile(join(home,'claude-stderr.log'),stderr)
const debug=await readFile(join(home,'claude-debug.log'),'utf8').catch(()=> '')
const registered=debug.includes('MCP server "agentx": Channel notifications registered')
const history=await readFile(transcript,'utf8').catch(()=> '')
const inspected=inspectHost(history,labels,live)
console.log(JSON.stringify({host:'Claude Code interactive',test:live?'actual Claude → Codex/Pi, busy continuation and channel consumption':'real host consumes asynchronous MCP channel',registered,...inspected,exitCode:code,evidence:home,transcript,options}))
if(!inspected.sawCall||!inspected.sawCompletion||!registered||!inspected.noParentRead||(live&&(!inspected.overlap||inspected.workers.some(w=>!w.oneCreation||!w.busyMessage||!w.completedRecall)))) {console.error(stderr.slice(-5000));process.exitCode=1}
