import {spawn,spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,readFile,readdir,realpath} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join,resolve} from 'node:path'

const binary=resolve(process.argv[2]??'bin/ax')
const home=await mkdtemp(join(tmpdir(),'agentx-profiles-'))
const workspace=join(home,'workspace')
const bin=join(home,'bin')
const codexHome=join(home,'codex-home')
await mkdir(workspace);await mkdir(bin);await mkdir(codexHome)
await writeFile(join(bin,'codex'),`#!${process.execPath}
const fs=require('fs'),path=require('path')
fs.writeFileSync(path.join(${JSON.stringify(home)},'codex-argv-'+Date.now()+'-'+process.pid+'.json'),JSON.stringify({argv:process.argv.slice(2),cwd:process.cwd()}))
if(process.argv[2]==='login'){console.log('Logged in using ChatGPT');process.exit(0)}
if(process.argv[2]==='app-server'){console.error('fake codex: no model list');process.exit(1)}
if(process.argv[2]==='exec'){const effort=(process.argv.find(a=>a.startsWith('model_reasoning_effort='))??'model_reasoning_effort="medium"').split('=')[1];const developer=process.argv.find(a=>a.startsWith('developer_instructions='));const thread='12345678-1234-1234-1234-'+String(process.pid).padStart(12,'0');const dir=path.join(process.env.CODEX_HOME,'sessions','2026','10','21');fs.mkdirSync(dir,{recursive:true});fs.writeFileSync(path.join(dir,'rollout-x-'+thread+'.jsonl'),JSON.stringify({type:'response_item',payload:{type:'message',role:'developer',content:[{type:'input_text',text:developer?JSON.parse(developer.slice('developer_instructions='.length)):''}]}})+'\\n');if(process.env.RUST_LOG==='codex_otel.trace_safe=info')console.error('x INFO codex_otel.trace_safe: event.name="codex.sse_event" event.kind=response.completed model_reasoning_effort='+effort);let input='';process.stdin.on('data',d=>input+=d).on('end',()=>{console.log(JSON.stringify({type:'thread.started',thread_id:thread}));console.log(JSON.stringify({type:'item.completed',item:{type:'agent_message',text:'FAKE_REPORT '+input}}));console.log(JSON.stringify({type:'turn.completed'}))})}
`,{mode:0o700})
const env={...process.env,HOME:home,AX_SUBAGENT_HOME:home,CODEX_HOME:codexHome,PATH:bin+':/usr/bin:/bin'}
delete env.CLAUDE_CONFIG_DIR
function assert(value,message){if(!value)throw Error(message)}
function ax(input){const result=spawnSync(binary,[],{cwd:workspace,input,env,encoding:'utf8'});return {...result,output:result.stdout+result.stderr}}
async function argvFiles(){return (await readdir(home)).filter(name=>name.startsWith('codex-argv-')).sort()}
const evidence={}

const empty=ax('q')
assert(empty.status!==0&&empty.output.includes('New session · Profiles')&&empty.output.includes('Save an agent, model and effort for reuse.')&&empty.output.includes('+ Create profile…')&&empty.output.includes('Recent sessions · Current workspace'),'P01 empty home: '+empty.output)
evidence.P01='empty collection shows hint, create entry and recent-session sections'

const create=ax('\r'+'j\r'+'smoke-model\n'+'jjj\r'+'smoke review\n'+'smoke-review\n'+'Smoke review profile for deterministic acceptance\n'+'j\r'+'q')
const profileFile=join(home,'.agentx','profiles','smoke-review','PROFILE.md')
const saved=await readFile(profileFile,'utf8')
assert(saved==='---\nagent: codex\nmodel: smoke-model\neffort: low\n---\n\nSmoke review profile for deterministic acceptance\n','saved file '+saved)
assert(create.output.includes('Model list unavailable')&&create.output.includes('unverified for this model')&&create.output.includes('Error: Invalid name')&&create.output.includes('Saving does not run inference.')&&create.output.includes('MCP: AgentX is not registered with Claude'),'P05/P07/P08 create output: '+create.output)
const afterCreate=await argvFiles()
assert(afterCreate.length===1&&JSON.parse(await readFile(join(home,afterCreate[0]),'utf8')).argv[0]==='app-server','P16: creation ran something besides model listing')
evidence.P05='created via TUI; PROFILE.md written; home lists it; no other configuration file exists or is needed';evidence.P08='fake codex model list failure shown honestly, manual ID accepted';evidence.P16='only app-server model listing ran during creation'

