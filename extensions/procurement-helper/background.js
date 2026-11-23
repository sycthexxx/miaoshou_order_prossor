import './core.js';
import './supplier.js';
import {registerWorkbench,hostPattern} from './connection.js';
const {snapshot,offerId}=globalThis.OPProcurement;
const defaults=['http://127.0.0.1:17381','http://localhost:17381'];
async function allowedOrigin(url){const {workbenchOrigin}=await chrome.storage.local.get('workbenchOrigin');return [...defaults,workbenchOrigin].includes(new URL(url).origin)}
const connecting=new Map();
async function findWorkbench(origin){
 const tabs=await chrome.tabs.query({url:hostPattern(origin)});
 let tab=tabs.find(tab=>tab.url&&new URL(tab.url).origin===origin);
 if(!tab)tab=await chrome.tabs.create({url:origin+'/#/procurement',active:false});
 for(let i=0;tab.status==='loading'&&i<60;i++){
  await new Promise(resolve=>setTimeout(resolve,250));
  tab=await chrome.tabs.get(tab.id);
 }
 if(!tab.url||new URL(tab.url).origin!==origin)throw Error('后台地址已变更，请检查插件设置');
 await chrome.scripting.executeScript({target:{tabId:tab.id},files:['bridge.js']});
 return tab;
}
function workbenchTab(origin){
 if(!connecting.has(origin))connecting.set(origin,findWorkbench(origin).finally(()=>connecting.delete(origin)));
 return connecting.get(origin);
}
const chatAttempts=new Map();
// Bind a popup to the offer that triggered it, even when it starts on a blank page.
chrome.webNavigation.onCreatedNavigationTarget.addListener(({sourceTabId,sourceFrameId,tabId})=>{
 const attempt=chatAttempts.get(sourceTabId);
 if(attempt?.accepting&&sourceFrameId===0&&attempt.tabId===undefined)attempt.tabId=tabId;
});
chrome.webNavigation.onCommitted.addListener(({tabId,frameId,url})=>{
 if(frameId!==0)return;
 for(const attempt of chatAttempts.values()){
  if(attempt.tabId!==tabId)continue;
  const chatUrl=OPSupplier.chatURL(url);
  if(chatUrl&&attempt.accepting){
   attempt.accepting=false;
   attempt.chatUrl=chatUrl;
   attempt.resolve(chatUrl);
  }
  return;
 }
},{url:[{hostEquals:'air.1688.com',schemes:['https']}]});

