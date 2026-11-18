import {test} from 'node:test';
import assert from 'node:assert/strict';
let receive,removed,nextID=10,configured='',relayResult={ok:true,result:{assigned:1,updated:0,retained:0,created:true,supplierName:'甲厂'}};
let settings={account:"7",gifts:true};
const local={},session={},tabs=new Map(),relays=[];
globalThis.chrome={
 runtime:{getManifest:()=>({version:'0.1.8'}),onMessage:{addListener:fn=>{receive=fn}},onInstalled:{addListener:()=>{}},onStartup:{addListener:()=>{}}},
 storage:{local:{get:async()=>({...local,workbenchOrigin:configured}),set:async data=>Object.assign(local,data)},session:{get:async key=>({[key]:session[key]}),set:async data=>Object.assign(session,data),remove:async key=>{delete session[key]}}},
 scripting:{executeScript:async()=>{}},
 tabs:{query:async()=>Array.from(tabs.values()),create:async data=>{const tab={id:++nextID,status:'complete',...data};tabs.set(tab.id,tab);return tab},get:async id=>{if(!tabs.has(id))throw Error('closed');return tabs.get(id)},update:async(id,data)=>Object.assign(tabs.get(id),data),remove:async id=>tabs.delete(id),onRemoved:{addListener:fn=>{removed=fn}},sendMessage:async(id,data,options)=>{relays.push({id,data,options});return data.type==="GET_PLUGIN_QUERY_SETTINGS"?{ok:true,result:settings}:relayResult}}
};
await import('../../extensions/procurement-helper/background.js');
const call=(message,sender)=>new Promise(resolve=>receive(message,sender,resolve));
const payload={version:1,url:'https://detail.1688.com/offer/100.html',offerId:'100',account:'1',title:'产品',createdAt:new Date().toISOString(),variants:[{spec:'黑色,M',quantity:2}]};
const opening=(url,account='1')=>{const tab={id:++nextID,url};tabs.set(tab.id,tab);return {tab,frameId:0,url}};
const offerSender=id=>({tab:{id},frameId:0,url:payload.url});
test('separate tab snapshots, exact offer check, trusted workbench only',async()=>{
 const sender=opening('http://127.0.0.1:17381/#/procurement');
 assert.equal((await call({type:'OPEN_PROCUREMENT',payload},sender)).ok,true);const tabA=nextID;
 assert.equal((await call({type:'OPEN_PROCUREMENT',payload:{...payload,account:'2',variants:[{spec:'黑色,M',quantity:9}]}},sender)).ok,true);
 const get=id=>call({type:'GET_PROCUREMENT'},offerSender(id));
 assert.equal((await get(tabA)).task.variants[0].quantity,2);assert.equal((await get(nextID)).task.variants[0].quantity,9);
 assert.equal((await get(tabA)).task.workbench,undefined);
 assert.equal((await call({type:'GET_PROCUREMENT'},{...offerSender(tabA),url:'https://detail.1688.com/offer/200.html'})).task,null);
 assert.equal((await call({type:'OPEN_PROCUREMENT',payload},{...sender,url:'https://evil.example'})).ok,false);
 assert.equal((await call({type:'OPEN_PROCUREMENT',payload},{...sender,frameId:1})).ok,false);
 removed(tabA);assert.equal((await get(tabA)).task,null);
});
test('LAN workstation connects explicitly and gets the same source snapshot without localhost fetch',async()=>{
 const sender=opening('http://192.168.1.104:17381/#/procurement');configured='';
 assert.equal((await call({type:'BRIDGE_STATUS'},sender)).ok,false);
 assert.equal((await call({type:'OPEN_PROCUREMENT',payload},sender)).ok,false);
 configured='http://192.168.1.104:17381';
 assert.equal((await call({type:'BRIDGE_STATUS'},sender)).version,'0.1.8');
 assert.equal((await call({type:'OPEN_PROCUREMENT',payload:{...payload,account:'9'}},sender)).ok,true);const offer=nextID;
 assert.equal((await call({type:'GET_PROCUREMENT'},offerSender(offer))).task.account,'9');
 const reply=await call({type:'REPORT_SUPPLIER',name:'甲厂',account:'999',productId:'999',origin:'http://localhost:17381'},offerSender(offer));
 assert.equal(reply.ok,true);const sent=relays.at(-1);
 assert.equal(sent.id,sender.tab.id);assert.equal(sent.data.origin,'http://192.168.1.104:17381');assert.equal(sent.data.account,'9');assert.equal(sent.data.productId,'100');assert.deepEqual(sent.options,{frameId:0});
 const count=relays.length;
 tabs.get(sender.tab.id).url='https://evil.example';assert.equal((await call({type:'REPORT_SUPPLIER',name:'甲厂'},offerSender(offer))).ok,false);assert.equal(relays.length,count);
 tabs.delete(sender.tab.id);assert.match((await call({type:'REPORT_SUPPLIER',name:'甲厂'},offerSender(offer))).error,/关闭/);assert.equal(relays.length,count);
});
test('supplier report never crosses offer tabs and exposes backend failure',async()=>{
 configured='http://192.168.1.104:17381';const sender=opening(configured+'/#/procurement');
 await call({type:'OPEN_PROCUREMENT',payload},sender);const offer=nextID,count=relays.length;
 assert.equal((await call({type:'REPORT_SUPPLIER',name:'甲厂'},{...offerSender(offer),url:'https://detail.1688.com/offer/200.html'})).ok,false);assert.equal(relays.length,count);
 relayResult={ok:false,error:'货源不属于当前账户'};assert.equal((await call({type:'REPORT_SUPPLIER',name:'甲厂'},offerSender(offer))).error,relayResult.error);
});