const mcp=spawn(binary,['mcp','serve'],{cwd:workspace,env,stdio:['pipe','pipe','pipe']})
let next=0,pending=''
const waiting=new Map()
mcp.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const event=JSON.parse(pending.slice(0,end));pending=pending.slice(end+1);if(event.id!==undefined){waiting.get(event.id)(event);waiting.delete(event.id)}}})
const rpc=(method,params={})=>new Promise(done=>{const id=++next;waiting.set(id,done);mcp.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})
const call=async(name,args)=>{const event=await rpc('tools/call',{name,arguments:args});return event.result.content.map(part=>part.text).join('\n')}
try{
 const init=(await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'profiles-smoke',version:'1'}})).result
 assert(init.instructions.includes('- smoke-review: execution agent="codex"')&&init.instructions.includes('exact configured model ID (including provider prefix)="smoke-model"')&&init.instructions.includes('effort="low"')&&init.instructions.includes('user description="Smoke review profile for deterministic acceptance"')&&init.instructions.includes('ExternalSubagent callable=yes'),'P10 initialize catalog: '+init.instructions)
 const tools=(await rpc('tools/list')).result.tools
 assert(JSON.stringify(tools[0].inputSchema.properties.model.enum)===JSON.stringify(['smoke-review'])&&!tools[0].inputSchema.properties.effort,'P10 enum/effort schema')
  const live=ax('m'+'jjj\r'+'smoke-review-pi\n'+'j\r'+'q'+'q')
 assert(live.output.includes('1 running AgentX MCP connection(s) still show the profile list'),'P15 change note: '+live.output)
 await writeFile(profileFile,saved.replace('Smoke review profile for deterministic acceptance','Hand-edited description'))
 await mkdir(join(home,'.agentx','profiles','broken'))
 await writeFile(join(home,'.agentx','profiles','broken','PROFILE.md'),'---\nagent: codex\n---\nmissing model\n')
 const refreshed=(await rpc('tools/list')).result.tools[0]
 assert(refreshed.description.includes('- profile smoke-review: codex "smoke-model" effort=low; callable; "Hand-edited description"')&&refreshed.description.includes('- profile broken: invalid (')&&refreshed.description.includes('model is required')&&refreshed.description.includes('- profile smoke-review-pi: codex')&&refreshed.description.length<4096&&JSON.stringify(refreshed.inputSchema.properties.model.enum)===JSON.stringify(['smoke-review','smoke-review-pi'])&&refreshed.inputSchema.properties.write.type==='boolean'&&refreshed.inputSchema.properties.writable_roots.type==='array','P14 MCP sees edited and broken items: '+refreshed.description)
 const report=await call('ExternalSubagent',{description:'smoke',prompt:'check',model:'smoke-review',run_in_background:false})
 assert(report.includes('profile=smoke-review; agent=codex; actual model=smoke-model; effort=low')&&report.includes('Write scope: read-only (no write requested)')&&report.includes(`Auth (discovered by preflight for this turn): CODEX_HOME=${JSON.stringify(codexHome)} (chatgpt)`)&&report.includes('FAKE_REPORT check')&&report.includes('reconnect AgentX MCP to refresh the profile list'),'P10/P15 call: '+report)
 const override=await call('ExternalSubagent',{description:'smoke',prompt:'check',model:'smoke-review',effort:'high',run_in_background:false})
 assert(override.includes('unknown field "effort"'),'per-call effort override accepted: '+override)
 const argvBefore=(await argvFiles()).length
 const unscoped=await call('ExternalSubagent',{description:'smoke',prompt:'check',model:'smoke-review',write:true,run_in_background:false})
 assert(unscoped.includes('write without isolation requires writable_roots')&&(await argvFiles()).length===argvBefore,'write without scope accepted: '+unscoped)
 const scoped=await call('ExternalSubagent',{description:'smoke',prompt:'check',model:'smoke-review',write:true,writable_roots:[workspace],run_in_background:false})
 const scopedArgv=JSON.parse(await readFile(join(home,(await argvFiles()).at(-1)),'utf8')).argv.join(' ')
 assert(scoped.includes('Write scope: working directory')&&scoped.includes('In write mode the MCP working directory is always writable as well (native Codex behavior)')&&scopedArgv.includes('sandbox_mode="workspace-write"')&&scopedArgv.includes(`writable_roots=${JSON.stringify([await realpath(workspace)])}`),'call-time write scope: '+scoped+scopedArgv)
 evidence.W01='write is per call: no write is read-only, write without roots is rejected before any native run, write with writable_roots reaches the Codex sandbox resolved'
 evidence.P10='initialize, tools/list enum and ExternalSubagent use ~/.agentx/profiles; report names profile, agent, model, effort'
 evidence.P15='TUI reports live MCP connection must reconnect; call result notes stale snapshot'
}finally{mcp.stdin.end();await new Promise(done=>mcp.once('close',done))}

const before=(await argvFiles()).length
const home2=ax('\r')
const launches=await argvFiles()
assert(launches.length===before+1,'P03 no launch: '+home2.output)
const launch=JSON.parse(await readFile(join(home,launches.at(-1)),'utf8'))
assert(launch.cwd.endsWith('/workspace')&&launch.argv.join(' ').includes('--model smoke-model')&&launch.argv.join(' ').includes('model_reasoning_effort="low"')&&home2.output.includes('broken')&&home2.output.includes('Error: model is required')&&/› smoke-review +Codex · smoke-model · low +Hand-edited description\r\n/.test(home2.output)&&!/\n +Codex · /.test(home2.output),'P03/P14 launch and one-line profile rows: '+JSON.stringify(launch)+home2.output)
evidence.P03='bare ax → profile launched codex in cwd with --model and effort';evidence.P14='broken item located on home without blocking valid profile'
console.log(JSON.stringify({passed:true,binary,evidence,home},null,2))
