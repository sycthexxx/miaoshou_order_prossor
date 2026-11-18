import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {chromium} from 'playwright';
import {requestWaybill,waybillEndpoints} from './waybill.mjs';
import {requestClientExport,submitPacking,packingEndpoint} from './packing.mjs';

const fixtureClient=`
function uiSubmit(e){return request({url:\`\${base}/manualRelateOrderPackagePurchaseOrders\`,data:e,method:'POST'})}
async function request(config){
 const form=new URLSearchParams();
 function append(name,value){if(value!==null&&typeof value==='object'){for(const [key,child] of Object.entries(value))append(name?name+'['+key+']':key,child)}else form.append(name,String(value??''))}
 append('',config.data);
 const response=await fetch('/api'+config.url,{method:config.method,headers:{'Content-Type':'application/x-www-form-urlencoded','x-fixture-native-client':'yes'},body:form});
 const data=await response.json();if(!response.ok||data.result!=='success')throw new Error('ERP error');return data;
}
export{request as clientV2};`;

test('resolves current client export instead of hard coding a minified name',()=>{
 assert.equal(requestClientExport(fixtureClient),'clientV2');
 assert.equal(requestClientExport(fixtureClient.replaceAll('request','$$').replace('clientV2','v3')),'v3');
 assert.throws(()=>requestClientExport('export{}'),/已变化/);
});

test('native browser submission stores complete form and failure response with no retry',async()=>{
 const root=await mkdtemp(path.join(tmpdir(),'packing-test-'));
 const browser=await chromium.launch({headless:true});
 try{
  const context=await browser.newContext();const page=await context.newPage();let writes=0,abort=false;
  const url='https://erp.91miaoshou.com',moduleURL=url+'/assets/index-fixture.js';
  const raw='{ "result":"success", "successCount":0, "failCount":1, "errors":[{"reason":"模拟业务失败"}], "extra":{"keep":true} }\n';
  // Every request is intercepted. This test never connects to the real ERP.
  await context.route('**/*',async route=>{
   const req=route.request();
   if(req.url()===moduleURL)return route.fulfill({contentType:'text/javascript',body:fixtureClient});
   if(req.url()===url+packingEndpoint){writes++;assert.equal(req.headers()['x-fixture-native-client'],'yes');if(abort)return route.abort();return route.fulfill({contentType:'application/json',body:raw})}
   if(req.url().startsWith(url+'/order/'))return route.fulfill({contentType:'text/html',body:'<html><body>fixture</body></html>'});
   return route.abort();
  });
  await page.goto(url+'/order/package/index');
  const payload={purchaseOrderInfoList:[{opOrderPackageId:'2177432687',forwarderId:'0',opOrderItemIdAndPurchaseOrderInfoMap:{'2616118132':[{purchaseOrderSn:'5127601970660025941',purchaseOrderBuyer:'测试买家',purchaseOrderSeller:'测试卖家',isChanged:true}], '2616118129':{purchasePlatform:'other',purchaseOrderPackages:[{purchaseOrderLogisticsName:'中通快递',purchaseOrderWaybillCode:'00123'}]}}}]};
  const result=await submitPacking({root,id:'1',session:{context},payload,resolveClient:async()=>({url:moduleURL,name:requestClientExport(fixtureClient)})});
  assert.equal(writes,1);assert.equal(result.status,200);assert.equal(result.body,raw);
  const dir=path.join(root,result.archive);
  assert.equal(await readFile(path.join(dir,'response.json'),'utf8'),raw);
  assert.deepEqual(JSON.parse(await readFile(path.join(dir,'request.json'),'utf8')),payload);
  const form=new URLSearchParams(await readFile(path.join(dir,'request.form.txt'),'utf8'));
  assert.equal(form.get('purchaseOrderInfoList[0][opOrderItemIdAndPurchaseOrderInfoMap][2616118132][0][purchaseOrderSn]'),'5127601970660025941');
  assert.equal(form.get('purchaseOrderInfoList[0][opOrderItemIdAndPurchaseOrderInfoMap][2616118129][purchaseOrderPackages][0][purchaseOrderWaybillCode]'),'00123');
  assert.equal(form.get('purchaseOrderInfoList[0][opOrderItemIdAndPurchaseOrderInfoMap][2616118132][0][isChanged]'),'true');
  abort=true;
  const lost=await submitPacking({root,id:'1',session:{context},payload,resolveClient:async()=>({url:moduleURL,name:'clientV2'})});
  assert.equal(writes,2);assert.equal(lost.state,'unknown');
  assert.equal(JSON.parse(await readFile(path.join(root,lost.archive,'result.json'),'utf8')).state,'unknown');
  await context.close();
 }finally{await browser.close();await rm(root,{recursive:true,force:true})}
});


test('waybill adapter returns successful responses without archiving them',async()=>{
 const root=await mkdtemp(path.join(tmpdir(),'waybill-test-'));
 const browser=await chromium.launch({headless:true});
 try{
  const context=await browser.newContext(),page=await context.newPage();
  const url='https://erp.91miaoshou.com',moduleURL=url+'/assets/index-fixture.js';
  const requests=[];
  await context.route('**/*',async route=>{
   const req=route.request();
   if(req.url()===moduleURL)return route.fulfill({contentType:'text/javascript',body:fixtureClient});
   if(Object.values(waybillEndpoints).some(endpoint=>url+endpoint===req.url())){
    requests.push({url:req.url(),form:new URLSearchParams(req.postData())});
    return route.fulfill({contentType:'application/json',body:JSON.stringify({result:'success',successCount:1,failCount:0,errors:[],orderPackageDetailInfo:{opOrderPackageId:'2177632828'}})});
   }
   if(req.url()===url+'/order/package/index')return route.fulfill({contentType:'text/html',body:'fixture'});
   return route.abort();
  });
  await page.goto(url+'/order/package/index');
  for(const action of ['inspect','check','apply']){
   const r=await requestWaybill({root,id:'1',session:{context},payload:{action,packageId:'2177632828'},resolveClient:async()=>({url:moduleURL,name:'clientV2'})});
   assert.equal(r.status,200);
   assert.equal(JSON.parse(r.body).result,'success');
   for(const file of ['response.json','response.txt','result.json'])await assert.rejects(readFile(path.join(root,r.archive,file)),{code:'ENOENT'});
  }
  assert.deepEqual(requests.map(r=>r.url),['inspect','check','apply'].map(a=>url+waybillEndpoints[a]));
  assert.equal(requests[0].form.get('opOrderPackageId'),'2177632828');
  for(const r of requests.slice(1)){assert.deepEqual([...r.form],[['opOrderPackageIds[0]','2177632828']])}
 }finally{await browser.close();await rm(root,{recursive:true,force:true})}
});
