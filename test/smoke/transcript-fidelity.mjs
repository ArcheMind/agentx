import {spawnSync} from 'node:child_process'
import {mkdtemp,mkdir,copyFile,readFile,readdir,writeFile} from 'node:fs/promises'
import {existsSync} from 'node:fs'
import {createRequire} from 'node:module'
import {tmpdir} from 'node:os'
import {dirname,join,resolve} from 'node:path'
import {fileURLToPath} from 'node:url'
import {isDeepStrictEqual} from 'node:util'

const binary=resolve(process.argv[2]??'bin/ax')
const protocolDir=process.argv[3]??''
const fixtures=join(dirname(fileURLToPath(import.meta.url)),'fixtures','transcript-fidelity')
const home=await mkdtemp(join(tmpdir(),'agentx-transcript-fidelity-'))
// Every session store and the pi resume dir are derived from HOME (os.UserHomeDir), so a temp HOME isolates them all.
const env={...process.env,HOME:home,AX_SUBAGENT_HOME:home}
for(const name of ['CLAUDE_CONFIG_DIR','CODEX_HOME','PI_CODING_AGENT_DIR','XDG_STATE_HOME','XDG_DATA_HOME','XDG_CONFIG_HOME'])delete env[name]

const log=[]
function ax(...args){const result=spawnSync(binary,args,{cwd:home,env,encoding:'utf8'});log.push({argv:['ax',...args],status:result.status,stdout:result.stdout,stderr:result.stderr});if(result.status!==0)fail('cli',`ax ${args.join(' ')} exited ${result.status}`);return result.stdout}
function fail(provider,message){console.error(JSON.stringify({passed:false,provider,failure:message,home,cli:log},null,2));process.exit(1)}
const check=(provider,item,value,detail='')=>{if(!value)fail(provider,`${item}${detail?': '+detail:''}`)}
const same=(provider,item,actual,expected)=>check(provider,item,isDeepStrictEqual(actual,expected),`expected ${JSON.stringify(expected)} got ${JSON.stringify(actual)}`)
const jsonl=async path=>(await readFile(path,'utf8')).split('\n').filter(Boolean).map(line=>JSON.parse(line))
const pick=(records,provider,item,predicate)=>{const found=records.filter(predicate);check(provider,item,found.length===1,`expected exactly one record, found ${found.length}`);return found[0]}

let validate=null,schema='skipped: no agentx-protocol module dir'
if(protocolDir){
  const require=createRequire(join(protocolDir,'package.json'))
  try{
    const Ajv2020=require('ajv/dist/2020.js'),addFormats=require('ajv-formats')
    const ajv=new (Ajv2020.default??Ajv2020)({strict:false,allErrors:true});(addFormats.default??addFormats)(ajv)
    validate=ajv.compile(JSON.parse(await readFile(join(protocolDir,'schema','unified-transcript.schema.json'),'utf8')))
    schema='validated against '+join(protocolDir,'schema','unified-transcript.schema.json')
  }catch(error){schema=`skipped: ajv not resolvable from ${protocolDir} (${error.code??error.message})`}
}

async function roundTrip(provider,id,nativePath,locate){
  await mkdir(dirname(join(home,nativePath)),{recursive:true})
  await copyFile(join(fixtures,provider+'.jsonl'),join(home,nativePath))
  const exported=ax('--json','session','export',id,'--source',provider)
  const transcript=JSON.parse(exported)
  if(validate&&!validate(transcript))fail(provider,'export violates unified-transcript schema: '+JSON.stringify(validate.errors))
  check(provider,'export id',transcript.id===id&&transcript.provider===provider,`${transcript.id}/${transcript.provider}`)
  const file=join(home,provider+'-transcript.json')
  await writeFile(file,exported)
  const imported=JSON.parse(ax('--json','session','import',provider,file))
  check(provider,'import agent',imported.agent===provider,JSON.stringify(imported))
  const written=await locate(imported.session)
  check(provider,'import landed inside isolated HOME',written?.startsWith(home+'/'),String(written))
  return {source:await jsonl(join(home,nativePath)),transcript,output:await jsonl(written),written}
}

async function findFile(root,suffix){
  if(!existsSync(root))return undefined
  for(const entry of await readdir(root,{recursive:true}))if(entry.endsWith(suffix))return join(root,entry)
}

const results={}
const ws='/tmp/agentx-fidelity-ws'

