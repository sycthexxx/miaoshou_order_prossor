import {test} from 'node:test';
import assert from 'node:assert/strict';
import {procurementHandoff} from '../../web/src/lib/procurement-handoff.js';
import '../../extensions/procurement-helper/core.js';
const {snapshot,offerId,comparison,quantity,tokens}=globalThis.OPProcurement;
const stamp='2026-11-16T08:00:00.000Z';
const group=(url,spec,quantity,supplier)=>({url,title:'商品',variants:[{spec,quantity,lines:[{orderNo:'PRIVATE-ORDER'}]}],supplier});
test('link identity merges across sellers and strips order/customer/seller data',()=>{
 const a=group('https://detail.1688.com/offer/100.html?spm=a','黑色,M',2,{id:1,name:'seller A'}),b=group('http://detail.1688.com/offer/100.html','黑色,M',3,{id:2,name:'seller B'}),c=group('https://detail.1688.com/offer/200.html','黑色,M',99,{id:1});
 const p=procurementHandoff([a,b,c],a,'7',stamp);
 assert.deepEqual(p.variants,[{spec:'黑色,M',quantity:5}]);assert.equal(p.offerId,'100');assert.ok(!JSON.stringify(p).includes('PRIVATE'));assert.ok(!JSON.stringify(p).includes('seller'));
 assert.deepEqual(snapshot(p),p);
});
test('exact quantity only; over-selection never green; unknown not treated as zero',()=>{
 assert.equal(comparison(5,5).kind,'match');assert.equal(comparison(5,6).kind,'over');assert.equal(comparison(5,4).kind,'short');assert.equal(comparison(5,null).kind,'unknown');
 assert.equal(quantity(''),null);assert.equal(quantity('1.5'),null);assert.equal(quantity('-1'),null);assert.equal(quantity('库存897套'),null);assert.equal(quantity('0'),0);
 assert.deepEqual(tokens('酒红色， M'),['酒红色','m']);assert.notDeepEqual(tokens('酒红色,M'),tokens('深红色,M'));
});
test('reject unrelated URLs and malformed snapshots',()=>{
 for(const u of ['javascript:alert(1)','https://detail.1688.com.evil.example/offer/100.html','https://detail.1688.com/offer/100.html/other'])assert.equal(offerId(u),'');
 const p={version:1,url:'https://detail.1688.com/offer/100.html',offerId:'101',createdAt:stamp,variants:[{spec:'M',quantity:1}]};assert.throws(()=>snapshot(p));p.offerId='100';p.variants[0].quantity=-1;assert.throws(()=>snapshot(p));
});

test('keeps checked colors, updates on return, and invalidates current failed reads',()=>{
 const {reconcile}=OPProcurement,ledger=new Map(),variants=[{spec:'酒红色,1XL',quantity:2},{spec:'玫瑰,5XL',quantity:1}],bindings={'玫瑰,5XL':{alias:'粉红色,5XL'}};
 const run=(red,pink,color)=>reconcile(variants,new Map([['酒红色,1XL',red],['玫瑰,5XL',pink]]),ledger,new Set([color]),bindings);
 assert.equal(run(2,null,'酒红色').get('酒红色,1XL').quantity,2);
 const switched=run(null,1,'粉红色');assert.deepEqual(switched.get('酒红色,1XL'),{quantity:2,cached:true});assert.equal(comparison(2,switched.get('酒红色,1XL').quantity).kind,'match');
 assert.deepEqual(run(0,null,'酒红色').get('酒红色,1XL'),{quantity:0,cached:false});
 run(2,null,'酒红色');assert.equal(run(null,null,'酒红色').get('酒红色,1XL').quantity,null);
 run(2,null,'酒红色');bindings['酒红色,1XL']={alias:'深红色,1XL'};assert.equal(run(null,1,'粉红色').get('酒红色,1XL').quantity,null);
 reconcile(variants,new Map(),ledger,new Set(),bindings,false);assert.equal(ledger.size,0);
});
test('images stay with their own SKU and survive the bridge snapshot',()=>{
 const a=group('https://detail.1688.com/offer/100.html','红色,S',1),b=group(a.url,'黑色,M',2);
 a.variants[0].images=['https://images.example/red.jpg'];b.variants[0].images=['https://images.example/black.jpg'];
 const p=snapshot(procurementHandoff([a,b],a,'1',stamp));
 assert.deepEqual(p.variants[0].images,a.variants[0].images);assert.deepEqual(p.variants[1].images,b.variants[0].images);
 const bad={...p,variants:[{spec:'red',quantity:1,images:['javascript:alert(1)','https://user:password@images.example/a.jpg']}]};assert.equal(snapshot(bad).variants[0].images,undefined);
});

test('quantity changes trigger follow without a button event, but color switches do not',()=>{
 const track=OPProcurement.quantityTracker();
 const values=(a,b)=>new Map([['酒红色,M',a],['粉红色,M',b]]);
 assert.deepEqual(track(values(0,null),'酒红色'),[]);
 assert.deepEqual(track(values(1,null),'酒红色'),['酒红色,M']);
 assert.deepEqual(track(values(1,null),'酒红色'),[]);
 assert.deepEqual(track(values(null,2),'粉红色'),[]);
 assert.deepEqual(track(values(null,1),'粉红色'),['粉红色,M']);
 assert.deepEqual(track(values(null,0),'粉红色'),['粉红色,M']);
 assert.deepEqual(track(values(null,null),'粉红色'),[]);
 assert.deepEqual(track(values(null,2),'粉红色'),[]);
});

test('click handoff and direct snapshots retain source status whitelist',()=>{
 const g=group('https://detail.1688.com/offer/100.html','M',1);g.sourceStatus={state:'invalid',reason:'removed manually',checkedAt:stamp,source:'manual',private:'secret'};
 const p=procurementHandoff([g],g,'7',stamp);assert.deepEqual(p.sourceStatus,{state:'invalid',reason:'removed manually',checkedAt:stamp,source:'manual'});
 assert.deepEqual(snapshot({...p,sourceStatus:g.sourceStatus}).sourceStatus,p.sourceStatus);
 assert.equal(snapshot({...p,sourceStatus:undefined}).sourceStatus.state,'unknown');
});
