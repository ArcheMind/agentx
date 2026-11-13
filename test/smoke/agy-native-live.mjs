import {spawnSync} from 'node:child_process'
import {mkdtempSync,writeFileSync,readFileSync} from 'node:fs'
import {tmpdir,homedir} from 'node:os'
import {join,resolve} from 'node:path'
import {randomBytes,randomUUID} from 'node:crypto'

const binary=resolve(process.argv[2]??'bin/sm')
const root=mkdtempSync(join(tmpdir(),'sm-agy-native-live-'))
const model=process.env.AX_CROSS_AGY_MODEL??'gemini-3.8-flash-low'
let call=0
const matrix={},failures=[]
function run(command,args,input){
  const r=spawnSync(command,args,{cwd:root,input,encoding:'utf8',timeout:90_000,maxBuffer:16*1024*1024})
  writeFileSync(join(root,`call-${++call}.json`),JSON.stringify({command,args,input,status:r.status,stdout:r.stdout,stderr:r.stderr,error:r.error?.message},null,2))
  if(r.error||r.status!==0)throw new Error(`${command} failed: ${r.error?.message??r.stderr}`)
  return JSON.parse(r.stdout)
}
const marker=()=>`AGY_MEMORY_${randomBytes(12).toString('hex')}`
const user=text=>({id:randomUUID(),role:'user',content:[{type:'text',text}]})
const assistant=content=>({id:randomUUID(),role:'assistant',content})
const body=marker(),thought=marker(),tool=marker(),unsigned=marker()
const cases=[
  {name:'native-unsigned-thinking-observation',observe:true,expected:unsigned,messages:[user('Retain the code in your earlier reasoning.'),assistant([{type:'text',text:unsigned}])],query:'What exact AGY_MEMORY code was stored in the earlier reasoning? Reply with only that code. Do not use tools or read files.'},
  {name:'body-source-free',expected:body,messages:[user('Retain the code from your reply.'),assistant([{type:'text',text:body}])],query:'What exact AGY_MEMORY code did the earlier assistant reply contain? Reply with only that code. Do not use tools or read files.'},
  {name:'foreign-thinking-projection',expected:thought,messages:[user('Retain the code in your reasoning.'),assistant([{type:'thinking',text:`The retained code is ${thought}.`,signature:'foreign-signature'},{type:'text',text:'Stored.'}])],query:'What exact AGY_MEMORY code was stored in the earlier reasoning? Reply with only that code. Do not use tools or read files.'},
  {name:'tool-history-source-free',expected:tool,messages:[user('Remember the historical research tool result.'),assistant([{type:'tool_use',id:'historical_call',name:'mcp_removed_server_research_tool',input:{}}]),{id:randomUUID(),role:'tool',toolUseId:'historical_call',toolName:'mcp_removed_server_research_tool',result:tool}],query:'What exact AGY_MEMORY code did the historical research tool return? Reply with only that code. Do not use tools or read files.'},
  {name:'image-inline-source-free',expected:'blue',messages:[{...user('Retain the attached image.'),content:[{type:'text',text:'Retain the attached image.'},{type:'image',source:{type:'base64',media_type:'image/png',data:'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNgYPgPAAEDAQAIicLsAAAAAElFTkSuQmCC'}}]},assistant([{type:'text',text:'Image stored.'}])],query:'What color filled the earlier attached image? Reply with only the color. Do not use tools or inspect files.'},
]
for(const c of cases){
  try{
    const source={id:randomUUID(),provider:'claude',workspace:root,messages:c.messages,messageCount:c.messages.length}
    const file=join(root,`${c.name}.json`);writeFileSync(file,JSON.stringify(source))
    const imported=run(binary,['--json','session','import','agy',file])
    if(imported.session===source.id)throw new Error('source session ID reused')
    if(c.observe){
      // Equal-length protobuf field replacement leaves SQLite record sizes and B-tree offsets intact.
      const path=join(homedir(),'.gemini','antigravity-cli','conversations',imported.session+'.db')
      const db=readFileSync(path),needle=Buffer.from(c.expected),offset=db.indexOf(needle)
      if(offset<2||db.indexOf(needle,offset+needle.length)!==-1||needle.length>=128||db[offset-2]!==0x0a||db[offset-1]!==needle.length)throw new Error('unsigned native fixture does not contain one expected response field')
      db[offset-2]=0x1a
      writeFileSync(path,db)
    }
    const listed=run(binary,['--json','session','list','--source','agy','--all'])
    if(!JSON.stringify(listed).includes(imported.session))throw new Error('new session missing from catalog before inference')
    const reply=run('agy',['--model',model,'--effort','low','--mode','plan','--print-timeout','45s','--conversation',imported.session,'-p',c.query,'--output-format','json'])
    const answer=(reply.response??'').trim()
    if(c.observe){matrix[c.name]={observedRetrieved:answer===c.expected,answer,conclusion:'Observation only; native unsigned thinking is not assumed to replay across providers.'};continue}
    if(answer.toLowerCase()!==c.expected.toLowerCase())throw new Error(`recall mismatch: expected=${c.expected},actual=${JSON.stringify(answer)}`)
    matrix[c.name]='pass'
  }catch(error){matrix[c.name]='fail';failures.push({case:c.name,error:error.message})}
}
console.log(JSON.stringify({passed:failures.length===0,matrix,failures,evidence:root},null,2))
if(failures.length)process.exitCode=1