test('mapping load and save use original server/account/offer, preserve revision conflict',async()=>{
 configured='http://192.168.1.104:17381';const sender=opening(configured+'/#/procurement');
 await call({type:'OPEN_PROCUREMENT',payload:{...payload,account:'7'}},sender);const offer=nextID;
 relayResult={ok:true,result:{revision:3,mapping:{version:2,colors:{'浅咖啡':'浅咖'},skus:{}}}};
 const loaded=await call({type:'GET_PROCUREMENT_MAPPING',account:'999',origin:'http://localhost:17381'},offerSender(offer));
 assert.equal(loaded.result.revision,3);assert.deepEqual(relays.at(-1).data,{type:'GET_PURCHASE_MAPPING',origin:configured,account:'7',productId:'100'});
 relayResult={ok:false,status:409,error:'其他电脑已修改',result:{revision:4,mapping:{version:2,colors:{},skus:{}}}};
 const config={version:2,colors:{'浅咖啡':'浅咖'},skus:{}};
 const conflict=await call({type:'PUT_PROCUREMENT_MAPPING',revision:3,mapping:config,account:'999',productId:'200'},offerSender(offer));
 assert.equal(conflict.status,409);assert.equal(conflict.result.revision,4);assert.equal(relays.at(-1).data.account,'7');assert.equal(relays.at(-1).data.productId,'100');assert.deepEqual(relays.at(-1).data.mapping,config);
 const count=relays.length;assert.equal((await call({type:'GET_PROCUREMENT_MAPPING'},{...offerSender(offer),url:'https://detail.1688.com/offer/200.html'})).ok,false);assert.equal(relays.length,count);
});

test('direct lookup reads current server settings and automatically reopens a closed workbench',async()=>{
 configured='http://192.168.1.104:17381';
 tabs.clear();
 const offer=opening(payload.url);
 relayResult={ok:true,result:{found:true,snapshot:{...payload,account:'7'}}};
 const result=await call({type:'GET_PROCUREMENT'},offer);
 assert.equal(result.task.account,'7');
 assert.equal(relays.at(-1).data.gifts,true);
 const tab=tabs.get(relays.at(-1).id);
 assert.equal(tab.url,configured+'/#/procurement');
 assert.equal(tab.active,false);
 settings={account:'8',gifts:false};
 relayResult={ok:true,result:{found:false,createdAt:Date.now()}};
 const miss=await call({type:'GET_PROCUREMENT'},offer);
 assert.equal(miss.task,null);
 assert.match(miss.reason,/#8/);
 assert.equal(relays.at(-1).id,tab.id);
 assert.equal(relays.at(-1).data.gifts,false);
});