{
  const P='claude',id='5f0c1e2a-7b3d-4c8e-9a1f-2d3e4f5a6b7c'
  // Claude import writes a fresh session id into the project dir derived from the transcript workspace.
  const {source,transcript,output,written}=await roundTrip(P,id,`.claude/projects/${ws.replaceAll('/','-')}/${id}.jsonl`,token=>findFile(join(home,'.claude','projects'),token+'.jsonl'))
  same(P,'export roles (attachment dropped, system kept)',transcript.messages.map(m=>m.role),['user','assistant','tool','assistant','tool','user','system','assistant'])
  const blocks=records=>records.filter(r=>r.type==='assistant').flatMap(r=>r.message.content)
  const results_=records=>records.filter(r=>r.type==='user'&&Array.isArray(r.message.content)).flatMap(r=>r.message.content).filter(b=>b.type==='tool_result')
  const srcThinking=blocks(source).find(b=>b.type==='thinking'),outThinking=blocks(output).filter(b=>b.type==='thinking')
  same(P,'thinking block with signature',outThinking,[srcThinking])
  same(P,'tool_use blocks',blocks(output).filter(b=>b.type==='tool_use'),blocks(source).filter(b=>b.type==='tool_use'))
  const [srcStructured,srcBare]=results_(source),[outStructured,outBare]=results_(output)
  check(P,'tool_result structured content stays an array',Array.isArray(outStructured?.content),JSON.stringify(outStructured))
  same(P,'tool_result structured content',outStructured,srcStructured)
  check(P,'tool_result bare string content stays a string',typeof outBare?.content==='string',JSON.stringify(outBare))
  same(P,'tool_result bare string content',outBare,srcBare)
  const image=records=>records.filter(r=>r.type==='user'&&Array.isArray(r.message.content)).flatMap(r=>r.message.content).filter(b=>b.type==='image')
  same(P,'user image source',image(output),image(source))
  const textOf=records=>records.filter(r=>r.type==='user'&&Array.isArray(r.message.content)).flatMap(r=>r.message.content).filter(b=>b.type==='text')
  same(P,'user text beside image',textOf(output),textOf(source))
  same(P,'user string prompt',output.filter(r=>r.type==='user'&&typeof r.message.content==='string').map(r=>r.message.content),source.filter(r=>r.type==='user'&&typeof r.message.content==='string').map(r=>r.message.content))
  const srcSystem=pick(source,P,'source system record',r=>r.type==='system'),outSystem=pick(output,P,'system informational record',r=>r.type==='system')
  same(P,'system record subtype and text',{subtype:outSystem.subtype,content:outSystem.content},{subtype:'informational',content:srcSystem.content})
  check(P,'attachment dropped by design',!output.some(r=>r.type==='attachment'))
  same(P,'assistant final text',blocks(output).filter(b=>b.type==='text'),blocks(source).filter(b=>b.type==='text'))
  results[P]={written,records:output.length}
}

{
  const P='codex',id='01a15000-1111-7222-8333-944455556666'
  const {source,transcript,output,written}=await roundTrip(P,id,`.codex/sessions/2026/10/20/rollout-2026-10-20T11-00-00-${id}.jsonl`,token=>findFile(join(home,'.codex','sessions'),token+'.jsonl'))
  same(P,'export roles (event_msg dropped, developer is system)',transcript.messages.map(m=>m.role),['system','user','assistant','tool','assistant','tool','assistant'])
  const items=records=>records.filter(r=>r.type==='response_item').map(r=>r.payload)
  const src=items(source),out=items(output)
  const one=(list,item,predicate)=>pick(list.map(payload=>({payload})),P,item,r=>predicate(r.payload)).payload
  const srcDev=one(src,'source developer message',p=>p.type==='message'&&p.role==='developer'),outDev=one(out,'developer message',p=>p.type==='message'&&p.role==='developer')
  same(P,'developer message content',outDev.content,srcDev.content)
  const srcReasoning=one(src,'source reasoning',p=>p.type==='reasoning'),outReasoning=one(out,'reasoning',p=>p.type==='reasoning')
  same(P,'reasoning empty summary',outReasoning.summary,[])
  check(P,'reasoning encrypted_content byte-exact',outReasoning.encrypted_content===srcReasoning.encrypted_content,JSON.stringify(outReasoning))
  const srcFn=one(src,'source function_call',p=>p.type==='function_call'),outFn=one(out,'function_call',p=>p.type==='function_call')
  same(P,'function_call name/call_id',{name:outFn.name,call_id:outFn.call_id},{name:srcFn.name,call_id:srcFn.call_id})
  same(P,'function_call arguments (JSON-equal)',JSON.parse(outFn.arguments),JSON.parse(srcFn.arguments))
  const srcFnOut=one(src,'source function_call_output',p=>p.type==='function_call_output'),outFnOut=one(out,'function_call_output',p=>p.type==='function_call_output')
  same(P,'function_call_output',{call_id:outFnOut.call_id,output:outFnOut.output},{call_id:srcFnOut.call_id,output:srcFnOut.output})
  const srcCustom=one(src,'source custom_tool_call',p=>p.type==='custom_tool_call'),outCustom=one(out,'custom_tool_call record type',p=>p.type==='custom_tool_call')
  check(P,'custom_tool_call not rewritten as function_call',!out.some(p=>p.type==='function_call'&&p.call_id===srcCustom.call_id))
  same(P,'custom_tool_call exec freeform input',{name:outCustom.name,call_id:outCustom.call_id,input:outCustom.input},{name:'exec',call_id:srcCustom.call_id,input:srcCustom.input})
  const srcCustomOut=one(src,'source custom_tool_call_output',p=>p.type==='custom_tool_call_output'),outCustomOut=one(out,'custom_tool_call_output record type',p=>p.type==='custom_tool_call_output')
  check(P,'custom_tool_call_output output stays an array',Array.isArray(outCustomOut.output),JSON.stringify(outCustomOut))
  same(P,'custom_tool_call_output input_text array',{call_id:outCustomOut.call_id,output:outCustomOut.output},{call_id:srcCustomOut.call_id,output:srcCustomOut.output})
  same(P,'user and assistant messages',out.filter(p=>p.type==='message'&&p.role!=='developer').map(p=>[p.role,p.content]),src.filter(p=>p.type==='message'&&p.role!=='developer').map(p=>[p.role,p.content]))
  // The serializer emits its own turn events; the source task_started/task_complete must not leak through.
  check(P,'event_msg dropped by design',!output.some(r=>r.type==='event_msg'&&['task_started','task_complete'].includes(r.payload.type)))
  results[P]={written,records:output.length}
}

