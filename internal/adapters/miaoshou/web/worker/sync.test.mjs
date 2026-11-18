import {test} from 'node:test';
import assert from 'node:assert/strict';
import {syncAll} from './sync.mjs';
const ids=(start,n)=>Array.from({length:n},(_,i)=>String(start+i));
const data=(page,list,total,pageSize=200,purchaseTab='')=>({result:'success',page,pageSize,total,purchaseTab,packageList:list.map(opOrderPackageId=>({opOrderPackageId}))});
function fixture(responses,scope='all'){
 let index=0,closed=false;const pauses=[],actions=[],published=[],responseArchives=new WeakMap();
 const control={waitFor:async()=>{},click:async()=>{}};
 let outboundListener;
 const page={on:(_event,fn)=>{outboundListener=fn},off:()=>{outboundListener=null},setViewportSize:async()=>{},locator:selector=>({filter:()=>control,getByText:text=>{assert.equal(selector,'#appScrollContainer');return {click:async()=>actions.push(text)}}}),goto:async()=>{},close:async()=>{closed=true},getByText:()=>{throw new Error('unscoped text matches both pagination and dropdown')},getByRole:(role,options)=>role==='listbox'?{filter:filter=>{assert.equal(filter.visible,true);return {getByRole:(child,option)=>{assert.equal(child,'option');return {click:async()=>actions.push(option.name)}}}}}:({...control,click:async()=>{if(role==='listitem')throw new Error('message-box intercepts pointer events');actions.push(options.name)},press:async key=>{assert.equal(key,'Enter');actions.push(options.name)}}),waitForResponse:async()=>{
  const d=responses[index++]||(scope==='all'?data(1,[],0,200,index===responses.length+1?'waitPurchase':'hasPurchase'):null);if(!d)throw new Error('unexpected request');
  const response={request:()=>({postData:()=>`appPackageTab=${d.purchaseTab?'waitProcess':'all'}&source=${d.purchaseTab?'orderProcess':'orderHistory'}&page=${d.page}&purchaseTab=${d.purchaseTab}`}),finished:async()=>{},json:async()=>{await outboundListener?.({url:()=>"https://erp.91miaoshou.com/api/order/package/render_list/getOpOrderPackageIdAndOutboundInfoMap",finished:async()=>{},status:()=>200,json:async()=>({result:'success',opOrderPackageIdAndOutboundInfoMap:Object.fromEntries(d.packageList.map(p=>[p.opOrderPackageId,{}]))})});return d},status:()=>200};responseArchives.set(response,{filename:`page-${index}.json`});return response;
 }};
 const s={state:'verified',responseArchives,publishScope:async d=>published.push(d),pending:new Set(),context:{newPage:async()=>page},sync:{state:'running',scope}};
 return {s,actions,pauses,published,pause:async ms=>pauses.push(ms),requests:()=>index,closed:()=>closed};
}
test('stable pagination uses keyboard, spacing and a head refresh',async()=>{
 const head=data(1,ids(1,200),201),f=fixture([head,data(2,['201'],201),head]);
 await syncAll(f.s,f.pause);assert.equal(f.s.sync.state,'completed');assert.equal(f.s.sync.collected,201);assert.equal(f.requests(),5);assert.deepEqual(f.actions,['重置','第 2 页','第 1 页','重置']);assert.ok(f.closed());assert.ok(f.pauses.every(ms=>ms>=3000&&ms<5000));
});
test('growing total is accepted and new head orders get a bounded reconciliation pass',async()=>{
 const first=data(1,ids(1,200),201),head=data(1,['202',...ids(1,199)],202);
 const f=fixture([first,data(2,['200','201'],202),head,data(2,['200','201'],202)]);
 await syncAll(f.s,f.pause);assert.equal(f.s.sync.state,'completed');assert.equal(f.s.sync.collected,202);assert.equal(f.s.sync.scope,'all');assert.match(f.s.sync.notice,/变化/);assert.equal(f.requests(),6);
});
test('shrinking totals continue and reconcile instead of failing',async()=>{
 const head=data(1,ids(2,200),201),f=fixture([data(1,ids(1,200),202),data(2,['202'],201),head,data(2,['202'],201)]);
 await syncAll(f.s,f.pause);assert.equal(f.s.sync.state,'completed');assert.equal(f.requests(),6);
});
test('selects 200 through page-size menu and checks actual response size',async()=>{
 const f=fixture([data(1,ids(1,20),30,20),data(1,ids(1,30),30)]);await syncAll(f.s,f.pause);
 assert.equal(f.s.sync.state,'completed');assert.equal(f.s.sync.pageSize,200);assert.ok(f.actions.includes('200条/页'));
 const bad=fixture([data(1,[],0,20),data(1,[],0,20)]);await syncAll(bad.s,bad.pause);assert.equal(bad.s.sync.state,'failed');
});
test('pending includes both groups and deduplicates packages',async()=>{
 const f=fixture([data(1,['1','2'],2,200,'waitPurchase'),data(1,['2','3'],2,200,'hasPurchase')],'pending');await syncAll(f.s,f.pause);assert.equal(f.s.sync.state,'completed');assert.equal(f.s.sync.collected,3);
});
test('wrong response page remains an error',async()=>{
 const f=fixture([data(1,ids(1,200),201),data(1,['201'],201)]);await syncAll(f.s,f.pause);assert.equal(f.s.sync.state,'failed');assert.match(f.s.sync.error,/分页不一致/);
});
test('cancellation keeps already collected progress',async()=>{
 const f=fixture([data(1,ids(1,200),201)]);let calls=0;await syncAll(f.s,async()=>{if(++calls===2)f.s.sync.cancel=true});assert.equal(f.s.sync.state,'cancelled');assert.equal(f.s.sync.collected,200);
});

test('complete multipage scope publishes exact archived page evidence',async()=>{
 const head=data(1,ids(1,200),201,200,'waitPurchase');
 const f=fixture([head,data(2,['201'],201,200,'waitPurchase'),head,data(1,['300'],1,200,'hasPurchase')],'pending');
 await syncAll(f.s,f.pause);assert.equal(f.s.sync.state,'completed');assert.equal(f.published.length,2);
 assert.deepEqual(f.published[0].archives,['page-3.json','page-2.json']);assert.equal(f.published[0].packageIds.length,201);
 assert.equal(f.published[1].scope,'hasPurchase');
});
test('failure after first completed group publishes no scope',async()=>{
 const f=fixture([data(1,['1'],1,200,'waitPurchase')],'pending');await syncAll(f.s,f.pause);
 assert.equal(f.s.sync.state,'failed');assert.equal(f.published.length,0);
});

test('all history additionally reconciles pending groups without double-counting',async()=>{
 const f=fixture([data(1,['1','2','3'],3),data(1,['1'],1,200,'waitPurchase'),data(1,['2'],1,200,'hasPurchase')]);
 await syncAll(f.s,f.pause);assert.equal(f.s.sync.state,'completed');assert.equal(f.s.sync.scope,'all');assert.equal(f.s.sync.collected,3);
 assert.deepEqual(f.published.map(p=>p.scope),['all','waitPurchase','hasPurchase']);assert.equal(f.requests(),3);
});

test('all sync cancellation during pending calibration publishes none and stays running until done',async()=>{
 const f=fixture([data(1,['1'],1)]);let calls=0;
 await syncAll(f.s,async()=>{assert.equal(f.s.sync.state,'running');if(++calls===3)f.s.sync.cancel=true});
 assert.equal(f.s.sync.state,'cancelled');assert.equal(f.published.length,0);
});
