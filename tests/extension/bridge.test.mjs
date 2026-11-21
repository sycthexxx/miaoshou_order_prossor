import {test} from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const script=await readFile(new URL('../../extensions/procurement-helper/bridge.js',import.meta.url),'utf8');
function fixture(status=200,data={assigned:1,updated:0,retained:0,created:true,supplierName:'甲厂'}){let receiver;const requests=[];const context={
 document:{documentElement:{dataset:{}},addEventListener:()=>{}},location:{origin:'http://192.168.1.104:17381'},window:{dispatchEvent:()=>{},addEventListener:()=>{}},CustomEvent:class{},AbortSignal:{timeout:()=>null},alert:()=>{},
 chrome:{runtime:{id:'own-extension',onMessage:{addListener:fn=>{receiver=fn}},sendMessage:async()=>({ok:true})}},
 fetch:async(url,options)=>{requests.push({url,options});return {status,ok:status===200,json:async()=>data}}
 };vm.runInNewContext(script,context);return {requests,call:(message,sender={id:'own-extension'})=>new Promise(resolve=>receiver(message,sender,resolve))};}
const message={type:'SAVE_CAPTURED_SUPPLIER',origin:'http://192.168.1.104:17381',account:'7',productId:'100',name:'甲厂'};
test('bridge uses its own origin with native same-origin fetch, never employee localhost',async()=>{
 const f=fixture();assert.equal((await f.call(message)).ok,true);const r=f.requests[0];assert.equal(r.url,'/api/accounts/7/source-supplier/capture');assert.equal(r.options.credentials,'same-origin');assert.deepEqual(JSON.parse(r.options.body),{productId:'100',name:'甲厂'});
 assert.equal((await f.call({...message,origin:'http://localhost:17381'})).ok,false);assert.equal(f.requests.length,1);
});
test('old backend 404 has an actionable update message',async()=>{const r=await fixture(404).call(message);assert.equal(r.ok,false);assert.match(r.error,/更新并重启/)});

test('mapping bridge GET/PUT stays same-origin and carries optimistic revision',async()=>{
 const config={version:2,colors:{'浅咖啡':'浅咖'},skus:{}};
 const f=fixture(200,{revision:2,mapping:config}),base={...message,name:undefined};
 assert.equal((await f.call({...base,type:'GET_PURCHASE_MAPPING'})).result.revision,2);
 assert.equal(f.requests[0].url,'/api/accounts/7/procurement-mappings/100');assert.equal(f.requests[0].options.method,'GET');assert.equal(f.requests[0].options.body,undefined);
 await f.call({...base,type:'PUT_PURCHASE_MAPPING',revision:2,mapping:config});
 assert.equal(f.requests[1].options.method,'PUT');assert.deepEqual(JSON.parse(f.requests[1].options.body),{revision:2,mapping:config});
 const conflict=await fixture(409,{error:'版本冲突',revision:3,mapping:config}).call({...base,type:'PUT_PURCHASE_MAPPING',revision:2,mapping:config});
 assert.equal(conflict.ok,false);assert.equal(conflict.status,409);assert.equal(conflict.result.revision,3);
});

test('offer query reads chosen account cache without refresh and preserves cache timestamp',async()=>{
 const f=fixture(200,{found:false,createdAt:123});const r=await f.call({...message,type:'GET_PURCHASE_OFFER',gifts:true});
 assert.equal(r.result.createdAt,123);assert.equal(f.requests[0].url,'/api/accounts/7/procurement/offers/100?gifts=1');assert.equal(f.requests[0].options.method,'GET');assert.equal(f.requests[0].options.body,undefined);
});

test('supplier capture succeeds without acquiring, renewing or releasing a purchase lease',async()=>{
 const f=fixture(200,{assigned:0,updated:1,retained:0,created:false,supplierName:'甲厂'});
 const r=await f.call(message);
 assert.equal(r.ok,true);assert.equal(r.result.updated,1);
 assert.equal(f.requests.length,1);
 const request=f.requests[0];
 assert.equal(request.url,'/api/accounts/7/source-supplier/capture');
 assert.equal(request.options.method,'POST');
 assert.equal(request.options.headers['X-Supplier-Lease'],undefined);
 assert.deepEqual(JSON.parse(request.options.body),{productId:'100',name:'甲厂'});
});

test('source status uses bound account and forces plugin source rather than manual override',async()=>{
 const f=fixture(200,{sourceStatus:{state:'invalid',source:'manual'},retainedManual:true,changed:false});
 const r=await f.call({...message,type:'SAVE_SOURCE_STATUS',state:'invalid',reason:'商品已下架',evidence:'offer_removed',source:'manual'});
 assert.equal(r.result.retainedManual,true);assert.equal(f.requests[0].url,'/api/accounts/7/source-status/100');assert.equal(f.requests[0].options.method,'PUT');
 assert.deepEqual(JSON.parse(f.requests[0].options.body),{state:'invalid',reason:'商品已下架',source:'plugin',evidence:'offer_removed'});
});
