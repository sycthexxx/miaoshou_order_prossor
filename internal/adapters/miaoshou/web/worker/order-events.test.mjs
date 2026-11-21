import test from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {mkdtemp,readFile,readdir,rm} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {orderTips,socketTips,lookupOrderPackages,attachOrderEvents} from './order-events.mjs';

const base='https://erp.91miaoshou.com';
const orderNumber='587279992876074977';
const tip={source:'new_order',platformOrderSn:orderNumber,orderTipMsgId:'42'};
test('separates new, cancelling, cancelled and filters subaccounts and unrelated socket frames',()=>{
 const rows=[tip,{...tip,source:'after_sales',status:'in_cancel'},{...tip,source:'after_sales',status:'cancelled'},
  {...tip,source:'after_sales',status:'refunding'},{...tip,platformOrderSn:undefined},{...tip,orderTipMsgId:undefined}];
 assert.deepEqual(orderTips(rows).map(x=>x.kind),['new_order','in_cancel','cancelled']);
 const frame={action:'ORDER_TIP_MSG',content:{subAccountId:'2',orderTipMsgList:rows}};
 assert.equal(socketTips(frame,{main:false,sub:'2'}).length,3);
 assert.equal(socketTips(frame,{main:false,sub:'3'}).length,0);
 assert.equal(socketTips(frame,{main:true}).length,3);
 assert.equal(socketTips({messageCode:'HEARTBEAT'},{}).length,0);
});

function mockPage(makeResult) {
 const p=new EventEmitter();
 p.url=()=>base+'/order/package/index';
 const moduleURL=base+'/assets/index-x.js';
 p.locator=()=>({evaluateAll:async()=>[moduleURL]});
 p.request={get:async()=>({ok:()=>true,text:async()=>'function test(e){return a({url:`/manualRelateOrderPackagePurchaseOrders`})}export{a as q}'})};
 p.waitForResponse=(predicate)=>new Promise(resolve=>{
  const listener=r=>{if(predicate(r)){p.off('response',listener);resolve(r);}};
  p.on('response',listener);
 });
 p.calls=[];
 p.evaluate=async(_, {data,url})=>{
  const req={method:()=>'POST',url:()=>base+'/api'+url,postData:()=>new URLSearchParams(data).toString(),frame:()=>({page:()=>p})};
  const body=makeResult(data.page);
  p.calls.push(data);
  p.emit('response',{url:req.url,request:()=>req,text:async()=>body,ok:()=>true});
 };
 return p;
}
const result=(page,total,rows)=>JSON.stringify({result:'success',page,total,packageList:rows});
test('exact lookup preserves long numeric IDs and all split/merged packages, rejects unrelated, empty or repeated pages',async()=>{
 const page=mockPage(n=>result(n,2,[{opOrderPackageId:String(n),subPackageList:[{platformOrderSn:orderNumber}]}]).replace('"'+orderNumber+'"',orderNumber));
 const pkgs=await lookupOrderPackages(page,orderNumber);
 assert.equal(pkgs.length,2);
 assert.equal(pkgs[1].subPackageList[0].platformOrderSn,orderNumber);
 assert.equal(page.calls[0].platformOrderSnsRp,'eq');
 assert.equal(page.calls[1].platformOrderSns,orderNumber);
 for(const fn of [n=>result(n,1,[{opOrderPackageId:1,platformOrderSn:'different'}]),
  n=>result(n,0,[]),n=>result(n,2,[{opOrderPackageId:1,platformOrderSn:orderNumber}])]){
  await assert.rejects(lookupOrderPackages(mockPage(fn),orderNumber));
 }
});
test('persists pending events, resumes on browser reopen, deduplicates HTTP and WS signals and ignores history views',async()=>{
 const dir=await mkdtemp(path.join(os.tmpdir(),'order-events-'));
 const ctx=new EventEmitter();
 const p=mockPage(n=>result(n,1,[{opOrderPackageId:'1',platformOrderSn:orderNumber}]));
 ctx.pages=()=>[p];const session={context:ctx,state:'closed'};
 const state=path.join(dir,'runtime/1/order-event-queue.json');
 const inbox=path.join(dir,'runtime/1/order-events');
 const send=async(url,obj)=>p.emit('response',{url:()=>base+url,ok:()=>true,text:async()=>JSON.stringify({result:'success',...obj})});
 const wait=async check=>{for(let i=0;i<100;i++){if(await check().catch(()=>false))return;await new Promise(r=>setTimeout(r,10))}assert.fail('timed out waiting for persisted state')};
 let monitor=await attachOrderEvents(dir,'1',session);
 try{
  await send('/api/order/purchase/searchOrderTipMsgList',{orderTipMsgList:[{...tip,orderTipMsgId:'old'}]});
  await send('/api/auth/account/getAccountInfo',{accountId:'1',subAccountId:'2'});
  await send('/api/auth/shop/getWsInfo',{wsSocketAddress:'wss://erp.example.test/orders'});
  const ws=new EventEmitter();ws.url=()=>'wss://erp.example.test/orders';p.emit('websocket',ws);
  ws.emit('framereceived',{payload:JSON.stringify({action:'ORDER_TIP_MSG',content:{subAccountId:'2',orderTipMsgList:[tip]}})});
  await send('/api/order/purchase/checkRsyncOrderTipMsg',{orderTipMsgList:[tip]});
  await wait(async()=>JSON.parse(await readFile(state,'utf8')).pending.length===1);
  await wait(async()=>JSON.parse(await readFile(state,'utf8')).batches?.length===2);
  assert.deepEqual(monitor.status().batches.map(({source,newOrders,cancelled,accepted})=>[source,newOrders,cancelled,accepted]),[
   ['http',1,0,0],['websocket',1,0,1]
  ]);
  assert.equal(p.calls.length,0);
  await monitor.stop();
  session.state='verified';monitor=await attachOrderEvents(dir,'1',session);
  assert.equal(monitor.status().batches.length,2);
  await wait(async()=>JSON.parse(await readFile(state,'utf8')).done.length===1);
  const files=await readdir(inbox);
  assert.equal(files.length,1);const event=JSON.parse(await readFile(path.join(inbox,files[0]),'utf8'));
  assert.equal(event.orderNumber,orderNumber);assert.equal(event.packages[0].platformOrderSn,orderNumber);
  await send('/api/order/purchase/checkRsyncOrderTipMsg',{orderTipMsgList:[tip]});
  await monitor.stop();
  assert.equal(p.calls.length,1);
 } finally {await monitor.stop();await rm(dir,{recursive:true,force:true})}
});
