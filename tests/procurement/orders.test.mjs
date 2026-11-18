import test from 'node:test';
import assert from 'node:assert/strict';
import {procurementOrderRows} from '../../web/src/lib/procurement-orders.js';
const line=(order,item,extra={})=>({orderNo:order,shopId:'shop',packageId:order,packageNo:'MS'+order,itemId:item,quantity:1,...extra});
test('same order across sizes appears once, quantities and source lines stay intact',()=>{
 const group={variants:[{spec:'白色,L',lines:[line('587229698073855171','1',{localNote:'采购'}),line('other','3')]},{spec:'白色,XL',lines:[line('587229698073855171','2',{localNote:'采购'})]}]};
 const before=JSON.stringify(group),rows=procurementOrderRows(group);
 const row=rows.find(r=>r.specs.length===2);
 assert.equal(row.quantity,2);assert.equal(row.orders.length,1);
 assert.equal(row.orders[0].orderNo,'587229698073855171');
 assert.equal(row.orders[0].packages[0].lines.length,2);
 assert.deepEqual(row.orders[0].packages[0].notes,['采购']);
 assert.equal(rows.reduce((n,r)=>n+r.quantity,0),3);
 assert.equal(JSON.stringify(group),before);
});
test('split packages retain separate price targets; different shops and missing order numbers remain separate',()=>{
 const rows=procurementOrderRows({variants:[{spec:'L',lines:[line('same','1'),line('same','2',{packageId:'split'}),line('same','3',{shopId:'other'}),line('','4',{packageId:'a'}),line('','5',{packageId:'b'})]}]});
 assert.equal(rows.length,1);assert.equal(rows[0].orders.length,4);
 assert.deepEqual(rows[0].orders[0].packages.map(p=>p.packageId),['same','split']);
 assert.deepEqual(rows[0].orders[0].packages[0].notes,[]);
});
