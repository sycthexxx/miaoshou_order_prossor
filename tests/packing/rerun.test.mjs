import {reactive} from 'vue';
import test from 'node:test';import assert from 'node:assert/strict';
import {rerunGroups,rerunRequests} from '../../web/src/lib/packing-rerun.js';
test('rerun groups shared info, preserves identifiers and keeps original unchanged',()=>{
 const info={purchase:{platform:'1688',orderNumber:'old',buyer:'b',seller:'s',address:'keep'},autoSyncAmount:true,amount:'',note:'keep note'};
 const job={items:[{request:{packageId:'p1',items:[{...info,orderItemId:'a'},{...info,orderItemId:'g'}]},lines:[{itemId:'a'},{itemId:'g',gift:true}]},{request:{packageId:'p2',items:[{...info,orderItemId:'b'}]},lines:[{itemId:'b'}]}]};
 const before=JSON.stringify(job),groups=reactive(rerunGroups(reactive(job)));assert.equal(groups.length,1);assert.equal(groups[0].lines.length,3);
 groups[0].info.purchase.orderNumber='new';let requests=rerunRequests(groups);assert.deepEqual(requests.map(r=>r.packageId),['p1','p2']);assert.deepEqual(requests[0].items.map(i=>i.orderItemId),['a','g']);assert.equal(requests[1].items[0].purchase.orderNumber,'new');assert.equal(requests[0].items[0].purchase.address,'keep');assert.equal(JSON.stringify(job),before);
 groups[0].mode='waybill';groups[0].info.waybills=[{company:'c',number:'n'}];requests=rerunRequests(groups);assert.equal(requests[0].items[0].purchase,undefined);assert.equal(requests[0].items[0].autoSyncAmount,false);assert.equal(requests[0].items[0].note,'keep note');
});
