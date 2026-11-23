(()=>{
 const selectors=[
  'a[data-trace="SHOP_CREDIT"][href] h1','[data-company-name]','[data-seller-name]','[itemprop="legalName"]',
  '[itemtype*="Organization"] [itemprop="name"]',
  '.company-name','.company-title','.shop-name','[class*="shopTitle"]','[class*="shop-title"]','[class*="sellerName"]','[class*="companyName"]','[class*="company-name"]'
 ];
 const sellerScope='[data-current-supplier],[data-role="offer-seller"],[data-role="current-supplier"],.od-pc-offer-company,.od-pc-offer-detail-company';
 const unrelatedScope='footer,[data-role="recommendation"],[class*="recommend"],[class*="Recommend"]';
 function cleanName(value){
  const name=String(value||'').replace(/\s+/g,' ').trim();
  if(name.length<2||name.length>120||/^(进入店铺|查看店铺|店铺|公司|厂家|供应商|联系卖家|旺旺|详情|诚信通|实力商家)$/.test(name)||/[<>\r\n]|https?:\/\//i.test(name))return '';
  return name;
 }
 function companyNodes(doc){
  const result=new Map();
  for(const selector of selectors)for(const el of doc.querySelectorAll(selector)){
   if(!el.isConnected||!el.getClientRects().length||getComputedStyle(el).visibility==='hidden')continue;
   const name=cleanName(el.getAttribute('data-company-name')||el.getAttribute('data-seller-name')||el.textContent);
   if(name)result.set(el,name);
  }
  return result;
 }
 function supplierNames(doc){return [...new Set(companyNodes(doc).values())]}
 function sellerLink(scope){
  const links=[...(scope.matches?.('a[href]')?[scope]:[]),...scope.querySelectorAll('a[href]')];
  return links.some(a=>{
   try{const u=new URL(a.getAttribute('href'), 'https://detail.1688.com');return ['http:','https:'].includes(u.protocol)&&/^[a-z0-9-]+\.1688\.com$/i.test(u.hostname)&&!['detail.1688.com','www.1688.com','login.1688.com','order.1688.com','buyer.1688.com'].includes(u.hostname)}catch{return false}
  });
 }
 function topLeftSeller(el,doc){
  const r=el.getBoundingClientRect?.();if(!r)return false;
  const view=doc.defaultView||globalThis,width=view.innerWidth||1440,height=view.innerHeight||900;
  if(r.left<0||r.top<0||r.left>Math.min(700,width*.55)||r.top>Math.min(360,height*.4))return false;
  // The name was already found through a company/shop semantic selector.
  // Require a shop link in its own header/compact ancestor, not any page link.
  const header=el.closest?.('header,[class*="shop-header"],[class*="shopHeader"],[class*="company-header"],[class*="companyHeader"],[class*="shop-info"],[class*="shopInfo"]');
  if(header&&sellerLink(header))return true;
  let parent=el;
  for(let i=0;parent&&i<3;i++,parent=parent.parentElement){
   if(['BODY','HTML','MAIN'].includes(parent.tagName))break;
   if(sellerLink(parent))return true;
  }
  return false;
 }
 const chatPage='https://air.1688.com/app/ocms-fusion-components-1688/def_cbu_web_im/index.html';
 function chatURL(raw){
  try{
   const url=new URL(raw,'https://detail.1688.com');
   const nick=url.searchParams.get('touid');
   if(url.origin!=='https://air.1688.com'||url.username||url.password||
     url.pathname!=='/app/ocms-fusion-components-1688/def_cbu_web_im/index.html'||
     !nick?.startsWith('cnalichn')||!nick.slice(8).trim()||url.searchParams.get('siteid')!=='cnalichn')return '';
   url.search=new URLSearchParams({touid:nick,siteid:'cnalichn',status:'1'});
   url.hash='';
   return url.href;
  }catch{return ''}
 }
 function isContactLabel(el){
  const label=String(el?.getAttribute?.('aria-label')||el?.textContent||'').trim();
  return ['客服','联系卖家','联系客服'].includes(label);
 }
 function contactButton(scope){
  const controls=scope.querySelectorAll('a,button,[role="button"],span,div');
  for(const el of controls){
   if(!isContactLabel(el)||!el.getClientRects?.().length||el.closest?.(unrelatedScope))continue;
   return el.closest?.('a,button,[role="button"]')||el;
  }
  return null;
 }
 function supplierReading(doc,offerId){
  const nodes=companyNodes(doc),names=[...new Set(nodes.values())],trusted=new Set(),topLeft=new Set(),scopes=new Map();
  for(const [el,name] of nodes){
   // A lone recommendation/footer company is still not the current seller.
   if(el.closest?.(unrelatedScope))continue;
   const owner=el.closest?.('[data-offer-id]');if(owner&&String(owner.getAttribute('data-offer-id'))!==String(offerId))continue;
   if(topLeftSeller(el,doc))topLeft.add(name);
   // Current shop headers can render the three actions as buttons without shop links.
   for(let parent=el.parentElement,depth=0;parent&&depth<5;parent=parent.parentElement,depth++){
    if(['BODY','HTML','MAIN'].includes(parent.tagName)||parent.closest?.(unrelatedScope))break;
    if(parent.textContent?.includes('关注')&&parent.textContent?.includes('商品')&&contactButton(parent)){
     if(!scopes.has(name))scopes.set(name,new Set());
     scopes.get(name).add(parent);
     topLeft.add(name);
     break;
    }
   }
   const scope=el.closest?.(sellerScope);
   if(scope&&sellerLink(scope)){trusted.add(name);if(!scopes.has(name))scopes.set(name,new Set());scopes.get(name).add(scope)}
   const header=el.closest?.('header,[class*="shop-header"],[class*="shopHeader"],[class*="company-header"],[class*="companyHeader"],[class*="shop-info"],[class*="shopInfo"]');
   if(header&&topLeft.has(name)&&sellerLink(header)){if(!scopes.has(name))scopes.set(name,new Set());scopes.get(name).add(header)}
  }
  const preferred=topLeft.size?topLeft:trusted;
  const automaticName=preferred.size===1?[...preferred][0]:'';
  const links=new Set(),chatButtons=new Set();
  for(const scope of scopes.get(automaticName)||[]){
   for(const link of scope.querySelectorAll('a[href]')){
    if(link.closest?.(unrelatedScope))continue;
    const href=chatURL(link.getAttribute('href'));
    if(href)links.add(href);
   }
   const button=contactButton(scope);
   if(button)chatButtons.add(button);
   const href=chatURL(button?.getAttribute('data-chat-url'));
   if(href)links.add(href);
   const touid=button?.getAttribute('data-touid');
   if(touid?.startsWith('cnalichn')){
    const href=chatURL(chatPage+'?'+new URLSearchParams({touid,siteid:'cnalichn'}));
    if(href)links.add(href);
   }
  }
  return {names,automaticName,chatUrl:links.size===1?[...links][0]:'',chatButton:chatButtons.size===1?[...chatButtons][0]:null};
 }
 function supplierResultText(result){
  if(!result||!['assigned','updated','retained'].every(key=>Number.isSafeInteger(result[key])&&result[key]>=0)||typeof result.supplierName!=='string'||!result.supplierName.trim())throw Error('后端尚未支持厂家名称更新，请更新并重启工作台后端');
  const name=result.supplierName;
  if(!result.assigned&&!result.updated)return `厂家名称相同（${name}），保持不变。`;
  const changes=[`${result.created?'已新增厂家':'已复用厂家'}「${name}」`];
  if(result.assigned)changes.push(`新增 ${result.assigned} 条货源关联`);
  if(result.updated)changes.push(`已更新 ${result.updated} 条货源厂家关联`);
  if(result.retained)changes.push(`${result.retained} 条同名关联保持不变`);
  return changes.join('；')+'。'+(result.cacheRefreshed===false?' 厂家已保存，汇总刷新失败，请在工作台刷新采购汇总。':'');
 }
 globalThis.OPSupplier={cleanName,supplierNames,supplierReading,supplierResultText,chatURL};
})();
