import {spawnSync} from 'node:child_process'
import {mkdtempSync,mkdirSync,writeFileSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {basename,join,resolve} from 'node:path'
import {randomBytes,randomUUID} from 'node:crypto'

const binary=resolve(process.argv[2]??'bin/sm')
const root=mkdtempSync(join(tmpdir(),'sm-transcript-cross-live-'))
const workspace=join(root,'workspace')
mkdirSync(workspace)
const agents=['claude','codex','pi']
const sources=new Map(),failures=[],matrix={}
const model={
  claude:process.env.AX_CROSS_CLAUDE_MODEL??'haiku',
  codex:process.env.AX_CROSS_CODEX_MODEL??'',
  pi:process.env.AX_CROSS_PI_MODEL??'deepseek/deepseek-flash',
}
let call=0
function sm(args,input){
  const result=spawnSync(binary,args,{cwd:workspace,input,encoding:'utf8',timeout:300_000,maxBuffer:16*1024*1024})
  const record={args,input,status:result.status,stdout:result.stdout,stderr:result.stderr,error:result.error?.message}
  writeFileSync(join(root,`call-${++call}.json`),JSON.stringify(record,null,2))
  if(result.error||result.status!==0)throw new Error(`${args.join(' ')} failed: ${result.error?.message??result.stderr?.slice(-1600)}`)
  return JSON.parse(result.stdout)
}
function turn(agent,transcript){
  const args=['session','continue',agent]
  if(model[agent])args.push('--model',model[agent])
  const delta=sm(args,JSON.stringify(transcript))
  if(!delta.messages?.length)throw new Error(`${agent}: no assistant delta`)
  if(delta.messages.some(m=>m.role==='tool'||m.content?.some(b=>b.type==='tool_use')))throw new Error(`${agent}: retrieval must use conversation context without tools`)
  return delta
}
const text=messages=>messages.filter(m=>m.role==='assistant').flatMap(m=>(m.content??[]).filter(b=>b.type==='text').map(b=>b.text)).join('\n').trim()
const signatures=messages=>messages.flatMap(m=>m.content??[]).filter(b=>b.type==='thinking'&&b.signature).map(b=>b.signature)

for(const agent of agents){
  try{
    const nonce=randomBytes(12).toString('hex')
    const prompt={id:randomUUID(),role:'user',content:[{type:'text',text:`Without tools, invent a new 16-character lowercase hexadecimal code. Reply with exactly MEMORY-${nonce}-<code>, replacing <code> with the code you invented. This is a private token to remember for a later conversation turn.`}]}
    console.log(`RUN source ${agent}`)
    const delta=turn(agent,{id:randomUUID(),provider:agent,workspace,messages:[prompt],messageCount:1})
    const marker=text(delta.messages)
    if(!new RegExp(`^MEMORY-${nonce}-[0-9a-f]{16}$`).test(marker))throw new Error(`${agent}: source did not generate a token in the requested format; answer=${JSON.stringify(marker)}`)
    if(JSON.stringify(prompt).includes(marker))throw new Error('complete token must originate in the source reply')
    const exported=sm(['--json','session','export',delta.id,'--source',agent])
    if(!text(exported.messages).includes(marker))throw new Error(`${agent}: generated token missing from native export`)
    sources.set(agent,{exported,marker})
    writeFileSync(join(root,`${agent}-source.json`),JSON.stringify(exported,null,2))
    console.log(`PASS source ${agent}: generated token exported; signed thinking blocks=${signatures(exported.messages).length}`)
  }catch(error){failures.push({source:agent,error:error.message});console.error(`FAIL source ${agent}: ${error.message}`)}
}

for(const src of agents)for(const dst of agents){
  if(src===dst)continue
  const pair=`${src}->${dst}`
  try{
    const source=sources.get(src)
    if(!source)throw new Error(`source ${src} failed; retrieval cannot be verified`)
    console.log(`RUN ${pair}`)
    const file=join(root,`${pair}-input.json`)
    writeFileSync(file,JSON.stringify(source.exported))
    const imported=sm(['--json','session','import',dst,file])
    const session=dst==='pi'?basename(imported.session,'.jsonl'):imported.session
    const converted=sm(['--json','session','export',session,'--source',dst])
    if(signatures(converted.messages).length)throw new Error('foreign thinking signature survived conversion')
    const query={id:randomUUID(),role:'user',content:[{type:'text',text:'What exact MEMORY token did the previous assistant return? Reply with that token only. Retrieve it from the conversation history; do not calculate a new token and do not use tools.'}]}
    if(JSON.stringify(query).includes(source.marker))throw new Error('retrieval prompt leaked the token')
    const messages=[...converted.messages,query]
    const delta=turn(dst,{...converted,workspace,messages,messageCount:messages.length})
    const answer=text(delta.messages)
    if(answer!==source.marker)throw new Error(`retrieval mismatch: expected=${source.marker}, actual=${JSON.stringify(answer)}`)
    matrix[pair]='pass'
    console.log(`PASS ${pair}: target continued and retrieved the exact source-generated token`)
  }catch(error){matrix[pair]='fail';failures.push({pair,error:error.message});console.error(`FAIL ${pair}: ${error.message}`)}
}
console.log(JSON.stringify({passed:failures.length===0,matrix,failures,evidence:root},null,2))
if(failures.length)process.exitCode=1
