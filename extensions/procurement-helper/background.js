import './core.js';
import {registerWorkbench,hostPattern} from './connection.js';
const {snapshot,offerId}=globalThis.OPProcurement;
const defaults=['http://127.0.0.1:17381','http://localhost:17381'];
async function allowedOrigin(url){const {workbenchOrigin}=await chrome.storage.local.get('workbenchOrigin');return [...defaults,workbenchOrigin].includes(new URL(url).origin)}
chrome.runtime.onMessage.addListener((message,sender,reply)=>{
 (async()=>{
  if(message.type==='BRIDGE_STATUS'){
   if(!sender.tab||sender.frameId!==0||!await allowedOrigin(sender.url))throw Error('未连接当前工作台，请点击插件图标保存并授权此地址');
   return {ok:true,version:chrome.runtime.getManifest().version};
  }
  if(message.type==='SELECT_PROCUREMENT_CONTEXT'){
   if(!sender.tab||sender.frameId!==0||!await allowedOrigin(sender.url))throw Error('请先授权当前工作台');
   const account=String(message.payload?.account||'');if(!/^[1-9][0-9]*$/.test(account))throw Error('请选择账户');
   await chrome.storage.local.set({procurementContext:{account,gifts:message.payload.gifts===true,workbench:{tabId:sender.tab.id,origin:new URL(sender.url).origin}}});return {ok:true};
  }
  if(message.type==='OPEN_PROCUREMENT'){
   if(!sender.tab||sender.frameId!==0||!await allowedOrigin(sender.url))throw Error('请先在插件设置中配置当前工作台地址');
   const p=snapshot(message.payload);
   if(!/^[1-9][0-9]*$/.test(p.account))throw Error('账户标识无效');
   p.workbench={tabId:sender.tab.id,origin:new URL(sender.url).origin};
   await chrome.storage.local.set({procurementContext:{account:p.account,gifts:message.payload.gifts===true,workbench:p.workbench}});
   const tab=await chrome.tabs.create({url:'about:blank',active:true});
   try{await chrome.storage.session.set({['task:'+tab.id]:p});await chrome.tabs.update(tab.id,{url:p.url})}catch(e){await chrome.tabs.remove(tab.id);throw e}
   return {ok:true};
  }
  if(message.type==='GET_PROCUREMENT'){
   const id=offerId(sender.url);if(!sender.tab||!id||sender.frameId!==0)throw Error('无效货源页面');
   const key='task:'+sender.tab.id;const data=await chrome.storage.session.get(key);let task=data[key];
   if(task?.offerId!==id||message.reload===true&&task?.direct){
    const {procurementContext:context,workbenchOrigin}=await chrome.storage.local.get(['procurementContext','workbenchOrigin']);
    const connectURL=context?.workbench?.origin||workbenchOrigin||'';
    if(!context)return {ok:true,task:null,connectURL,reason:'请在工作台采购汇总选择账户，点击“设为插件查询账户”；不会自动选择第一个账户。'};
    const {workbench,account,gifts}=context;
    let tab;try{tab=await chrome.tabs.get(workbench.tabId)}catch{}
    if(message.connect===true&&(!tab||new URL(tab.url).origin!==workbench.origin)){
     if(!await allowedOrigin(workbench.origin))throw Error('所选服务器未授权，请先打开插件设置授权');
     tab=await chrome.tabs.create({url:workbench.origin+'/#/procurement',active:false});
     for(let attempt=0;attempt<60;attempt++){tab=await chrome.tabs.get(tab.id);if(tab.status==='complete')break;await new Promise(r=>setTimeout(r,250))}
     await chrome.scripting.executeScript({target:{tabId:tab.id},files:['bridge.js']});
     const ready=await chrome.tabs.sendMessage(tab.id,{type:'REFRESH_WORKBENCH_BRIDGE'},{frameId:0});if(!ready?.ok)throw Error('工作台未连接，请刷新后重试');
     workbench.tabId=tab.id;await chrome.storage.local.set({procurementContext:context});
    }
    if(!tab)return {ok:true,task:null,connectURL,reason:'工作台标签页已关闭。点击“连接工作台并查询”，将使用已选择的账户重新连接。'};
    if(new URL(tab.url).origin!==workbench.origin||!await allowedOrigin(tab.url))return {ok:false,task:null,connectURL,error:'原工作台已离开或未授权，请重新选择查询账户'};
    let response;try{response=await chrome.tabs.sendMessage(tab.id,{type:'GET_PURCHASE_OFFER',origin:workbench.origin,account,productId:id,gifts},{frameId:0})}catch{return {ok:false,task:null,connectURL,error:'工作台连接失效，请刷新工作台并重新选择账户'}}
    if(!response?.ok)return {ok:false,task:null,connectURL,error:response?.error||'查询失败'};
    if(!response.result.found){await chrome.storage.session.remove(key);return {ok:true,task:null,connectURL,reason:`账户 #${account}（${gifts?'包含':'不含'}赠品）的采购缓存中没有此货源的待采购商品。缓存时间：${new Date(response.result.createdAt).toLocaleString()}。需要更新时在工作台点击刷新采购汇总。`}};
    const p=snapshot(response.result.snapshot);if(p.account!==account||p.offerId!==id)throw Error('返回清单账户或货源不一致');
    task={...p,workbench,direct:true,gifts};await chrome.storage.session.set({[key]:task});
   }

   const {workbench,...publicTask}=task;return {ok:true,task:publicTask,connectURL:workbench.origin};
  }
  if(['REPORT_SUPPLIER','GET_PROCUREMENT_MAPPING','PUT_PROCUREMENT_MAPPING'].includes(message.type)){
   const id=offerId(sender.url);if(!sender.tab||!id||sender.frameId!==0)throw Error('无效货源页面');
   const key='task:'+sender.tab.id,{[key]:task}=await chrome.storage.session.get(key);
   if(!task||task.offerId!==id||!task.workbench)throw Error('请重新从采购汇总打开货源');
   const name=String(message.name||'').trim();if(message.type==='REPORT_SUPPLIER'&&(!name||name.length>120))throw Error('厂家名称无效');
   let tab;try{tab=await chrome.tabs.get(task.workbench.tabId)}catch{throw Error('工作台标签页已关闭，请重新从采购汇总打开货源')}
   if(new URL(tab.url).origin!==task.workbench.origin||!await allowedOrigin(tab.url))throw Error('原工作台页面已离开，请重新从采购汇总打开货源');
   // Destination and account come from the opening workbench, never from 1688.
   const request={type:message.type==='REPORT_SUPPLIER'?'SAVE_CAPTURED_SUPPLIER':message.type==='GET_PROCUREMENT_MAPPING'?'GET_PURCHASE_MAPPING':'PUT_PURCHASE_MAPPING',origin:task.workbench.origin,account:task.account,productId:id};
   if(message.type==='REPORT_SUPPLIER')request.name=name;
   if(message.type==='PUT_PROCUREMENT_MAPPING'){request.mapping=message.mapping;request.revision=message.revision}
   try{return await chrome.tabs.sendMessage(tab.id,request,{frameId:0})}catch{throw Error('无法连接工作台，请刷新工作台并重新打开货源')}
  }
  throw Error('不支持的消息');
 })().then(reply,e=>reply({ok:false,error:e.message}));
 return true;
});
chrome.tabs.onRemoved.addListener(id=>{void chrome.storage.session.remove('task:'+id)});

async function repairRegistration(){const {workbenchOrigin}=await chrome.storage.local.get('workbenchOrigin');if(workbenchOrigin&&await chrome.permissions.contains({origins:[hostPattern(workbenchOrigin)]}))await registerWorkbench(chrome,workbenchOrigin)}
chrome.runtime.onInstalled.addListener(()=>{void repairRegistration().catch(()=>{})});
chrome.runtime.onStartup.addListener(()=>{void repairRegistration().catch(()=>{})});
