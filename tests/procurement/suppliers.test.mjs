import test from 'node:test';import assert from 'node:assert/strict';
import {procurementSupplierGroups,sourceStatusOfferId} from '../../web/src/lib/procurement-suppliers.js';
test('invalid sources form one pinned group without duplication or losing original supplier and quantities',()=>{
 const a={key:'a',quantity:2,supplier:{id:1,name:'甲'},sourceStatus:{state:'invalid',reason:'下架'}},b={key:'b',quantity:9,supplier:{id:1,name:'甲'}},c={key:'c',quantity:12,supplier:{id:2,name:'乙'}};
 const groups=procurementSupplierGroups([a,b,c]);assert.deepEqual(groups.map(g=>g.id),['invalid',2,1]);assert.equal(groups[0].products[0],a);assert.equal(a.supplier.name,'甲');assert.equal(groups.flatMap(g=>g.products).length,3);assert.equal(groups.reduce((n,g)=>n+g.quantity,0),23);
 a.sourceStatus.state='valid';assert.deepEqual(procurementSupplierGroups([a,b,c]).map(g=>g.id),[2,1]);
});
test('status uses purchase URL offer identity with 1688 ID fallback',()=>{
 assert.equal(sourceStatusOfferId({url:'https://detail.1688.com/offer/123.html?a=1',source:{source:'1688',sourceItemId:'456'}}),'123');
 assert.equal(sourceStatusOfferId({source:{source:'1688',sourceItemId:'456'}}),'456');assert.equal(sourceStatusOfferId({source:{source:'other',sourceItemId:'456'}}),'');
});
