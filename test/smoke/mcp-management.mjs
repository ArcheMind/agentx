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
run('configure',JSON.stringify({enabled:false,default:'review',options:[{name:'review',agent:'codex',model:'gpt-5.6-luna',description:'Read-only review',enabled:true}]}))
assert(JSON.parse(run('status')).enabled===false,'initial disabled state')
run('enable');run('enable');run('disable');run('enable')
const status=JSON.parse(run('status'))
assert(status.enabled===true&&status.default==='review','persistent configuration lost')
const stored=JSON.parse(await readFile(join(home,'.claude.json'),'utf8'))
assert(JSON.stringify(stored.mcpServers.unrelated)===JSON.stringify(unrelated),'unrelated MCP entry changed')
assert(stored.mcpServers.agentx.command===executable,'MCP command wrong')
stored.mcpServers.agentx.command='conflicting-user-server'
await writeFile(join(home,'.claude.json'),JSON.stringify(stored))
const rejected=spawnSync(executable,['sub-agent-management','enable'],{env,encoding:'utf8'})
assert(rejected.status!==0&&rejected.stderr.includes('different configuration'),'conflicting server overwritten')
assert(JSON.parse(await readFile(join(home,'.claude.json'),'utf8')).mcpServers.agentx.command==='conflicting-user-server','conflict mutated')
console.log(JSON.stringify({passed:true,test:'real Claude user MCP registration with isolated HOME, repeated enable, disable/re-enable, unrelated entry preservation, explicit conflict rejection',evidence:home}))
