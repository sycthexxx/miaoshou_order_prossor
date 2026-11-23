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

test('saved supplier reports cache refresh warning as success text without requesting a supplier retry',()=>{
 const text=OPSupplier.supplierResultText({assigned:1,updated:0,retained:0,created:false,supplierName:'甲厂',cacheRefreshed:false,cacheRefreshError:'refresh failed'});
 assert.match(text,/厂家已保存，汇总刷新失败/);assert.match(text,/工作台刷新采购汇总/);assert.doesNotMatch(text,/回传厂家.*重试/);
});

test('capture only an explicit web chat link from the current seller and strip extra parameters',()=>{
 const company=el('正格服饰厂');
 const path='https://air.1688.com/app/ocms-fusion-components-1688/def_cbu_web_im/index.html';
 const href=path+'?touid=cnalichnabc&siteid=cnalichn&offerId=100#ignored';
 const anchor=href=>({getAttribute:()=>href});
 const shop=anchor('https://shop123.1688.com/');
 const scope={querySelectorAll:()=>[shop,anchor(href),anchor(href.replace('air.1688.com','air.1688.com.invalid'))]};
 company.closest=selector=>selector.startsWith('[data-current-supplier]')?scope:null;
 const page=doc({'.company-name':[company]});
 assert.equal(OPSupplier.supplierReading(page,'100').chatUrl,path+'?touid=cnalichnabc&siteid=cnalichn&status=1');
 scope.querySelectorAll=()=>[shop,anchor(href),anchor(href.replace('cnalichnabc','cnalichnother'))];
 assert.equal(OPSupplier.supplierReading(page,'100').chatUrl,'');
});

test('shop header provides the seller chat target through the customer service button',()=>{
 const company=el('东莞市虎门正格服饰厂');
 const button=el('客服',{'data-touid':'cnalichn正格服饰厂'});
 const header={tagName:'DIV',textContent:'东莞市虎门正格服饰厂关注客服商品',
  querySelectorAll:selector=>selector.includes('button')?[el('关注'),button,el('商品')]:[]};
 company.parentElement=header;
 const result=OPSupplier.supplierReading(doc({'.company-name':[company]}),'100');
 assert.equal(result.automaticName,'东莞市虎门正格服饰厂');
 assert.equal(new URL(result.chatUrl).searchParams.get('touid'),'cnalichn正格服饰厂');
});

test('the SHOP_CREDIT link identifies the company but is not a chat URL',()=>{
 const company=el('揭阳市揭东区新亨镇良晨服装厂');
 const shop={getAttribute:()=> 'https://shop23081902a52a8.1688.com?offerId=1039397478773'};
 const header={querySelectorAll:selector=>selector==='a[href]'?[shop]:[]};
 company.getBoundingClientRect=()=>({left:30,top:55});
 company.closest=selector=>selector.startsWith('header,')?header:null;
 const reading=OPSupplier.supplierReading(doc({'a[data-trace="SHOP_CREDIT"][href] h1':[company]}),'1039397478773');
 assert.equal(reading.automaticName,'揭阳市揭东区新亨镇良晨服装厂');
 assert.equal(reading.chatUrl,'');
});

test('a pasted address must identify a seller on the 1688 chat page',()=>{
 const page='https://air.1688.com/app/ocms-fusion-components-1688/def_cbu_web_im/index.html';
 assert.equal(OPSupplier.chatURL('https://detail.1688.com/offer/984933052528.html'),'');
 assert.equal(OPSupplier.chatURL(page+'?siteid=cnalichn'),'');
 assert.equal(OPSupplier.chatURL(page+'?siteid=cnalichn&touid=cnalichnabc&tracking=1'),page+'?touid=cnalichnabc&siteid=cnalichn&status=1');
});
