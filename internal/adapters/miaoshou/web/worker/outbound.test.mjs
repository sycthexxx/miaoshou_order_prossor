import {test} from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {observeOutbound} from './outbound.mjs';
const response=ids=>({url:()=> 'https://erp.91miaoshou.com/api/order/package/render_list/getOpOrderPackageIdAndOutboundInfoMap',finished:async()=>{},status:()=>200,json:async()=>({result:'success',opOrderPackageIdAndOutboundInfoMap:Object.fromEntries(ids.map(id=>[id,{}]))})});
test('waits for all package chunks, removes observer',async()=>{const page=new EventEmitter(),o=observeOutbound(page,1000);page.emit('response',response(['1']));const done=o.wait(['1','2']);page.emit('response',response(['2']));await done;o.stop();assert.equal(page.listenerCount('response'),0)});
test('missing outbound response reports partial collection',async()=>{const page=new EventEmitter(),o=observeOutbound(page,10);await assert.rejects(o.wait(['1']),/出货信息超时/);o.stop()});
