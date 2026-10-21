import {spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,readFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join,resolve} from 'node:path'

const home=await mkdtemp(join(tmpdir(),'agentx-mcp-management-'))
const env={...process.env,HOME:home,AX_SUBAGENT_HOME:home}
delete env.CLAUDE_CONFIG_DIR
const executable=resolve(process.argv[2]??'bin/ax')
function run(action,input){const result=spawnSync(executable,['sub-agent-management',action],{input,env,encoding:'utf8'});if(result.status!==0)throw Error(`${action}: ${result.stderr}`);return result.stdout}
function assert(value,message){if(!value)throw Error(message)}
const unrelated={type:'stdio',command:'echo',args:['unrelated-server']}
await writeFile(join(home,'.claude.json'),JSON.stringify({mcpServers:{unrelated}}))
await mkdir(join(home,'.agentx','presets','review'),{recursive:true})
await writeFile(join(home,'.agentx','presets','review','PRESET.md'),'---\nagent: codex\nmodel: gpt-5.6-luna\n---\n\nRead-only review\n')
await mkdir(join(home,'.config','agentx'),{recursive:true})
await writeFile(join(home,'.config','agentx','subagents.json'),JSON.stringify({enabled:false,grants:{review:{execution:{policy:'read-only',approval:'never'},auth:{source:'codex-home',path:home,method:'chatgpt'}}}}))
assert(JSON.parse(run('status')).presets[0].execution.policy==='read-only','grant not visible in status')
assert(JSON.parse(run('status')).enabled===false,'initial disabled state')
run('enable');run('enable');run('disable');run('enable')
const status=JSON.parse(run('status'))
assert(status.enabled===true&&status.presets.length===1&&status.presets[0].name==='review'&&status.presets[0].effort==='native default','persistent configuration or shared preset lost')
const stored=JSON.parse(await readFile(join(home,'.claude.json'),'utf8'))
assert(JSON.stringify(stored.mcpServers.unrelated)===JSON.stringify(unrelated),'unrelated MCP entry changed')
assert(stored.mcpServers.agentx.command===executable,'MCP command wrong')
stored.mcpServers.agentx.command='conflicting-user-server'
await writeFile(join(home,'.claude.json'),JSON.stringify(stored))
const rejected=spawnSync(executable,['sub-agent-management','enable'],{env,encoding:'utf8'})
assert(rejected.status!==0&&rejected.stderr.includes('different configuration'),'conflicting server overwritten')
assert(JSON.parse(await readFile(join(home,'.claude.json'),'utf8')).mcpServers.agentx.command==='conflicting-user-server','conflict mutated')
console.log(JSON.stringify({passed:true,test:'real Claude user MCP registration with isolated HOME, repeated enable, disable/re-enable, unrelated entry preservation, explicit conflict rejection',evidence:home}))