{
  const P='pi',id='01a15000-2222-7333-8444-955566667777'
  // Pi import does not write into ~/.pi; the CLI writes <id>.jsonl into ~/.local/state/agentx/resume and returns that path.
  const {source,transcript,output,written}=await roundTrip(P,id,`.pi/agent/sessions/--${ws.slice(1).replaceAll('/','-')}--/2026-10-20T12-00-00-000Z_${id}.jsonl`,async token=>{same('pi','import path',token,join(home,'.local','state','agentx','resume',id+'.jsonl'));return token})
  same(P,'export roles (compaction dropped)',transcript.messages.map(m=>m.role),['user','assistant','tool','tool','assistant'])
  check(P,'no synthetic pi-N ids in export',!transcript.messages.some(m=>/^pi-\d+$/.test(m.id)),JSON.stringify(transcript.messages.map(m=>m.id)))
  const msgs=records=>records.filter(r=>r.type==='message')
  const src=msgs(source),out=msgs(output)
  same(P,'record id/parentId chain',out.map(r=>[r.id,r.parentId]),src.map(r=>[r.id,r.parentId]))
  check(P,'no synthetic pi-N ids in native output',!out.some(r=>/^pi-\d+$/.test(r.id)))
  src.forEach(r=>check(P,'fixture message.timestamp differs from record timestamp',new Date(r.message.timestamp).toISOString()!==r.timestamp,r.id))
  same(P,'message.timestamp epoch millis',out.map(r=>r.message.timestamp),src.map(r=>r.message.timestamp))
  const assistant=records=>records.filter(r=>r.message.role==='assistant')
  same(P,'assistant stopReason',assistant(out).map(r=>r.message.stopReason),['toolUse','stop'])
  const calls=records=>assistant(records).flatMap(r=>r.message.content).filter(b=>b.type==='toolCall')
  check(P,'toolCall arguments stay objects',calls(out).length===2&&calls(out).every(b=>b.arguments!==null&&typeof b.arguments==='object'&&!Array.isArray(b.arguments)),JSON.stringify(calls(out)))
  same(P,'toolCall blocks',calls(out),calls(src))
  same(P,'assistant thinking and text blocks',assistant(out).map(r=>r.message.content.filter(b=>b.type!=='toolCall')),assistant(src).map(r=>r.message.content.filter(b=>b.type!=='toolCall')))
  const toolResults=records=>records.filter(r=>r.message.role==='toolResult').map(r=>({toolCallId:r.message.toolCallId,toolName:r.message.toolName,content:r.message.content,isError:r.message.isError}))
  same(P,'toolResult content arrays and isError',toolResults(out),toolResults(src))
  check(P,'toolResult isError:true present',toolResults(out).some(r=>r.isError===true))
  same(P,'user content',out.filter(r=>r.message.role==='user').map(r=>r.message.content),src.filter(r=>r.message.role==='user').map(r=>r.message.content))
  check(P,'compaction dropped by design',!output.some(r=>r.type==='compaction'))
  results[P]={written,records:output.length}
}

console.log(JSON.stringify({passed:true,test:'transcript round-trip fidelity: native fixture -> ax session export -> ax session import -> native file keeps fidelity-critical content',schema,home,results},null,2))
