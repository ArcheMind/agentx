import {spawn,spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile} from 'node:fs/promises'
import {tmpdir,homedir} from 'node:os'
import {join,resolve} from 'node:path'
import {randomUUID} from 'node:crypto'

const binary=resolve(process.env.AX_MCP_BINARY??process.argv[2]??'bin/ax')
const home=await mkdtemp(join(tmpdir(),'agentx-preset-visibility-'))
const names=[`codex-${randomUUID().slice(0,8)}`,`pi-${randomUUID().slice(0,8)}`]
const options=[{name:names[0],agent:'codex',model:'gpt-5.6-luna',description:'Read-only research and code review',enabled:true,efforts:['low']},{name:names[1],agent:'pi',model:'openai-codex/gpt-5.6-luna',description:'Read-only independent analysis',enabled:true}]
const codexHome=process.env.AX_MCP_CODEX_HOME??join(homedir(),'.codex'),piAgentDir=process.env.AX_MCP_PI_AGENT_DIR??join(homedir(),'.pi','agent')
const readOnly=(agent)=>({execution:{policy:'read-only',approval:'never'},auth:agent==='codex'?{source:'codex-home',path:codexHome,method:process.env.AX_MCP_CODEX_AUTH_METHOD??'chatgpt'}:{source:'pi-agent-dir',path:piAgentDir,method:process.env.AX_MCP_PI_AUTH_METHOD??'oauth'}})
await mkdir(join(home,'.config','agentx'),{recursive:true})
await writeFile(join(home,'.config','agentx','subagents.json'),JSON.stringify({enabled:true,default:names[0],options,grants:Object.fromEntries(options.map(o=>[o.name,readOnly(o.agent)]))}))
const mcp={mcpServers:{agentx:{type:'stdio',command:binary,args:['sub-agent-management','serve'],env:{AX_SUBAGENT_HOME:home}}}}
const prompt=process.env.MCP_PRESET_PROMPT??'有哪些可用subagent选择？'
const judgment=process.env.MCP_PRESET_JUDGMENT??'reviewer'
if(!['reviewer','presets'].includes(judgment))throw Error('MCP_PRESET_JUDGMENT must be reviewer or presets')
const version=spawnSync('claude',['--version'],{encoding:'utf8'})
if(version.status!==0)throw Error(version.stderr||'Cannot read Claude version')
const args=['--print','--verbose','--output-format','stream-json','--permission-mode','dontAsk','--allowedTools','mcp__agentx__listExternalSubagentSessions','--model','fable','--strict-mcp-config','--mcp-config',JSON.stringify(mcp),'--',prompt]
const cwd=process.env.MCP_PRESET_CWD??home
const child=spawn('claude',args,{cwd,env:{...process.env,ENABLE_TOOL_SEARCH:'true'},stdio:['ignore','pipe','pipe'],detached:true})
let stdout='',stderr='',timedOut=false,hardKill
child.stdout.on('data',data=>stdout+=data)
child.stderr.on('data',data=>stderr+=data)
function kill(signal){try{process.kill(-child.pid,signal)}catch(error){if(error.code!=='ESRCH')throw error}}
const timer=setTimeout(()=>{timedOut=true;kill('SIGTERM');hardKill=setTimeout(()=>kill('SIGKILL'),3000)},90000)
const code=await new Promise(resolve=>child.on('close',resolve));clearTimeout(timer);clearTimeout(hardKill)
const events=stdout.trim().split('\n').flatMap(line=>{try{return [JSON.parse(line)]}catch{return []}})
const assistant=events.filter(event=>event.type==='assistant')
const calls=assistant.flatMap(event=>event.message?.content??[]).filter(block=>block.type==='tool_use').map(block=>({name:block.name,input:block.input}))
const terminal=events.findLast(event=>event.type==='result')
const answer=terminal?.result??''
const correctPresets=options.every(option=>answer.split('\n').some(line=>line.includes(option.name)&&line.toLowerCase().includes(option.agent)&&line.includes(option.model)&&/read.only|只读/i.test(line)))
const noWork=calls.every(call=>call.name==='ToolSearch')
const readOnlyCalls=calls.every(call=>call.name==='ToolSearch'||call.name==='mcp__agentx__listExternalSubagentSessions')
const succeeded=code===0&&!timedOut&&terminal?.subtype==='success'&&terminal.is_error===false
const result={prompt,judgment,binary,cwd,hostVersion:version.stdout.trim(),parentModels:[...new Set(assistant.map(event=>event.message?.model).filter(Boolean))],deferred:true,nativeTools:'default',permissionMode:'dontAsk',options,code,timedOut,succeeded,correctPresets,noWork,readOnlyCalls,calls,answer,evidence:home}
await writeFile(join(home,'raw-private.jsonl'),stdout)
await writeFile(join(home,'stderr-private.log'),stderr)
await writeFile(join(home,'observed.json'),JSON.stringify(result,null,2))
console.log(JSON.stringify(result,null,2))
if(!succeeded||!readOnlyCalls||(judgment==='presets'&&(!correctPresets||!noWork)))process.exitCode=1
