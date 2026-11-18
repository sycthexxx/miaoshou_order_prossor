export function workbenchOrigin(value){
 const u=new URL(String(value).trim());
 if(!['http:','https:'].includes(u.protocol)||u.username||u.password||u.pathname!=='/'||u.search||u.hash)throw Error('请填写工作台的协议、主机和端口，不包含路径');
 return u.origin;
}
export function hostPattern(origin){const u=new URL(origin);return `${u.protocol}//${u.hostname}/*`}
export function suggestedOrigin(tabURL,saved){
 try{const u=new URL(tabURL);if(['http:','https:'].includes(u.protocol)&&['#/procurement','#/accounts','#/orders','#/products','#/suppliers'].some(p=>u.hash.startsWith(p)))return u.origin}catch{}
 return saved||'';
}
export async function registerWorkbench(chrome,origin){
 const registered=await chrome.scripting.getRegisteredContentScripts({ids:['workbench']});
 if(registered.length)await chrome.scripting.unregisterContentScripts({ids:['workbench']});
 const u=new URL(origin);
 if(!(u.protocol==='http:'&&['localhost','127.0.0.1'].includes(u.hostname)))await chrome.scripting.registerContentScripts([{id:'workbench',matches:[hostPattern(origin)],js:['bridge.js'],runAt:'document_idle'}]);
}