chrome.runtime.onMessage.addListener((message,sender,reply)=>{
 (async()=>{
  if(message.type==='BRIDGE_STATUS'){
   if(!sender.tab||sender.frameId!==0||!await allowedOrigin(sender.url))throw Error('未连接当前工作台，请点击插件图标保存并授权此地址');
   return {ok:true,version:chrome.runtime.getManifest().version};
  }
  if(message.type==='OPEN_PROCUREMENT'){
   if(!sender.tab||sender.frameId!==0||!await allowedOrigin(sender.url))throw Error('请先在插件设置中配置当前工作台地址');
   const p=snapshot(message.payload,true);
   if(!/^[1-9][0-9]*$/.test(p.account))throw Error('账户标识无效');
   p.workbench={tabId:sender.tab.id,origin:new URL(sender.url).origin};
   const tab=await chrome.tabs.create({url:'about:blank',active:true});
   try{await chrome.storage.session.set({['task:'+tab.id]:p});await chrome.tabs.update(tab.id,{url:p.url})}catch(e){await chrome.tabs.remove(tab.id);throw e}
   return {ok:true};
  }
  if(message.type==='GET_PROCUREMENT'){
   const id=offerId(sender.url);if(!sender.tab||!id||sender.frameId!==0)throw Error('无效货源页面');
   const key='task:'+sender.tab.id;const data=await chrome.storage.session.get(key);let task=data[key];
   if(task?.offerId!==id||task?.direct){
    const {workbenchOrigin:connectURL}=await chrome.storage.local.get('workbenchOrigin');
    if(!connectURL)return {ok:true,task:null,reason:'请点击采购助手图标，保存并授权后台地址。'};
    await chrome.storage.session.remove(key);
    const tab=await workbenchTab(connectURL);
    const workbench={tabId:tab.id,origin:connectURL};
    const settings=await chrome.tabs.sendMessage(tab.id,{type:'GET_PLUGIN_QUERY_SETTINGS',origin:connectURL},{frameId:0});
    if(!settings?.ok)throw Error(settings?.error||'读取后台插件设置失败');
    const {account,gifts}=settings.result;
    if(!/^[1-9][0-9]*$/.test(account))return {ok:true,task:null,connectURL,reason:'请在后台账户管理中添加查询账户。'};
    const response=await chrome.tabs.sendMessage(tab.id,{type:'GET_PURCHASE_OFFER',origin:connectURL,account,gifts,productId:id},{frameId:0});
    if(!response?.ok)return {ok:false,task:null,connectURL,error:response?.error||'查询失败'};
    if(!response.result.found){await chrome.storage.session.remove(key);return {ok:true,task:null,connectURL,reason:`账户 #${account}（${gifts?'包含':'不含'}赠品）的采购缓存中没有此货源的待采购商品。缓存时间：${new Date(response.result.createdAt).toLocaleString()}。需要更新时在工作台点击刷新采购汇总。`}};
    const p=snapshot(response.result.snapshot);if(p.account!==account||p.offerId!==id)throw Error('返回清单账户或货源不一致');
    task={...p,workbench,direct:true,gifts};await chrome.storage.session.set({[key]:task});
   }

   const {workbench,...publicTask}=task;return {ok:true,task:publicTask,connectURL:workbench.origin};
  }
  if(message.type==='PREPARE_CHAT_CAPTURE'||message.type==='WAIT_CHAT_CAPTURE'){
   const id=offerId(sender.url),source=sender.tab?.id;
   if(!id||!source||sender.frameId!==0)throw Error('无效货源页面');
   const key='task:'+source,{[key]:task}=await chrome.storage.session.get(key);
   if(task?.offerId!==id||!task.workbench)throw Error('请从工作台重新打开货源');
   if(message.type==='WAIT_CHAT_CAPTURE'){
    const attempt=chatAttempts.get(source);
    return {ok:true,chatUrl:attempt?.offerId===id?await attempt.result:''};
   }
   if(chatAttempts.has(source))throw Error('本页已尝试打开客服');
   const attempt={offerId:id,accepting:true};
   attempt.result=new Promise(resolve=>{attempt.resolve=resolve});
   chatAttempts.set(source,attempt);
   function finishWithoutLink(){
    attempt.accepting=false;
    attempt.resolve('');
   }
   // Popup blockers and embedded chats create no tab, so use the shorter wait in that case.
   setTimeout(()=>{if(attempt.tabId===undefined)finishWithoutLink()},2500).unref?.();
   setTimeout(finishWithoutLink,9000).unref?.();
   setTimeout(()=>{if(chatAttempts.get(source)===attempt)chatAttempts.delete(source)},30000).unref?.();
   return {ok:true};
  }
  if(['REPORT_SUPPLIER','GET_PROCUREMENT_MAPPING','PUT_PROCUREMENT_MAPPING','REPORT_SOURCE_STATUS'].includes(message.type)){
   const id=offerId(sender.url);if(!sender.tab||!id||sender.frameId!==0)throw Error('无效货源页面');
   const key='task:'+sender.tab.id,{[key]:task}=await chrome.storage.session.get(key);
   if(!task||task.offerId!==id||!task.workbench)throw Error('请重新从采购汇总打开货源');
   const name=String(message.name||'').trim();if(message.type==='REPORT_SUPPLIER'&&(!name||name.length>120))throw Error('厂家名称无效');
   let tab;try{tab=await chrome.tabs.get(task.workbench.tabId)}catch{throw Error('工作台标签页已关闭，请重新从采购汇总打开货源')}
   if(new URL(tab.url).origin!==task.workbench.origin||!await allowedOrigin(tab.url))throw Error('原工作台页面已离开，请重新从采购汇总打开货源');
   // Destination and account come from the opening workbench, never from 1688.
   const request={type:message.type==='REPORT_SOURCE_STATUS'?'SAVE_SOURCE_STATUS':message.type==='REPORT_SUPPLIER'?'SAVE_CAPTURED_SUPPLIER':message.type==='GET_PROCUREMENT_MAPPING'?'GET_PURCHASE_MAPPING':'PUT_PURCHASE_MAPPING',origin:task.workbench.origin,account:task.account,productId:id};
   if(message.type==='REPORT_SUPPLIER'){
    request.name=name;
    request.chatUrl=message.chatUrl;
   }
   if(message.type==='REPORT_SOURCE_STATUS'){if(!['valid','invalid'].includes(message.state))throw Error('无效货源状态');request.state=message.state;request.reason=message.reason;request.evidence=message.evidence}
   if(message.type==='PUT_PROCUREMENT_MAPPING'){request.mapping=message.mapping;request.revision=message.revision}
   let result;
   try{result=await chrome.tabs.sendMessage(tab.id,request,{frameId:0})}catch{throw Error('无法连接工作台，请刷新工作台并重新打开货源')}
   const attempt=chatAttempts.get(sender.tab.id);
   if(result?.ok&&message.type==='REPORT_SUPPLIER'&&attempt?.offerId===id&&attempt.chatUrl&&attempt.chatUrl===request.chatUrl){
    chatAttempts.delete(sender.tab.id);
    // The user may have navigated the popup elsewhere while the workbench was saving.
    const frame=await chrome.webNavigation.getFrame({tabId:attempt.tabId,frameId:0}).catch(()=>null);
    if(frame&&OPSupplier.chatURL(frame.url)===attempt.chatUrl)await chrome.tabs.remove(attempt.tabId).catch(()=>{});
   }
   return result;
  }
  throw Error('不支持的消息');
 })().then(reply,e=>reply({ok:false,error:e.message}));
 return true;
});
chrome.tabs.onRemoved.addListener(id=>{chatAttempts.delete(id);void chrome.storage.session.remove('task:'+id)});

async function repairRegistration(){const {workbenchOrigin}=await chrome.storage.local.get('workbenchOrigin');if(workbenchOrigin&&await chrome.permissions.contains({origins:[hostPattern(workbenchOrigin)]}))await registerWorkbench(chrome,workbenchOrigin)}
chrome.runtime.onInstalled.addListener(()=>{void repairRegistration().catch(()=>{})});
chrome.runtime.onStartup.addListener(()=>{void repairRegistration().catch(()=>{})});
