import test from 'node:test';import assert from 'node:assert/strict';
import '../../extensions/procurement-helper/source-status.js';
globalThis.getComputedStyle=()=>({visibility:'visible'});
const node=(text,{owner='100',excluded=false,disabled=false}={})=>({isConnected:true,textContent:text,disabled,getClientRects:()=>[{}],getAttribute:key=>key==='data-offer-id'?owner:null,closest:selector=>selector==='[data-offer-id]'?{getAttribute:()=>owner}:excluded?{}:null});
function page({text='',owner='100',excluded=false,available=false,disabled=false}={}){
 const root=node('',{owner});root.querySelectorAll=selector=>selector.startsWith('[data-role="offer-status"]')?[node(text,{owner,excluded})]:selector.startsWith('input')?(available?[node('',{owner,disabled})]:[]):available?[node('立即下单',{owner,disabled,excluded})]:[];
 return {querySelectorAll:selector=>selector.startsWith('[role="dialog"]')?[]:[root]};
}
test('explicit removed and absent messages inside current offer are the only invalid evidence',()=>{
 assert.equal(OPSourceStatus.inspect(page({text:'该商品已下架'}),'100').evidence,'offer_removed');
 assert.equal(OPSourceStatus.inspect(page({text:'您访问的商品不存在'}),'100').evidence,'offer_not_found');
 for(const text of ['网络错误，请重试','请登录查看商品','请完成安全验证','暂无库存','商品未找到，请稍后重试',''])assert.equal(OPSourceStatus.inspect(page({text}),'100'),null);
});
test('recommendations, other offer and contradictory valid area cannot mark current offer invalid',()=>{
 assert.equal(OPSourceStatus.inspect(page({text:'商品已下架',excluded:true}),'100'),null);
 assert.equal(OPSourceStatus.inspect(page({text:'商品已下架',owner:'200'}),'100'),null);
 assert.equal(OPSourceStatus.inspect(page({text:'商品已下架',available:true}),'100'),null);
});
test('available requires enabled current SKU quantity AND purchase control',()=>{
 assert.equal(OPSourceStatus.inspect(page({available:true}),'100').state,'valid');
 assert.equal(OPSourceStatus.inspect(page({available:true,text:'请登录查看商品'}),'100'),null);
 assert.equal(OPSourceStatus.inspect(page({available:true,disabled:true}),'100'),null);
 assert.equal(OPSourceStatus.inspect(page({available:true,owner:'200'}),'100'),null);
 assert.equal(OPSourceStatus.inspect(page({available:true,excluded:true}),'100'),null);
});
test('manual invalid status is visibly retained and cache failure is not a write failure',()=>{
 assert.match(OPSourceStatus.resultText({sourceStatus:{state:'invalid',source:'manual'},retainedManual:true}),/保留人工标记/);
 assert.match(OPSourceStatus.resultText({sourceStatus:{state:'invalid',source:'plugin'},cacheRefreshed:false}),/状态已保存/);
});

test('saved manual invalid is still described when page evidence is absent',()=>{
 assert.equal(OPSourceStatus.inspect(page(),'100'),null);
 const text=OPSourceStatus.resultText({sourceStatus:{state:'invalid',source:'manual',reason:'removed manually'}});
 assert.match(text,/removed manually/);assert.match(text,/人工标记/);assert.match(text,/无效/);
});

function offlinePage({excluded=false,owner='100',top=180,action='查看该店铺其他上架商品',otherLink=false}={}){
 const label=node('商品已下架',{owner,excluded});label.getBoundingClientRect=()=>({top});
 const button=node(action,{owner,excluded}),pane=node('商品已下架 '+action,{owner,excluded});pane.tagName='DIV';
 pane.querySelectorAll=selector=>selector==='a[href]'?(otherLink?[{getAttribute:()=> 'https://detail.1688.com/offer/200.html'}]:[]):[button];label.parentElement=pane;
 const recommended=page({available:true,owner:'200'});
 return {defaultView:{innerHeight:900},querySelectorAll:selector=>selector==='h1,h2,h3,p,div,span'?[label]:selector.startsWith('[role="dialog"]')?[]:recommended.querySelectorAll(selector)};
}
test('plain offline pane plus view other shop products is detected without a legacy detail container',()=>{
 assert.equal(OPSourceStatus.inspect(offlinePage(),'100').evidence,'offer_removed');
 assert.equal(OPSourceStatus.inspect(offlinePage({action:'查看其他商品'}),'100').state,'invalid');
});
test('other available recommendations do not negate primary offline pane, but recommendation cards cannot declare it',()=>{
 assert.equal(OPSourceStatus.inspect(offlinePage(),'100').state,'invalid');
 for(const options of [{excluded:true},{owner:'200'},{top:800},{action:'进入店铺'},{otherLink:true}])assert.equal(OPSourceStatus.inspect(offlinePage(options),'100'),null);
});
