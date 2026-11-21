(()=>{
 const excluded='footer,nav,[data-role="recommendation"],[class*="recommend"],[class*="Recommend"]';
 const scopes='[data-offer-id],[data-role="offer-detail"],#mod-detail,#detail,.od-pc-offer-detail';
 const messages='[data-role="offer-status"],.offer-status,.offer-error,[class*="offer-offline"],[class*="offer-not-found"],h1,h2,[role="alert"]';
 const visible=el=>el.isConnected&&el.getClientRects().length&&getComputedStyle(el).visibility!=='hidden';
 function own(el,id){const owner=el.closest?.('[data-offer-id]');return !owner||owner.getAttribute('data-offer-id')===String(id)}
 function unavailableText(text){
  const value=String(text||'').replace(/\s+/g,'').trim();
  if(value.length>100||/登录|验证|网络|重试|繁忙|访问频繁/.test(value))return null;
  if(/^(?:很抱歉[，,！!]?|抱歉[，,！!]?|您访问的|您要查看的|该|此|当前)*(?:商品|产品|货品)(?:已被|已经|已)?(?:下架|删除)(?:[，,。.!！].*)?$/.test(value))return {state:'invalid',reason:'1688商品已下架或删除',evidence:'offer_removed'};
  if(/^(?:很抱歉[，,！!]?|抱歉[，,！!]?|您访问的|您要查看的|该|此|当前)*(?:商品|产品|货品)(?:信息)?不存在[。.!！]?$/.test(value))return {state:'invalid',reason:'1688商品不存在',evidence:'offer_not_found'};
  return null;
 }
 function unavailablePane(doc,id){
  // Offline pages can replace the entire normal detail component with plain divs.
  // Require the standalone top-page message AND the shop's other-products action
  // in one compact pane, never a recommendation card or another offer's wrapper.
  for(const node of doc.querySelectorAll('h1,h2,h3,p,div,span')){
   if(!visible(node)||!own(node,id)||node.closest?.(excluded))continue;
   const reading=unavailableText(node.textContent);if(!reading)continue;
   const r=node.getBoundingClientRect?.(),height=doc.defaultView?.innerHeight||900;
   if(!r||r.top<0||r.top>Math.min(650,height*.75))continue;
   let pane=node.parentElement;
   for(let i=0;pane&&i<4;i++,pane=pane.parentElement){
    if(['BODY','HTML'].includes(pane.tagName)||!own(pane,id)||pane.closest?.(excluded))break;
    const text=(pane.textContent||'').replace(/\s+/g,'');if(text.length>350)break;
    const actions=[...pane.querySelectorAll('a,button,[role="button"]')];
    const other=actions.some(a=>visible(a)&&own(a,id)&&!a.closest?.(excluded)&&/^(查看(?:该|本)?店铺其他(?:上架)?商品|查看其他商品)[>›»]?$/u.test((a.textContent||'').replace(/\s+/g,'')));
    const anotherOffer=[...pane.querySelectorAll('a[href]')].some(a=>{try{const u=new URL(a.getAttribute('href'),'https://detail.1688.com');const offer=u.hostname==='detail.1688.com'&&u.pathname.match(/^\/offer\/(\d+)\.html$/);return offer&&offer[1]!==String(id)}catch{return false}});
    if(other&&!anotherOffer)return reading;
   }
  }
  return null;
 }
 function inspect(doc,id){
  // Read only a current-offer scope. A lone error elsewhere on the page is not evidence.
  if([...doc.querySelectorAll('[role="dialog"],[class*="captcha"],[id*="captcha"]')].some(el=>visible(el)&&/登录|验证|验证码/.test(el.textContent||'')))return null;
  const offline=unavailablePane(doc,id);if(offline)return offline;
  const roots=[...doc.querySelectorAll(scopes)].filter(el=>visible(el)&&own(el,id)&&!el.closest?.(excluded));
  const found=[];
  for(const root of roots){
   for(const node of root.querySelectorAll(messages)){
    if(!visible(node)||!own(node,id)||node.closest?.(excluded))continue;
    const text=(node.textContent||'').replace(/\s+/g,'').trim();
    if(/登录|验证|网络|重试|繁忙|访问频繁/.test(text))return null;
    const reading=unavailableText(text);if(reading)found.push(reading);
   }
   // Available means an enabled current-offer SKU input AND purchase action.
   const inputs=[...root.querySelectorAll('input[role="spinbutton"],input[type="number"],[data-role="sku-quantity"]')].filter(el=>visible(el)&&own(el,id)&&!el.closest?.(excluded)&&!el.disabled);
   const buy=[...root.querySelectorAll('button,[role="button"]')].some(el=>visible(el)&&own(el,id)&&!el.closest?.(excluded)&&!el.disabled&&el.getAttribute('aria-disabled')!=='true'&&/^(立即下单|加入采购车|加采购车)$/.test((el.textContent||'').trim()));
   if(inputs.length&&buy)found.push({state:'valid',reason:'1688当前商品规格与采购区域可用'});
  }
  const states=new Set(found.map(x=>x.state));return states.size===1?found[0]:null;
 }
 function resultText(result){
  const s=result?.sourceStatus;if(!s)return '未收到货源状态，请重试检查';
  if(s.state==='unknown')return '后台货源状态：未确认。';
  const label=s.state==='invalid'?'无效':s.state==='valid'?'有效':'未确认';
  let text=result.retainedManual?'保留人工标记的无效状态，请在工作台人工恢复。':`货源${label}${s.reason?'：'+s.reason:''}（${s.source==='manual'?'人工标记':s.source==='plugin'?'插件检查':'后台状态'}）`;
  if(result.cacheRefreshed===false)text+='；状态已保存，采购汇总刷新失败，请在工作台刷新。';return text;
 }
 globalThis.OPSourceStatus={inspect,resultText};
})();
