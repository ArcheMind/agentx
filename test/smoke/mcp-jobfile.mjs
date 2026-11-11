import {spawn} from 'node:child_process'
import {mkdtemp, mkdir, writeFile, readFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {join, resolve} from 'node:path'

const home=await mkdtemp(join(tmpdir(),'agentx-jobfile-'))
const binary=resolve(process.env.AX_MCP_BINARY??process.argv[2]??'bin/ax')
await mkdir(join(home,'bin'))
await writeFile(join(home,'bin','pi'),`#!/usr/bin/env node
(async()=>{
if(process.argv.includes('auth')){console.log(JSON.stringify({status:'ready',provider:'test',authType:'api_key'}));return}
let input='';for await (const chunk of process.stdin) input+=chunk;
await new Promise(r=>setTimeout(r,1500));
console.log(JSON.stringify({type:'session',id:'test-session'}));
console.log(JSON.stringify({type:'message_end',message:{role:'assistant',provider:'test',model:'jobfile',stopReason:'stop',content:[{type:'text',text:'JOBFILE_TOKEN_8f326c'}]}}));
console.log(JSON.stringify({type:'agent_end'}));
console.log(JSON.stringify({type:'agent_settled',aborted:false}));
})().catch(error=>{console.error(error);process.exitCode=1})
`,{mode:0o700})
const dir=join(home,'.agentx','presets','jobfile-test');await mkdir(dir,{recursive:true})
await writeFile(join(dir,'PRESET.md'),'---\nagent: pi\nmodel: test/jobfile\n---\n\nJob file integration test fixture.\n')
const child=spawn(binary,['mcp','serve'],{env:{...process.env,AX_SUBAGENT_HOME:home,AX_LOG:'debug',PI_CODING_AGENT_DIR:join(home,'pi-agent'),PATH:join(home,'bin')+':'+process.env.PATH},stdio:['pipe','pipe','pipe']})
let next=0,pending='',stderr=''
const requests=new Map()
child.stderr.on('data',chunk=>stderr+=chunk)
child.stdout.on('data',chunk=>{pending+=chunk;let end;while((end=pending.indexOf('\n'))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);const event=JSON.parse(line);if(event.id!==undefined){const request=requests.get(event.id);requests.delete(event.id);event.error?request.reject(Error(JSON.stringify(event.error))):request.resolve(event.result)}}})
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=++next;requests.set(id,{resolve,reject});child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')})}
function assert(value,message){if(!value)throw Error(message)}
const timer=setTimeout(()=>child.kill('SIGTERM'),30000)
try{
 const init=await rpc('initialize',{protocolVersion:'2024-11-05',capabilities:{},clientInfo:{name:'jobfile-smoke',version:'1'}})
 assert(!init.capabilities.experimental,'channel capability still advertised')
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n')
 const started=await rpc('tools/call',{name:'StartExternalSubagent',arguments:{description:'jobfile',prompt:'Return the token',model:'jobfile-test',run_in_background:true}})
 assert(!started.isError,'start failed: '+JSON.stringify(started))
 const text=started.content[0].text
 const path=text.match(/^Result file[^\n]*: (\/\S+\.json)$/m)?.[1]
 assert(path,'response lacks result file path: '+text)
 assert(text.includes(`until [ -f "${path}" ]; do sleep 1; done; cat "${path}"`),'response lacks wait command')
 assert(!await readFile(path).catch(()=>null),'result file existed before completion')
 let data
 for(let i=0;i<200&&!data;i++){data=await readFile(path,'utf8').catch(()=>null);if(!data)await new Promise(r=>setTimeout(r,100))}
 assert(data,'result file did not appear')
 const doc=JSON.parse(data)
 assert(doc.meta.report_origin==='agentx-subagent'&&doc.meta.status==='completed'&&doc.meta.agent_id&&path.endsWith(doc.meta.task_id+'.json'),'unexpected meta '+data)
 assert(doc.content.includes('JOBFILE_TOKEN_8f326c'),'report missing from content')
 assert(stderr.includes('debug job result file path='+path),'debug log missing result file path')
 console.log(JSON.stringify({passed:true,binary,resultFile:path,status:doc.meta.status,evidence:home}))
}finally{clearTimeout(timer);child.stdin.end();await new Promise(resolve=>child.once('close',resolve))}
