import {test} from 'node:test';import assert from 'node:assert/strict';import {syncProducts} from './product-sync.mjs';
test('product pagination selects 200, visits published statuses and accepts empty sources',async()=>{
 const product=i=>({platform:'tiktok',shopId:'1',platformItemId:String(i),sourceList:[]});
 const data=[{status:'onsale',page:1,size:20,total:201,items:[product(1)]},{status:'onsale',page:1,size:200,total:201,items:Array.from({length:200},(_,i)=>product(i+1))},{status:'onsale',page:2,size:200,total:201,items:[product(201)]},...['forsale','suspendSales','deleted'].map(status=>({status,page:1,size:200,total:0,items:[]}))];
 let index=0;const pauses=[];const actions=[];const control={waitFor:async()=>{},click:async()=>{}};
 const page={setViewportSize:async()=>{},goto:async()=>{},close:async()=>{},locator:selector=>({...control,click:async()=>actions.push(selector),filter:()=>control}),getByRole:()=>({...control,filter:()=>({...control,click:async()=>actions.push('200')})}),waitForResponse:async()=>{const d=data[index++];return {request:()=>({postData:()=>`status=${d.status}&pageNo=${d.page}&pageSize=${d.size}&shopId[0]=1`}),finished:async()=>{},json:async()=>({result:'success',total:d.total,itemList:d.items}),status:()=>200}}};
 const s={state:'verified',pending:new Set(),context:{newPage:async()=>page},sync:{scope:'products',state:'running'}};
 await syncProducts(s,async ms=>pauses.push(ms));assert.equal(s.sync.state,'completed');assert.equal(s.sync.collected,201);assert.equal(index,6);assert.ok(actions.includes('200'));assert.ok(actions.includes('.el-pagination .btn-next'));assert.ok(pauses.every(ms=>ms>=3000&&ms<5000));
});
