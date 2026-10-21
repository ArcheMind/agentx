import {spawn} from 'node:child_process'
import {mkdtemp} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join,resolve} from 'node:path'

const home=await mkdtemp(join(tmpdir(),'agentx-mcp-contract-'))
const binary=resolve(process.env.AX_MCP_BINARY??process.argv[2]??'bin/ax')
const child=spawn(binary,['sub-agent-management','serve'],{env:{...process.env,AX_SUBAGENT_HOME:home},stdio:['pipe','pipe','pipe']})
let next=0,pending='',stderr=''
const requests=new Map()
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const request=requests.get(event.id);requests.delete(event.id);event.error?request.reject(Error(JSON.stringify(event.error))):request.resolve(event.result)}}})
child.on('close',()=>{for(const request of requests.values())request.reject(Error(`MCP process closed: ${stderr}`));requests.clear()})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++next;requests.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
function assert(value,message){if(!value)throw Error(message)}
const timer=setTimeout(()=>child.kill('SIGTERM'),10000)
try{
 const init=await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'deterministic-contract-check',version:'1'}})
 assert(init.serverInfo.name==='AgentX'&&init.protocolVersion==='2024-11-05','server/protocol identity changed')
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n')
 const {tools}=await rpc('tools/list')
 const expected=['ExternalSubagent','SendExternalSubagentMessage','StopExternalSubagent','listExternalSubagentSessions']
 assert(JSON.stringify(tools.map(tool=>tool.name))===JSON.stringify(expected),'tools/list did not expose exactly four renamed tools')
 assert(JSON.stringify(Object.keys(tools[0].inputSchema.properties).sort())===JSON.stringify(['description','prompt','model','subagent_type','run_in_background','name','isolation','write','writable_roots','team_name','mode'].sort()),'ExternalSubagent input keys changed')
 const props=tools[0].inputSchema.properties
 assert(props.write.type==='boolean'&&props.write.default===false&&props.writable_roots.type==='array'&&props.writable_roots.items.type==='string'&&!tools[0].inputSchema.required.includes('write')&&props.writable_roots.description.startsWith('In write mode the MCP working directory is always writable as well (native Codex behavior); writable_roots grants additional directories.'),'write parameters changed')
 for(const old of ['Agent','SendMessage','TaskStop','ListAgents']){
  const result=await rpc('tools/call',{name:old,arguments:{}})
  assert(result.isError===true&&result.content.some(part=>part.text?.includes('unknown tool')),`${old} remained accepted`)
 }
 const listed=await rpc('tools/call',{name:'listExternalSubagentSessions',arguments:{}})
 assert(!listed.isError&&listed.content.some(part=>part.text?.includes('No AgentX ExternalSubagents in MCP connection connection-')),'new session listing did not return current empty connection')
 console.log(JSON.stringify({passed:true,binary,tools:expected,oldNamesRejected:true,noWorkCreated:true,server:'AgentX',protocol:init.protocolVersion,evidence:home}))
}finally{clearTimeout(timer);child.stdin.end();await new Promise(resolve=>child.once('close',resolve))}
