import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {chromium} from 'playwright';
import {lookupPurchase,purchaseLookupEndpoint} from './purchase-lookup.mjs';

test('purchase lookup uses read-only GET and archives the full unmodified response',async()=>{
 const root=await mkdtemp(path.join(tmpdir(),'purchase-lookup-'));
 const browser=await chromium.launch({headless:true});
 try{
  const context=await browser.newContext(),page=await context.newPage();let calls=0;
  const raw='{ "result":"success","purchaseOrderInfo":{"purchaseOrderSn":"5127929209710117142","purchaseOrderBuyer":"buyer","purchaseOrderSeller":"seller"},"extra":[1,2] }\n';
  await context.route('**/*',route=>{
   const request=route.request(),url=new URL(request.url());
   if(url.pathname===purchaseLookupEndpoint){
    calls++;assert.equal(request.method(),'GET');assert.equal(url.searchParams.get('purchaseOrderSn'),'5127929209710117142');assert.equal(url.searchParams.get('purchasePlatform'),'ali1688');
    return route.fulfill({contentType:'application/json',body:raw});
   }
   if(url.pathname==='/order/package/index')return route.fulfill({contentType:'text/html',body:'fixture'});
   return route.abort();
  });
  await page.goto('https://erp.91miaoshou.com/order/package/index');
  const result=await lookupPurchase({root,id:'1',session:{context},payload:{orderNumber:'5127929209710117142'}});
  assert.equal(calls,1);assert.equal(result.body,raw);assert.equal(result.status,200);
  assert.equal(await readFile(path.join(root,result.archive,'response.json'),'utf8'),raw);
 }finally{await browser.close();await rm(root,{recursive:true,force:true})}
});
