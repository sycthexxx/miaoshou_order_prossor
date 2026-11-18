import {test} from 'node:test';
import assert from 'node:assert/strict';
import '../../extensions/procurement-helper/supplier.js';
globalThis.getComputedStyle=()=>({visibility:'visible'});
const el=(name,attr={},visible=true)=>({isConnected:true,textContent:name,getClientRects:()=>visible?[{}]:[],getAttribute:key=>attr[key]||null});
const doc=map=>({querySelectorAll:selector=>map[selector]||[]});
test('only explicit company/shop labels count; duplicate same name is one candidate',()=>{
 const page=doc({'.company-name':[el('正格服饰厂')],'.company-title':[el(' 正格服饰厂 ')],'.shop-name':[el('进入店铺')]});
 assert.deepEqual(OPSupplier.supplierNames(page),['正格服饰厂']);
 assert.deepEqual(OPSupplier.supplierNames(doc({'.company-name':[el('甲服饰厂'),el('乙服饰厂')]})),['甲服饰厂','乙服饰厂']);
 assert.deepEqual(OPSupplier.supplierNames(doc({'.company-name':[el('隐藏厂',{},false)]})),[]);
 assert.deepEqual(OPSupplier.supplierNames(doc({})),[]);
});
test('explicit company data attribute is supported without parsing unrelated page titles',()=>{
 assert.deepEqual(OPSupplier.supplierNames(doc({'[data-company-name]':[el('详情',{'data-company-name':'甲服饰厂'})]})),['甲服饰厂']);
 assert.equal(OPSupplier.cleanName('https://evil.example'),'');assert.equal(OPSupplier.cleanName('进入店铺'),'');
});

test('a lone generic or recommendation company cannot auto-submit',()=>{
 const generic=el('推荐服饰厂');
 assert.equal(OPSupplier.supplierReading(doc({'.company-name':[generic]}),'100').automaticName,'');
 const recommended=el('推荐服饰厂');recommended.closest=selector=>selector.startsWith('footer')?{}:null;
 assert.equal(OPSupplier.supplierReading(doc({'.company-name':[recommended]}),'100').automaticName,'');
});
test('automatic candidate requires current seller scope and a shop link; other offer is excluded',()=>{
 const company=el('正格服饰厂'),scope={querySelectorAll:()=>[{getAttribute:()=> 'https://shop123.1688.com/page/offerlist.htm'}]};
 let owner=null;
 company.closest=selector=>selector.startsWith('[data-current-supplier]')?scope:selector==='[data-offer-id]'?owner:null;
 assert.equal(OPSupplier.supplierReading(doc({'.company-name':[company]}),'100').automaticName,'正格服饰厂');
 owner={getAttribute:()=> '200'};assert.equal(OPSupplier.supplierReading(doc({'.company-name':[company]}),'100').automaticName,'');
 owner=null;scope.querySelectorAll=()=>[{getAttribute:()=> 'https://detail.1688.com/offer/100.html'}];assert.equal(OPSupplier.supplierReading(doc({'.company-name':[company]}),'100').automaticName,'');
});

test('top-left company header with its shop link is preferred, not generic coordinates',()=>{
 const name=el('左上角服饰厂'),header={querySelectorAll:()=>[{getAttribute:()=> 'https://shop456.1688.com/'}]};
 name.getBoundingClientRect=()=>({left:40,top:70});
 name.closest=selector=>selector.startsWith('header,')?header:null;
 const other=el('其他候选服饰厂');
 assert.equal(OPSupplier.supplierReading(doc({'.company-name':[name,other]}),'100').automaticName,'左上角服饰厂');
 header.querySelectorAll=()=>[];name.querySelectorAll=()=>[];
 assert.equal(OPSupplier.supplierReading(doc({'.company-name':[name]}),'100').automaticName,'');
 header.querySelectorAll=()=>[{getAttribute:()=> 'https://shop456.1688.com/'}];
 name.getBoundingClientRect=()=>({left:900,top:700});
 assert.equal(OPSupplier.supplierReading(doc({'.company-name':[name]}),'100').automaticName,'');
});

test('supplier status distinguishes same name, changed name, and first assignment',()=>{
 const result=(extra)=>({assigned:0,updated:0,retained:0,created:false,supplierName:'乙服饰厂',...extra});
 assert.equal(OPSupplier.supplierResultText(result({retained:1})),'厂家名称相同（乙服饰厂），保持不变。');
 assert.match(OPSupplier.supplierResultText(result({updated:1})),/已更新 1 条货源厂家关联/);
 assert.match(OPSupplier.supplierResultText(result({updated:1,created:true})),/已新增厂家「乙服饰厂」/);
 assert.match(OPSupplier.supplierResultText(result({assigned:1})),/新增 1 条货源关联/);
 const mixed=OPSupplier.supplierResultText(result({assigned:1,updated:2,retained:3}));
 assert.match(mixed,/更新 2 条/);assert.match(mixed,/3 条同名关联保持不变/);
 assert.throws(()=>OPSupplier.supplierResultText({assigned:0,retained:1,created:false}),/更新并重启/);
});
