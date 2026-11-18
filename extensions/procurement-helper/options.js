import {workbenchOrigin,hostPattern,suggestedOrigin,registerWorkbench} from './connection.js';
const field=document.querySelector('#origin'),result=document.querySelector('#result');
(async()=>{
 const saved=await chrome.storage.local.get('workbenchOrigin');
 const [tab]=await chrome.tabs.query({active:true,currentWindow:true});
 field.value=suggestedOrigin(tab?.url,saved.workbenchOrigin);
 result.textContent=field.value?'请确认这是当前电脑实际访问的工作台地址，再点击保存授权。':'先打开公司工作台，再点击插件图标；或填写工作台内网地址。不要填写这台员工电脑的 localhost。';
})().catch(()=>{result.textContent='请填写公司工作台的实际访问地址。'});
document.querySelector('#form').addEventListener('submit',async e=>{
 e.preventDefault();result.textContent='';
 try{
  const origin=workbenchOrigin(field.value),pattern=hostPattern(origin);
  if(!await chrome.permissions.request({origins:[pattern]}))throw Error('未授权，工作台尚未连接');
  // Store first: immediately injected bridges must pass the exact-origin check.
  await chrome.storage.local.set({workbenchOrigin:origin});
  await registerWorkbench(chrome,origin);
  const tabs=await chrome.tabs.query({url:[pattern]});let connected=0;
  for(const tab of tabs){if(new URL(tab.url).origin!==origin)continue;try{await chrome.scripting.executeScript({target:{tabId:tab.id},files:['bridge.js']});await chrome.tabs.sendMessage(tab.id,{type:'REFRESH_WORKBENCH_BRIDGE'},{frameId:0});connected++}catch{}}
  result.textContent=connected?`已连接 ${connected} 个工作台页面。回到采购汇总点击“打开货源”。`:'已授权。请打开此工作台，刷新采购汇总后再点击“打开货源”。';
 }catch(e){result.textContent=e.message}
});
