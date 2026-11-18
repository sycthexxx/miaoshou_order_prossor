import test from 'node:test';
import assert from 'node:assert/strict';
import {sourceLines,filteredLines,selectionStats,groupLinesBySpec} from '../../web/src/lib/packing-selection.js';
test('SKU selection preserves exact item IDs, hidden choices, gifts and multi-item packages',()=>{
 const lines=sourceLines({variants:[{spec:'深咖啡,L',lines:[{packageId:'1',itemId:'11',packageNo:'MS1',skuId:'100',quantity:2},{packageId:'2',itemId:'21',quantity:1}]},{spec:'深咖啡，M',lines:[{packageId:'1',itemId:'12',quantity:1}]},{spec:'默认情况下',lines:[{packageId:'1',itemId:'13',gift:true,quantity:1}]}]});
 const l=filteredLines(lines,{colors:['深咖啡'],sizes:['L']});assert.equal(l.length,2);const selected=new Set(l.map(l=>l.key));
 assert.equal(filteredLines(lines,{sizes:['M']}).length,1);assert.deepEqual(selectionStats(lines,selected),{packages:2,lines:2,quantity:3});
 assert.equal(filteredLines(lines,{query:'MS1'})[0].itemId,'11');assert.equal(lines.filter(l=>l.gift&&selected.has(l.key)).length,0);
 selected.add(lines.find(l=>l.itemId==='12').key);assert.deepEqual(selectionStats(lines,selected),{packages:2,lines:3,quantity:4});
});

test('one packing selection spans multiple colors and sizes, including hidden selections',()=>{
 const lines=sourceLines({variants:[{spec:'黑色,L',lines:[{packageId:'1',itemId:'11',quantity:1}]},{spec:'红色,M',lines:[{packageId:'1',itemId:'12',quantity:2}]},{spec:'红色,L',lines:[{packageId:'2',itemId:'21',quantity:1}]},{spec:'蓝色,S',lines:[{packageId:'3',itemId:'31',quantity:1}]}]});
 const shown=filteredLines(lines,{colors:['黑色','红色'],sizes:['L','M']});assert.equal(shown.length,3);
 const selected=new Set(shown.map(l=>l.key));
 const narrowed=filteredLines(lines,{colors:['蓝色'],sizes:['S']});assert.equal(narrowed.length,1);
 const applied=groupLinesBySpec(lines.filter(l=>selected.has(l.key)));
 assert.deepEqual(applied.map(v=>v.spec),['黑色,L','红色,M','红色,L']);
 assert.deepEqual(selectionStats(lines,selected),{packages:2,lines:3,quantity:4});
});

test('facet badges count distinct orders and units, respect other filters and omit empty options',async()=>{
 const {filterOptions}=await import('../../web/src/lib/packing-selection.js');
 const lines=sourceLines({variants:[{spec:'红色,L',lines:[{packageId:'1',itemId:'11',quantity:2}]},{spec:'红色,M',lines:[{packageId:'1',itemId:'12',quantity:1},{packageId:'2',itemId:'21',quantity:3}]},{spec:'蓝色,XL',lines:[{packageId:'3',itemId:'31',quantity:4}]}]});
 const red=filterOptions(lines,{},'color').find(o=>o.value==='红色');
 assert.deepEqual(red,{value:'红色',packages:2,quantity:6});
 assert.deepEqual(filterOptions(lines,{colors:['红色'],sizes:['L']},'size'),[{value:'L',packages:1,quantity:2},{value:'M',packages:2,quantity:4}]);
 assert.deepEqual(filterOptions(lines,{sizes:['XL']},'color'),[{value:'蓝色',packages:1,quantity:4}]);
 assert.deepEqual(filterOptions(lines,{query:'不存在'},'size'),[]);
});

test('supplier selection spans products without merging identical color/size choices',async()=>{
 const {supplierPackingGroup}=await import('../../web/src/lib/packing-selection.js');
 const product=(id,supplier,item)=>({key:id,supplier:{id:supplier,name:'factory'},title:'product '+id,source:{sourceItemId:id},variants:[{spec:'黑色,L',lines:[{packageId:'1',itemId:item,quantity:1}]}]});
 const groups=[product('100',3,'11'),product('200',3,'12'),product('300',4,'13')];
 const scope=supplierPackingGroup({id:3,name:'factory'},groups),lines=sourceLines(scope);
 assert.equal(scope.key,'supplier:3');assert.equal(scope.sources.length,2);
 assert.deepEqual(groupLinesBySpec(lines).map(g=>g.sourceItemId),['100','200']);
 assert.equal(filteredLines(lines,{sources:['200']})[0].itemId,'12');
 assert.equal(filteredLines(lines,{query:'product 100'})[0].itemId,'11');
 const selected=new Set(lines.map(l=>l.key));assert.deepEqual(selectionStats(lines,selected),{packages:1,lines:2,quantity:2});
 assert.equal(sourceLines(groups[0]).length,1);
});
