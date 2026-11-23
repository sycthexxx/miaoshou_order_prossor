import assert from 'node:assert/strict';
import {test} from 'node:test';
import {firstMilePurchases} from './first-mile.mjs';

test('refreshes eligible 1688 purchase records only and honors a signed copy', () => {
  function purchase(id, status, overrides = {}) {
    return {purchaseOrderId:id, purchaseOrderSn:'5127948757295409' + id.padStart(3,'0'),
      purchasePlatform:'ali1688', purchaseOrderStatus:status, ...overrides};
  }
  const outbound = {
    '1':{purchaseOrders:[
      purchase('1','has_sign'), purchase('2','has_send'), purchase('3','wait_send'),
      purchase('4','wait_pay'), purchase('5','part_send'), purchase('6','cancel'),
      purchase('7',''), purchase('8','has_send',{purchasePlatform:'pdd'}),
      purchase('9','has_send',{isUnrelated:'1'}), purchase('10','has_send',{purchaseOrderSn:purchase('1','has_sign').purchaseOrderSn}),
      purchase('11','has_send',{purchaseOrderSn:5127948757295409109})
    ]},
    '2':{purchaseOrders:[purchase('2','has_send')]}
  };
  assert.deepEqual([...firstMilePurchases(outbound).keys()], ['2','3','4','5']);
});

test('uses hourly default and reads current account settings', async () => {
  const {mkdtemp,writeFile,rm} = await import('node:fs/promises');
  const {tmpdir} = await import('node:os');
  const {join} = await import('node:path');
  const {readFirstMileSettings} = await import('./first-mile.mjs');
  const dir = await mkdtemp(join(tmpdir(),'first-mile-'));
  try {
    assert.deepEqual(await readFirstMileSettings(dir), {enabled:true,intervalMinutes:60});
    await writeFile(join(dir,'first-mile-settings.json'), '{"enabled":false,"intervalMinutes":25}');
    assert.deepEqual(await readFirstMileSettings(dir), {enabled:false,intervalMinutes:25});
    await writeFile(join(dir,'first-mile-settings.json'), '{"enabled":true,"intervalMinutes":0}');
    await assert.rejects(readFirstMileSettings(dir), /设置无效/);
  } finally {
    await rm(dir,{recursive:true,force:true});
  }
});

