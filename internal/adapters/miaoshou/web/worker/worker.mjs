import http from 'node:http';
import {requestContext as captureRequestContext} from './request-context.mjs';
import {syncAll} from './sync.mjs';
import {syncProducts} from './product-sync.mjs';
import path from 'node:path';
import {mkdir,readFile} from 'node:fs/promises';
import {readJob,jobWriter} from './job-state.mjs';
import {chromium} from 'playwright';
import {archiveResponse,endpoints} from './archive.mjs';
import {writeScopeSnapshot} from './scope-snapshot.mjs';
import {submitPacking} from './packing.mjs';
import {requestWaybill} from './waybill.mjs';
import {lookupLogisticsCompany} from './logistics-company.mjs';
import {lookupPurchase} from './purchase-lookup.mjs';
import {getAmountDetail} from './amount-detail.mjs';
import {getTracking} from './tracking.mjs';
import {attachOrderEvents} from './order-events.mjs';
import {startFirstMileSync} from './first-mile.mjs';
const root=process.env.ORDER_DATA_DIR, accountKey=process.env.ORDER_ACCOUNT_KEY;
const tokenFile=process.env.BROWSER_TOKEN_FILE;
if(!root||!accountKey||!tokenFile)throw new Error('请通过工作台启动账户浏览器容器');
const token=(await readFile(tokenFile,'utf8')).trim();
if(!token)throw new Error('账户浏览器容器缺少访问凭据');
const sessions=new Map();
const orderURL='https://erp.91miaoshou.com/order/package/index?appPackageTab=waitProcess';
function view(s){return s?{state:s.state,error:s.error||'',lastVerifiedAt:s.lastVerifiedAt||null,archives:s.archives||0,sync:s.sync||null,orderEvents:s.orderEvents?.status()||null}: {state:'closed',error:'',lastVerifiedAt:null,archives:0}}
async function login(id){
 let s=sessions.get(id);if(s&&s.state!=='closed'&&s.context){const page=s.context.pages()[0];if(page)await page.bringToFront();return view(s)}
 if(s?.state==='opening')return view(s);
 if(s?.eventsStopping)await s.eventsStopping;
 s={state:'opening',archives:0,pending:new Set(),sync:s?.sync||null};sessions.set(id,s);
 s.writeJob=jobWriter(root,id);
 s.responseArchives=new WeakMap();s.publishScope=snapshot=>writeScopeSnapshot(root,id,snapshot);
 if(!s.sync)s.sync=await readJob(root,id);
 const profile=path.join(root,'profiles',id);await mkdir(profile,{recursive:true,mode:0o700});
 try{
  const options={headless:false,viewport:null,acceptDownloads:false};
  const extension=process.env.ORDER_BROWSER_EXTENSION_DIR;
  if(extension){
   options.ignoreDefaultArgs=['--disable-extensions'];
   options.args=[`--disable-extensions-except=${extension}`,`--load-extension=${extension}`];
  }
  s.context=await chromium.launchPersistentContext(profile,options);
  s.state='waiting_login';
  try{s.orderEvents=await attachOrderEvents(root,id,s)}catch{console.error('[订单消息] 待处理队列无法打开，常规订单同步仍可使用')}
  s.context.on('close',()=>{s.state='closed';s.context=null;s.firstMile?.stop();s.eventsStopping=s.orderEvents?.stop().catch(()=>console.error('[订单消息] 关闭队列失败'))});
  s.context.on('response',response=>{
   const job=(async()=>{
    const u=new URL(response.url());if(u.origin!=='https://erp.91miaoshou.com'||!endpoints.has(u.pathname)||s.orderEvents?.handles(response))return;
    const req=response.request();const body=await response.body();
    const requestContext=captureRequestContext(req);

    const archived=await archiveResponse(root,id,{endpoint:u.pathname,status:response.status(),contentType:response.headers()['content-type']||'',requestBody:req.postData()||'',requestContext,body});
    s.responseArchives.set(response,archived);
    s.archives++;
    let data;try{data=JSON.parse(body.toString('utf8'))}catch{return}
    if(u.pathname.endsWith('/searchOrderPackageList')&&response.status()===200&&data.result==='success'&&Array.isArray(data.packageList)){
     s.state='verified';s.lastVerifiedAt=new Date().toISOString();s.error='';
    }else if(response.status()===401||response.status()===403){s.state='waiting_login';s.error='会话或访问权限需要检查'}
   })().catch(()=>{s.archiveFailures=(s.archiveFailures||0)+1;s.error='原始响应保存失败，本次采集不可视为成功';s.state='archive_error'});
   s.pending.add(job);job.finally(()=>s.pending.delete(job));
  });
  const page=s.context.pages()[0]||await s.context.newPage();
  page.on('framenavigated',frame=>{if(frame===page.mainFrame()&&/login|signin/i.test(frame.url())){s.state='waiting_login';s.lastVerifiedAt=null}});
  await page.goto(orderURL,{waitUntil:'domcontentloaded',timeout:45000});
  if(extension)s.firstMile=startFirstMileSync(s,path.join(root,'runtime',id));
  return view(s);
 }catch(e){s.error=String(e.message).split('\n')[0];s.state=s.context?'waiting_login':'closed';return view(s)}
}
const server=http.createServer(async(req,res)=>{
 res.setHeader('Content-Type','application/json');
 const reply=(status,data)=>{res.writeHead(status);res.end(JSON.stringify(data))};
 if(req.headers.authorization!=='Bearer '+token)return reply(403,{error:'禁止访问'});
 const match=req.url.match(/^\/accounts\/([a-f0-9]{32}|[1-9][0-9]*)\/(login|close|status|sync|sync-pending|sync-products|stop-sync|packing|waybill|purchase-lookup|logistics-company|amount-detail|tracking|first-mile-start)$/);if(!match)return reply(404,{error:'接口不存在'});
 const [,id,action]=match;
 if(id!==accountKey)return reply(404,{error:'账户不属于当前容器'});
 try{if(action==='status'&&req.method==='GET'){let s=sessions.get(id);if(!s){s={state:'closed',sync:await readJob(root,id)};sessions.set(id,s)}return reply(200,view(s))}
 if(sessions.get(id)?.packing&&action!=='status')return reply(409,{error:'正在提交代打包，请等待结果'});
 if(action==='first-mile-start'&&req.method==='POST'){
  const session=sessions.get(id);
  if(!session?.context||session.state!=='verified')return reply(409,{error:'请先登录妙手并打开订单处理页'});
  if(!session.firstMile)return reply(409,{error:'请先在账户容器启用妙手采购插件'});
  const result=await session.firstMile.runNow();
  return reply(result.error?409:202,result);
 }
 if(action==='tracking'&&req.method==='POST'){
  const session=sessions.get(id);
  if(!session?.context||session.state!=='verified')return reply(409,{error:'请先在账户管理登录妙手并打开订单页面'});
  let body='';for await(const chunk of req)body+=chunk;
  return reply(200,await getTracking({session,payload:JSON.parse(body)}));
 }
 if(action==='logistics-company'&&req.method==='POST'){
  const session=sessions.get(id);
  if(!session?.context)return reply(409,{error:'请先在账户管理打开并登录妙手浏览器'});
  let body='';for await(const chunk of req)body+=chunk;
  return reply(200,await lookupLogisticsCompany({session,payload:JSON.parse(body)}));
 }
 if((action==='packing'||action==='waybill'||action==='purchase-lookup'||action==='amount-detail')&&req.method==='POST'){
  const s=sessions.get(id);
  if(!s?.context||s.state!=='verified')return reply(409,{error:'请先在账户管理打开浏览器并完成订单读取验证'});
  if(s.sync?.state==='running')return reply(409,{error:'请等待网页同步完成，再提交代打包'});
  s.packing=true;
  try{let body='';for await(const chunk of req)body+=chunk;const payload=JSON.parse(body);return reply(200,await (action==='packing'?submitPacking:action==='waybill'?requestWaybill:action==='purchase-lookup'?lookupPurchase:getAmountDetail)({root,id,session:s,payload}))}
  finally{s.packing=false}
 }
 if((action==='sync'||action==='sync-pending'||action==='sync-products')&&req.method==='POST'){const s=sessions.get(id);if(!s?.context||s.state!=='verified')return reply(409,{error:'请先在账户管理打开浏览器并完成订单读取验证'});if(s.sync?.state==='running'&&s.sync.scope!==(action==='sync-products'?'products':action==='sync-pending'?'pending':'all'))return reply(409,{error:'当前账户已有同步任务，请等待完成或先停止'});if(s.sync?.state!=='running'){s.sync={scope:action==='sync-products'?'products':action==='sync-pending'?'pending':'all',state:'running',page:0,pages:0,total:0,collected:0,startedAt:new Date().toISOString()};await s.writeJob(s.sync);void runSync(s,action==='sync-products')}return reply(202,view(s))}
 if(action==='stop-sync'&&req.method==='POST'){const s=sessions.get(id);if(s?.sync?.state==='running')s.sync.cancel=true;return reply(200,view(s))}
 if(action==='login'&&req.method==='POST')return reply(200,await login(id));
 if(action==='close'&&req.method==='POST'){const s=sessions.get(id);if(s?.sync?.state==='running'||s?.firstMileWriting)return reply(409,{error:'浏览器正在同步，请稍后关闭'});if(s?.context){await Promise.all([...s.pending]);await s.context.close()}return reply(200,view(s))}
 return reply(405,{error:'请求方法不支持'});
 }catch{reply(500,{error:'浏览器操作失败，请重新打开登录窗口'})}
});
server.listen(Number(process.env.BROWSER_PORT),'0.0.0.0',()=>console.log('浏览器登录服务已启动'));
async function shutdown(){for(const s of sessions.values()){if(s.sync?.state==='running'){s.sync.cancel=true;s.sync.state='failed';s.sync.error='浏览器进程关闭，同步中断；请重新同步。';await s.writeJob?.(s.sync).catch(()=>{})}await Promise.all([...(s.pending||[])]);await s.context?.close().catch(()=>{});await s.eventsStopping}server.close();process.exit(0)}
process.on('SIGTERM',shutdown);process.on('SIGINT',shutdown);

async function runSync(s,products){
 const timer=setInterval(()=>{s.writeJob(s.sync).catch(()=>{s.sync.cancel=true;s.error='同步任务状态保存失败';console.error('同步任务状态保存失败')})},1000);
 try{await (products?syncProducts(s):syncAll(s))}catch{ s.sync.state='failed';s.sync.error='同步意外中断，请重新同步';s.sync.finishedAt=new Date().toISOString() }finally{clearInterval(timer);await s.writeJob(s.sync).catch(()=>console.error('同步结果保存失败'))}
}
