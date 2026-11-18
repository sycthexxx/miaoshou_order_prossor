import {observeOutbound} from './outbound.mjs';
import {completeScope} from './scope-snapshot.mjs';
// Use the site's own navigation so session headers and browser signatures stay native.
export async function syncAll(s, pause = ms => new Promise(r=>setTimeout(r,ms))) {
 const snapshots=[],allSeen=new Set(),job=s.sync;
 let completed=await syncScope(s,pause,job.scope==='pending',allSeen,snapshots);
 if(job.scope==='all'&&completed){
  job.step='校准待处理订单范围';
  await pause(3000+Math.floor(Math.random()*2000));
  completed=await syncScope(s,pause,true,allSeen,snapshots);
 }
 if(!completed)return;
 try{
  if(job.cancel)throw new Error('已停止');
  for(const snapshot of snapshots)await s.publishScope?.(snapshot);
  job.state='completed';job.step='采集完成';
 }catch(e){job.state=job.cancel?'cancelled':'failed';job.error='保存完整订单范围失败：'+e.message}
 job.finishedAt=new Date().toISOString();
}
async function syncScope(s,pause,pending,allSeen,snapshots){
 const job=s.sync;let page;let groupKey='';let completed=false;
 const check=()=>{if(job.cancel)throw new Error('已停止');if(!s.context)throw new Error('浏览器已关闭')};
 try {
  check();job.step='打开订单页面';
  page=await s.context.newPage();
  await page.setViewportSize({width:1728,height:1000});
  async function capture(action,expected){
   check();const failures=s.archiveFailures||0;
   const outbound=observeOutbound(page);
   try {
   const waiting=page.waitForResponse(r=>r.url().includes('/api/order/package/render_list/searchOrderPackageList')&&(!expected||Number(new URLSearchParams(r.request().postData()).get('page'))===expected),{timeout:45000});
   const [response]=await Promise.all([waiting,action()]);
   await response.finished();
   // The response listener registers persistence synchronously before this continuation.
   await Promise.all([...s.pending]);
   if((s.archiveFailures||0)!==failures||s.state==='archive_error')throw new Error(s.error);
   const data=await response.json();
   if(response.status()!==200||data.result!=='success'||!Array.isArray(data.packageList))throw new Error('妙手未返回有效订单，请检查登录或验证提示');
   const params=new URLSearchParams(response.request().postData());
   if(params.get('appPackageTab')!==(pending?'waitProcess':'all')||params.get('source')!==(pending?'orderProcess':'orderHistory'))throw new Error('返回的订单范围与同步任务不一致，已停止');
   data.purchaseTab=params.get('purchaseTab')||'';
   if(groupKey&&data.purchaseTab!==groupKey)throw new Error('待处理分组发生变化，已停止');
   if(!Number.isSafeInteger(Number(data.total))||Number(data.total)<0)throw new Error('订单总数无效');
   if(!Number.isSafeInteger(Number(data.pageSize))||Number(data.pageSize)<=0)throw new Error('每页条数无效');
   if(expected&&Number(data.page)!==expected)throw new Error('分页不一致，已停止');
   await outbound.wait(data.packageList.map(p=>p.opOrderPackageId));
   await Promise.all([...s.pending]);
   if((s.archiveFailures||0)!==failures||s.state==='archive_error')throw new Error(s.error);
   data.archiveFile=s.responseArchives?.get(response)?.filename;
   return data;
   } finally {outbound.stop()}
  }
  job.step='加载订单列表';
  await page.goto(pending?'https://erp.91miaoshou.com/order/package/index?appPackageTab=waitProcess':'https://erp.91miaoshou.com/order/package/all?appPackageTab=all',{waitUntil:'domcontentloaded'});
  await page.getByRole('button',{name:'重置',exact:true}).waitFor({timeout:45000});
  await pause(3000);
  job.step='重置订单筛选';
  let data=await capture(()=>page.getByRole('button',{name:'重置',exact:true}).click());
  if(pending&&data.purchaseTab!=='waitPurchase'){
   await pause(3000+Math.floor(Math.random()*2000));
   job.step='切换未出货分组';
   data=await capture(()=>page.locator('label').filter({has:page.locator('input[type=radio][value=waitPurchase]')}).click(),1);
  }
  if(pending&&data.purchaseTab!=='waitPurchase')throw new Error('未能确认未出货分组');
  // Select a supported UI page size, preserving the site's native request environment.
  if(Number(data.pageSize)!==200){
   job.step='设置每页 200 条';await pause(3000);
   await page.locator('#appScrollContainer').getByText(`${data.pageSize}条/页`,{exact:true}).click({timeout:15000});
   data=await capture(()=>page.getByRole('listbox').filter({visible:true}).getByRole('option',{name:'200条/页',exact:true}).click({timeout:15000}),1);
   if(Number(data.pageSize)!==200)throw new Error('每页条数设置未生效');
  }
  job.pageSize=Number(data.pageSize);
  for(let group=0;group<(pending?2:1);group++){
   job.group=pending?(group===0?'未出货':'已出货'):'全部状态';
   groupKey=pending?data.purchaseTab:'';
   if(pending&&!groupKey)throw new Error('缺少待处理分组标识');
   if(Number(data.page)!==1)throw new Error('起始页不是第一页');
   const seen=new Set();let scopePages=new Map();let previousTotal=Number(data.total),changed=false;
   const record=()=>{
    scopePages.set(Number(data.page),data);
    if(Number(data.total)!==previousTotal){changed=true;job.notice='同步期间订单有变化，已按最新总数继续，结束后补扫；本次不是固定快照。'}
    previousTotal=Number(data.total);
    for(const p of data.packageList){if(!p.opOrderPackageId)throw new Error('包裹标识缺失');seen.add(String(p.opOrderPackageId));allSeen.add(String(p.opOrderPackageId))}
    job.page=Number(data.page);job.total=Number(data.total);job.collected=allSeen.size;job.pageSize=Number(data.pageSize);job.pages=Math.max(1,Math.ceil(job.total/job.pageSize));
    if(!Number.isInteger(job.page)||job.page<1)throw new Error('分页参数无效');
   };
   for(let pass=0;pass<2;pass++){
    job.pass=pass+1;if(pass>0)scopePages=new Map();
    for(;;){
     check();const before=seen.size;record();
     if(pass===0&&seen.size===before&&data.packageList.length)changed=true;
     if(job.page>=job.pages)break;
     if(job.page>=10000)throw new Error('超过单轮分页上限，请缩小范围后重试');
     await pause(3000+Math.floor(Math.random()*2000));
     const next=job.page+1;job.step=`${job.group}：${pass?'补扫':'采集'}第 ${next} 页`;
     // Keyboard activation avoids the order-message floating panel intercepting clicks.
     data=await capture(()=>page.getByRole('listitem',{name:`第 ${next} 页`,exact:true}).press('Enter',{timeout:15000}),next);
    }
    if(seen.size!==job.total)changed=true;
    // Refresh the head after paging to pick up new orders inserted ahead of page 1.
    if(pass===0&&job.page>1){
     await pause(3000+Math.floor(Math.random()*2000));job.step=`${job.group}：补扫首页`;
     const before=seen.size;
     data=await capture(()=>page.getByRole('listitem',{name:'第 1 页',exact:true}).press('Enter',{timeout:15000}),1);
     record();if(seen.size>before)changed=true;
    }else if(pass===0&&changed){
     // Single-page lists are already fully represented by their latest response.
     break;
    }
    if(pass===0&&changed&&job.pages>1){job.notice='订单动态变化或分页重叠，正在补扫一轮；持续变化的数据可能需要下次同步补齐。';continue}
    break;
   }
   const snapshot=completeScope(pending?groupKey:'all',scopePages);if(snapshot)snapshots.push(snapshot);
   else job.notice='已采集原始响应，但分页变化或重叠，未替换权威范围；请再次同步。';
   if(changed)job.notice='已完成分页及补扫。期间订单有变化，数据不是固定快照；后续同步继续更新补齐。';
   if(pending&&group===0){
    groupKey='';await pause(3000+Math.floor(Math.random()*2000));
    job.step='切换已出货分组';
    data=await capture(()=>page.locator('label').filter({has:page.locator('input[type=radio][value=hasPurchase]')}).click(),1);
    if(!data.purchaseTab||data.purchaseTab==='waitPurchase')throw new Error('未能切换到已出货分组');
   }
  }
  await pause(3000);
  await Promise.all([...s.pending]);
  if(s.state==='archive_error')throw new Error(s.error);
  check();
  completed=true;
 }catch(e){job.state=job.cancel?'cancelled':'failed';job.errorDetail=String(e.message).slice(0,6000);job.error=`${job.step||'同步'}失败：${/Timeout|timeout/.test(String(e.message))?'等待页面控件或订单响应超时，请检查妙手是否出现登录、验证或弹窗':String(e.message).split('\n')[0]}`;console.error('[订单同步]',job.scope||'all',job.step,job.errorDetail)}
 finally{if(!completed)job.finishedAt=new Date().toISOString();await page?.close().catch(()=>{})}
 return completed;
}
