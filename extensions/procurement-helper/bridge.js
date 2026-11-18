(()=>{
 if(typeof globalThis.__opProcurementBridge?.refresh==='function'){globalThis.__opProcurementBridge.refresh();return}
 function state(ready,error=''){
  document.documentElement.dataset.opProcurementBridgeReady=ready?'0.1.12':'';
  document.documentElement.dataset.opProcurementBridgeError=error;
  window.dispatchEvent(new CustomEvent('op-procurement-bridge-status'));
 }
 async function refresh(){try{const r=await chrome.runtime.sendMessage({type:'BRIDGE_STATUS'});state(!!r?.ok,r?.error||'')}catch{state(false,'插件已更新，请刷新工作台页面')}}
 globalThis.__opProcurementBridge={refresh};
 chrome.runtime.onMessage.addListener((message,sender,reply)=>{
  if(sender.id!==chrome.runtime.id)return;
  if(message.type==='REFRESH_WORKBENCH_BRIDGE'){void refresh().then(()=>reply({ok:true}));return true}
  const settings=message.type==='GET_PLUGIN_QUERY_SETTINGS';
  if(!settings&&!['SAVE_CAPTURED_SUPPLIER','GET_PURCHASE_MAPPING','PUT_PURCHASE_MAPPING','GET_PURCHASE_OFFER'].includes(message.type))return;
  if(message.origin!==location.origin||(!settings&&(!/^\d+$/.test(message.account)||!/^\d+$/.test(message.productId)))){reply({ok:false,error:'货源回传来源无效'});return}
  const supplier=message.type==='SAVE_CAPTURED_SUPPLIER',offer=message.type==='GET_PURCHASE_OFFER',read=settings||offer||message.type==='GET_PURCHASE_MAPPING';
  const url=settings?'/api/plugin-query-settings':offer?`/api/accounts/${message.account}/procurement/offers/${message.productId}?gifts=${message.gifts?1:0}`:supplier?`/api/accounts/${message.account}/source-supplier/capture`:`/api/accounts/${message.account}/procurement-mappings/${message.productId}`;
  const options={method:read?'GET':supplier?'POST':'PUT',credentials:'same-origin',signal:AbortSignal.timeout(15000)};
  if(!read){options.headers={'Content-Type':'application/json'};options.body=JSON.stringify(supplier?{productId:message.productId,name:message.name}:{revision:message.revision,mapping:message.mapping})}
  fetch(url,options).then(async r=>{
    if(r.status===404)throw Error('后端未提供此接口或货源不存在，请更新并重启工作台后端后重试');
    const data=await r.json();reply({ok:r.ok,result:data,status:r.status,...(!r.ok?{error:data.error||'工作台保存失败'}:{})});
   }).catch(e=>reply({ok:false,error:e.message||'无法连接工作台'}));
  return true;
 });
 function open(event){
  const a=event.target.closest?.('a[data-op-procurement]');if(!a||!event.isTrusted||(event.type==='click'?event.button!==0:event.button!==1))return;
  event.preventDefault();event.stopImmediatePropagation();
  let payload;try{payload=JSON.parse(a.dataset.opProcurement)}catch{state(false,'采购清单无效，请刷新汇总');alert('采购清单无效，请刷新汇总');return}
  chrome.runtime.sendMessage({type:'OPEN_PROCUREMENT',payload}).then(r=>{if(!r?.ok){state(false,r?.error);alert(r?.error||'采购助手未能打开货源')}}).catch(()=>{state(false,'插件已更新，请刷新工作台页面');alert('采购助手已更新，请刷新工作台页面再打开货源')});
 }
 document.addEventListener('click',open,true);document.addEventListener('auxclick',open,true);
 window.addEventListener('focus',refresh);window.addEventListener('op-procurement-check-bridge',refresh);
 void refresh();
})();
