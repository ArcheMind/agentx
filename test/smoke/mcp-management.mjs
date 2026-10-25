import {spawnSync} from 'node:child_process'
import {existsSync} from 'node:fs'
import {mkdtemp,mkdir,writeFile,readFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join,resolve} from 'node:path'

const home=await mkdtemp(join(tmpdir(),'agentx-mcp-management-'))
const env={...process.env,HOME:home,AX_SUBAGENT_HOME:home}
delete env.CLAUDE_CONFIG_DIR
const executable=resolve(process.argv[2]??'bin/ax')
const exec=(command,args)=>spawnSync(command,args,{env,encoding:'utf8'})
function ax(...args){const result=exec(executable,args);if(result.status!==0)throw Error(`ax ${args.join(' ')}: ${result.stderr}`);return result.stdout}
function assert(value,message){if(!value)throw Error(message)}
const listProfiles=()=>JSON.parse(ax('--json','profile','list'))
const launch=()=>exec(executable,['--json','claude','--cwd',home,'--dry-run'])
for(const [name,agent,model] of [['review','codex','gpt-5.6-luna'],['daily','claude','opus']]){await mkdir(join(home,'.agentx','profiles',name),{recursive:true});await writeFile(join(home,'.agentx','profiles',name,'PROFILE.md'),`---\nagent: ${agent}\nmodel: ${model}\n---\n\n${name} profile\n`)}
// A leftover file from the earlier configuration model must be ignored, never read or rewritten.
const legacy=join(home,'.config','agentx','subagents.json'),legacyText=JSON.stringify({enabled:true,grants:{review:{}}})
await mkdir(join(home,'.config','agentx'),{recursive:true});await writeFile(legacy,legacyText)
for(const removed of ['enable','disable','status']){const r=exec(executable,['profile',removed]);assert(r.status!==0&&r.stderr.includes('usage: ax [--json|--yaml] profile <list|show>'),`${removed} still exists: ${r.stdout}${r.stderr}`)}
const initial=listProfiles()
const review=initial.profiles.find(p=>p.name==='review'),daily=initial.profiles.find(p=>p.name==='daily')
assert(initial.enabled===undefined&&initial.grant_problems===undefined&&initial.profiles.length===2&&review.execution===undefined&&review.auth===undefined&&review.limits.includes('read-only')&&daily.limits!==undefined&&daily.limits.includes('read-only'),'initial state: '+JSON.stringify(initial))
const plan=launch(),args=plan.status===0?JSON.parse(plan.stdout).args:[]
assert(!existsSync(join(home,'.claude.json'))&&args[0]==='--mcp-config'&&JSON.parse(args[1]).mcpServers.agentx.args.join(' ')==='mcp serve'&&args.join(' ').includes('--allowedTools mcp__agentx__ExternalSubagent'),'ax claude did not attach the AgentX MCP connection without registration: '+plan.stdout+plan.stderr)
assert(await readFile(legacy,'utf8')===legacyText,'legacy subagents.json was touched')
console.log(JSON.stringify({passed:true,test:'ax claude attaches the AgentX MCP connection with no ~/.claude.json; enable/disable subcommands gone; all profiles listed; legacy subagents.json ignored',evidence:home}))
