import test from 'node:test';
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
import {getAmountDetail} from './amount-detail.mjs';

test('amount read is live, uses exact package/order IDs, and returns only amount metadata',async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const context=await browser.newContext(),page=await context.newPage();let requests=[];
  await context.route('**/*',route=>{
   const r=route.request(),u=new URL(r.url());requests.push(u.pathname);
   if(u.pathname==='/index-fixture.js')return route.fulfill({contentType:'text/javascript',body:`export async function request(c){const r=await fetch('/api'+c.url+(c.method==='GET'?'?'+new URLSearchParams(c.params):''),{method:c.method,body:c.method==='POST'?new URLSearchParams(c.data):undefined});return r.json()}`});
   let body;
   if(u.pathname.endsWith('/getOrderPackageAmountDetail')){assert.equal(u.searchParams.get('opOrderPackageId'),'2178796421');body={orderPackageAmountDetail:{platformOrderSn:'587229498025411885',orderProfit:'36.14'}}}
   else if(u.pathname.endsWith('/getOrderDetailInfo')){assert.equal(new URLSearchParams(r.postData()).get('opOrderId'),'2209871569');body={orderDetailInfo:{opOrderId:'2209871569',platformOrderSn:'587229498025411885',currency:'MXN',buyerMobile:'private-not-returned'}}}
   else if(u.pathname.endsWith('/getExchangeRateList'))body={exchangeRateList:[{currency:'MXN',exchangeRate:'2.76'}]};
   else if(u.pathname.endsWith('/getEstimatedShippingFeeInfoMapByOpOrderPackageIds'))body={opOrderPackageIdAndEstimatedShippingFeeInfoMap:{}};
   else if(u.pathname==='/order/package/index')return route.fulfill({contentType:'text/html',body:'fixture'});
   else return route.abort();
   return route.fulfill({contentType:'application/json',body:JSON.stringify({result:'success',...body})});
  });
  await page.goto('https://erp.91miaoshou.com/order/package/index');
  const options={session:{context},payload:{packageId:'2178796421',orderId:'2209871569'},resolveClient:async()=>({url:'https://erp.91miaoshou.com/index-fixture.js',name:'request'})};
  for(let n=0;n<2;n++){
   const result=await getAmountDetail(options),data=JSON.parse(result.body);
   assert.equal(data.order.orderNo,'587229498025411885');assert.equal(data.amount.orderProfit,'36.14');assert.equal(data.order.buyerMobile,undefined);assert.equal(result.archive,undefined);
  }
  assert.equal(requests.filter(p=>p.endsWith('/getOrderPackageAmountDetail')).length,2);
 }finally{await browser.close()}
});
