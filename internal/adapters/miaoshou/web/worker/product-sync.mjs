const endpoint='/api/platform/tiktok/item/item/searchItemList';
const scopes=[['onsale','在售中'],['forsale','已下架'],['suspendSales','暂停销售'],['deleted','已删除']];
export async function syncProducts(s,pause=ms=>new Promise(r=>setTimeout(r,ms))){
 const job=s.sync;let page;const seen=new Set();let expectedStatus='',selectedShops=null;
 const check=()=>{if(job.cancel)throw new Error('已停止');if(!s.context)throw new Error('浏览器已关闭')};
 const spacing=()=>pause(3000+Math.floor(Math.random()*2000));
 try{
  job.step='打开店铺产品';page=await s.context.newPage();await page.setViewportSize({width:1728,height:1000});
  async function capture(action,expectedPage){
   check();const failures=s.archiveFailures||0;
   const responsePromise=page.waitForResponse(r=>new URL(r.url()).pathname===endpoint,{timeout:60000});
   const [r]=await Promise.all([responsePromise,action()]);await r.finished();await Promise.all([...s.pending]);
   if((s.archiveFailures||0)!==failures||s.state==='archive_error')throw new Error('原始响应保存失败');
   const d=await r.json(),params=new URLSearchParams(r.request().postData());
   if(r.status()!==200||d.result!=='success'||!Array.isArray(d.itemList))throw new Error('未返回有效产品列表，请检查登录及权限');
   d.page=Number(params.get('pageNo'));d.pageSize=Number(params.get('pageSize'));d.status=params.get('status');
   if(expectedStatus&&d.status!==expectedStatus)throw new Error('产品状态范围不一致');
   if(expectedPage&&d.page!==expectedPage)throw new Error('产品页码不一致');
   if(!Number.isSafeInteger(Number(d.total))||Number(d.total)<0||!Number.isSafeInteger(d.pageSize)||d.pageSize<=0)throw new Error('产品分页数据无效');
   const shops=[...params].filter(([k])=>/^shopId(?:\[\d+\])?$/.test(k)).map(([,v])=>v).sort();
   if(selectedShops!==null&&JSON.stringify(shops)!==selectedShops)throw new Error('同步期间店铺筛选发生变化');
   selectedShops=JSON.stringify(shops);job.shops=shops;return d;
  }
  await page.goto('https://erp.91miaoshou.com/tiktok/item/item',{waitUntil:'domcontentloaded'});
  await page.getByRole('button',{name:'重置',exact:true}).waitFor({timeout:45000});await spacing();
  job.step='重置产品筛选';let d=await capture(()=>page.getByRole('button',{name:'重置',exact:true}).click(),1);
  if(d.pageSize!==200){job.step='设置产品每页 200 条';await spacing();await page.locator('.el-pagination__sizes input').click();
   d=await capture(()=>page.getByRole('listitem').filter({hasText:/^200条\/页$/,visible:true}).click(),1);
   if(d.pageSize!==200)throw new Error('产品每页条数设置未生效');
  }
  for(const [status,label] of scopes){
   expectedStatus=status;job.group=label;
   if(d.status!==status){await spacing();job.step=`切换${label}`;d=await capture(()=>page.locator('label').filter({has:page.locator(`input[type=radio][value="${status}"]`)}).click(),1)}
   let previousTotal=Number(d.total);
   for(;;){
    check();if(Number(d.total)!==previousTotal)job.notice='产品总数有变化，已继续采集；后续同步可补齐期间发生的变更。';previousTotal=Number(d.total);
    for(const p of d.itemList){if(!p.platform||!p.shopId||!p.platformItemId)throw new Error('产品关联标识缺失');seen.add(JSON.stringify([p.platform,p.shopId,p.platformItemId]))}
    Object.assign(job,{page:d.page,pageSize:d.pageSize,pages:Math.max(1,Math.ceil(Number(d.total)/d.pageSize)),total:Number(d.total),collected:seen.size});
    if(job.page>=job.pages)break;if(job.page>=10000)throw new Error('超过产品分页上限');
    await spacing();const next=d.page+1;job.step=`${label}：采集第 ${next} 页`;
    d=await capture(()=>page.locator('.el-pagination .btn-next').click({timeout:15000}),next);
   }
  }
  await spacing();await Promise.all([...s.pending]);check();if(s.state==='archive_error')throw new Error('原始响应保存失败');job.state='completed';job.step='产品采集完成，等待后台入库';
 }catch(e){job.state=job.cancel?'cancelled':'failed';job.errorDetail=String(e.message).slice(0,6000);job.error=`${job.step}失败：${String(e.message).split('\n')[0]}`}
 finally{job.finishedAt=new Date().toISOString();await page?.close().catch(()=>{})}
}