async function checkRefresh({stop}) {
  const {EventEmitter}=await import('node:events');
  const {syncFirstMile}=await import('./first-mile.mjs');
  const origin='https://erp.91miaoshou.com';
  const listPath='/api/order/package/render_list/searchOrderPackageList';
  const outboundPath='/api/order/package/render_list/getOpOrderPackageIdAndOutboundInfoMap';
  const a='5127948757295409109',b='5127948757295409110';
  const purchase=(id,number,status='has_send')=>({purchaseOrderId:id,purchaseOrderSn:number,purchasePlatform:'ali1688',purchaseOrderStatus:status});
  let stopped=false,readbacks=0,size=100,listPage=1,group='waitPurchase';
  const clicks=[];
  const page=new EventEmitter();
  page.setViewportSize=page.waitForTimeout=page.close=async()=>{};
  page.waitForResponse=predicate=>new Promise(resolve=>page.on('response',function receive(r){if(predicate(r)){page.off('response',receive);resolve(r)}}));
  function response(path,data){
    return {url:()=>origin+path,status:()=>200,ok:()=>true,
      request:()=>({postData:()=> 'appPackageTab=waitProcess&purchaseTab='+group+'&page='+listPage}),json:async()=>data,finished:async()=>{}};
  }
  function emitPage(refreshed){
    page.emit('response',response(listPath,{result:'success',packageList:[{opOrderPackageId:'1'}],total:501,pageSize:size}));
    page.emit('response',response(outboundPath,{result:'success',opOrderPackageIdAndOutboundInfoMap:{
      '1':{purchaseOrders:[purchase('1',a,refreshed?'has_sign':'has_send'),purchase('2',a),...(listPage===2?[purchase('3',b)]:[])]}
    }}));
  }
  page.goto=async url=>{
    assert.equal(new URL(url).searchParams.get('appPackageTab'),'waitProcess');
    assert.equal(new URL(url).searchParams.get('purchaseTab'),'hasPurchase');
    page.emit('response',response(listPath,{result:'success',packageList:[{opOrderPackageId:'99'}],total:1,pageSize:200}));
  };
  page.context=()=>({waitForEvent:async()=>({ok:()=>true,json:async()=>({result:'success'})})});
  function box(id){
    return {filter:()=>box(id),first:()=>box(id),count:async()=>1,locator:()=>({first:()=>({click:async()=>{
      clicks.push(id);
      stopped=stop;
    }})})};
  }
  page.locator=selector=>{
    if(selector==='input[type=radio][value=hasPurchase]')return {waitFor:async()=>{},isChecked:async()=>false};
    if(selector==='label')return {filter:()=>({click:async()=>{group='hasPurchase';emitPage(false);}})};
    if(selector==='#appScrollContainer')return {waitFor:async()=>{},evaluate:async()=>true,getByText:()=>({count:async()=>1,click:async()=>{}})};
    if(selector==='#J_refreshOrderList')return {evaluate:async()=>{readbacks++;emitPage(true)}};
    if(selector==='.J_purchaseOrderInfoSingleBox[data-purchase-order-id]')return {evaluateAll:async()=>listPage===1?['1','2']:['2','3']};
    return box(selector.match(/="(\d+)"\]$/)?.[1]);
  };
  const listbox={filter:()=>listbox,getByRole:(role,options)=>options
    ? {click:async()=>{assert.equal(options.name,'500条/页');size=500;emitPage(false)}}
    : {first:()=>({waitFor:async()=>{}}),allTextContents:async()=>['100条/页','200条/页','500条/页']}};
  page.getByRole=(role,options)=>role==='listbox'?listbox:{
    press:async()=>{assert.equal(options.name,'第 2 页');listPage=2;emitPage(false);}
  };
  const session={state:'verified',context:{newPage:async()=>page}};
  const report=await syncFirstMile(session,undefined,async()=>stopped);
  assert.equal(size,500);
  assert.deepEqual(clicks,stop?['1']:['1','3']);
  assert.equal(readbacks,stop?1:2);
  assert.equal(report.updated,stop?1:2);
  assert.equal(report.failed,0);
  assert.equal(report.items[0].afterStatus,'has_sign');
}

test('a stop received during writeback preserves completed work', () => checkRefresh({stop:true}));
test('selects the largest supported page size and refreshes each purchase number only once across pages', () => checkRefresh({stop:false}));

test('manual start bypasses the disabled schedule and refuses concurrent rounds', async () => {
  const {mkdtemp,readFile,writeFile,rm} = await import('node:fs/promises');
  const {tmpdir} = await import('node:os');
  const {join} = await import('node:path');
  const {startFirstMileSync} = await import('./first-mile.mjs');
  const dir = await mkdtemp(join(tmpdir(),'first-mile-manual-'));
  const session={state:'verified',context:{}};
  let finish;
  const gate = new Promise(resolve=>{finish=resolve;});
  const scheduler=startFirstMileSync(session,dir,async(_session,report)=>{
    await gate;
    report.updated=1;
  });
  try {
    await writeFile(join(dir,'first-mile-settings.json'),'{"enabled":false,"intervalMinutes":60}');
    const run=await scheduler.runNow();
    assert.equal(run.state,'running');
    assert.match((await scheduler.runNow()).error,/正在运行/);
    const file=join(dir,'first-mile-runs',run.id+'.json');
    const record=()=>readFile(file,'utf8').then(JSON.parse);
    assert.equal((await record()).trigger,'manual');
    await writeFile(join(dir,'first-mile-runs',run.id+'.stop'),'');
    finish();
    for(let i=0;i<100 && (await record()).state==='running';i++)await new Promise(resolve=>setTimeout(resolve,5));
    const stopped=await record();
    assert.equal(stopped.state,'stopped');
    assert.equal(stopped.updated,1);
  } finally {
    finish();
    scheduler.stop();
    await rm(dir,{recursive:true,force:true});
  }
});
