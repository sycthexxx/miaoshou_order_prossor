import test from 'node:test';
import assert from 'node:assert/strict';
import {procurementSnapshotGate,procurementStream} from '../../web/src/lib/procurement-stream.js';
const summary=(revision,groups)=>({revision,createdAt:revision,groups});
test('whole snapshots replace groups and late HTTP cannot restore the former supplier',()=>{
 const gate=procurementSnapshotGate('1',false);let data;
 const apply=(d,a='1',g=false)=>{if(gate(d,a,g))data=d};
 apply(summary(10,[{key:'old',supplier:null}]));
 apply(summary(12,[{key:'new',supplier:{id:3}}]));
 apply(summary(11,[{key:'old',supplier:null}]));
 assert.deepEqual(data.groups,[{key:'new',supplier:{id:3}}]);
 apply(summary(12,[]));assert.equal(data.groups.length,1);
 apply(summary(13,[]));assert.deepEqual(data.groups,[]);
});
test('account and gifts scopes never replace one another and preserve independent revisions',()=>{
 const normal=procurementSnapshotGate('1',false),gifts=procurementSnapshotGate('1',true);
 assert.equal(normal(summary(20,[]),'1',true),false);
 assert.equal(normal(summary(20,[]),'2',false),false);
 assert.equal(normal(summary(20,[]),'1',false),true);
 assert.equal(gifts(summary(10,[]),'1',true),true);
 assert.equal(normal({groups:[]},'1',false),false);
});
function fixture(){
 const sockets=[],timers=new Map(),messages=[],statuses=[];let next=0;
 class Socket{constructor(url){this.url=url;sockets.push(this)}close(){this.closed=true;this.onclose?.()}}
 const stop=procurementStream({account:'7',gifts:true,origin:'https://intranet.example',Socket,onSnapshot:(...args)=>messages.push(args),onStatus:s=>statuses.push(s),schedule:fn=>{timers.set(++next,fn);return next},cancel:id=>timers.delete(id)});
 const send=(socket,account='7',gifts=true,revision=1)=>socket.onmessage?.({data:JSON.stringify({type:'procurement_snapshot',account,gifts,summary:summary(revision,[])})});
 return {sockets,timers,messages,statuses,stop,send};
}
test('same-origin websocket scopes query and reconnect receives the full current snapshot',()=>{
 const f=fixture(),first=f.sockets[0];assert.equal(first.url,'wss://intranet.example/api/accounts/7/procurement/ws?gifts=1');
 f.send(first,'8');f.send(first,'7',false);assert.equal(f.messages.length,0);
 f.send(first);first.close();assert.equal(f.timers.size,1);
 const callback=f.timers.values().next().value;f.timers.clear();callback();
 f.send(first);assert.equal(f.messages.length,1);
 f.send(f.sockets[1],'7',true,4);assert.equal(f.messages.at(-1)[0].revision,4);
 f.stop();assert.equal(f.sockets[1].closed,true);
});
test('unmount or scope change closes socket and cancels reconnection',()=>{
 const f=fixture();f.sockets[0].close();assert.equal(f.timers.size,1);f.stop();assert.equal(f.timers.size,0);
 assert.equal(f.sockets[0].onmessage,null);assert.equal(f.sockets[0].onclose,null);
});
