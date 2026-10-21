import {spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,writeFile,readFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join,resolve} from 'node:path'

const home=await mkdtemp(join(tmpdir(),'agentx-mcp-management-'))
const env={...process.env,HOME:home,AX_SUBAGENT_HOME:home}
delete env.CLAUDE_CONFIG_DIR
const executable=resolve(process.argv[2]??'bin/ax')
const exec=(command,args)=>spawnSync(command,args,{env,encoding:'utf8'})
function ax(...args){const result=exec(executable,args);if(result.status!==0)throw Error(`ax ${args.join(' ')}: ${result.stderr}`);return result.stdout}
function claude(...args){const result=exec('claude',['mcp',...args]);if(result.status!==0)throw Error(`claude mcp ${args.join(' ')}: ${result.stderr}`);return result.stdout}
function assert(value,message){if(!value)throw Error(message)}
const status=()=>JSON.parse(ax('sub-agent-management','status'))
const launch=()=>exec(executable,['--json','claude','--cwd',home,'--subagents','--dry-run'])
const unrelated={type:'stdio',command:'echo',args:['unrelated-server']}
await writeFile(join(home,'.claude.json'),JSON.stringify({mcpServers:{unrelated}}))
for(const [name,agent,model] of [['review','codex','gpt-5.6-luna'],['daily','claude','opus']]){await mkdir(join(home,'.agentx','presets',name),{recursive:true});await writeFile(join(home,'.agentx','presets',name,'PRESET.md'),`---\nagent: ${agent}\nmodel: ${model}\n---\n\n${name} preset\n`)}
// A leftover file from the earlier configuration model must be ignored, never read or rewritten.
const legacy=join(home,'.config','agentx','subagents.json'),legacyText=JSON.stringify({enabled:true,grants:{review:{}}})
await mkdir(join(home,'.config','agentx'),{recursive:true});await writeFile(legacy,legacyText)
for(const removed of ['enable','disable']){const r=exec(executable,['sub-agent-management',removed]);assert(r.status!==0&&r.stderr.includes('claude mcp add --scope user agentx'),`${removed} still exists: ${r.stdout}${r.stderr}`)}
const initial=status()
assert(initial.mcp_registered===false&&initial.enabled===undefined&&initial.grant_problems===undefined&&initial.presets.length===1&&initial.presets[0].name==='review'&&initial.presets[0].execution===undefined&&initial.write_behavior.includes('read-only unless it sets write'),'initial state: '+JSON.stringify(initial))
const unregistered=launch()
assert(unregistered.status!==0&&unregistered.stderr.includes(`claude mcp add --scope user agentx -- ${executable} sub-agent-management serve`),'ax claude --subagents without registration: '+unregistered.stderr)
claude('add','--scope','user','agentx','--',executable,'sub-agent-management','serve')
const registered=status()
assert(registered.mcp_registered===true&&registered.mcp_registration_problem==='','native registration not recognized: '+JSON.stringify(registered))
const plan=launch()
assert(plan.status===0&&JSON.parse(plan.stdout).args.join(' ').includes('--dangerously-load-development-channels server:agentx'),'ax claude did not supply the AgentX MCP connection: '+plan.stdout+plan.stderr)
claude('remove','--scope','user','agentx')
assert(status().mcp_registered===false,'native removal not recognized')
const stored=JSON.parse(await readFile(join(home,'.claude.json'),'utf8'))
assert(JSON.stringify(stored.mcpServers.unrelated)===JSON.stringify(unrelated),'unrelated MCP entry changed')
stored.mcpServers.agentx={type:'stdio',command:'conflicting-user-server',args:[]}
await writeFile(join(home,'.claude.json'),JSON.stringify(stored))
const conflict=status()
assert(conflict.mcp_registered===false&&conflict.mcp_registration_problem.includes('does not run'),'conflicting entry not reported: '+JSON.stringify(conflict))
const conflicted=launch()
assert(conflicted.status!==0&&conflicted.stderr.includes('does not run'),'ax claude used a conflicting agentx entry')
assert(JSON.parse(await readFile(join(home,'.claude.json'),'utf8')).mcpServers.agentx.command==='conflicting-user-server','conflict mutated')
assert(await readFile(legacy,'utf8')===legacyText,'legacy subagents.json was touched')
console.log(JSON.stringify({passed:true,test:'registration through real claude mcp add/remove with isolated HOME is recognized by status and ax claude; enable/disable subcommands gone; only MCP-callable presets listed; conflicting agentx entry reported and never used or changed; unrelated entry preserved; legacy subagents.json ignored',evidence:home}))
