import path from 'node:path';
import {mkdir,writeFile} from 'node:fs/promises';


// Resolve the request client used by this version of the ERP page. Its native
// interceptors own the session timestamp/signature and nested form encoding.
export function requestClientExport(source){
 const call=source.match(/function [\w$]+\(\w+\)\{return ([\w$]+)\(\{url:`[^`]*\/manualRelateOrderPackagePurchaseOrders`/);
 if(!call)throw new Error('妙手提交接口代码已变化，需更新网页适配器');
 const name=call[1].replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
 const exported=source.slice(source.lastIndexOf('export{')).match(new RegExp('(?:\\{|,)'+name+' as ([\\w$]+)(?:,|\\})'));
 if(!exported)throw new Error('未找到妙手网页请求客户端，需更新网页适配器');
 return exported[1];
}

export async function nativeClient(page){
 const urls=await page.locator('script[type="module"][src]').evaluateAll(nodes=>nodes.map(n=>n.src));
 const url=urls.find(u=>/\/index-[^/]+\.js(?:\?|$)/.test(u));
 if(!url)throw new Error('请先在账户管理打开妙手订单页并完成登录');
 const response=await page.request.get(url,{timeout:15000});
 if(!response.ok())throw new Error('无法读取妙手网页请求客户端');
 return {url,name:requestClientExport(await response.text())};
}

async function attemptDirectory(root,id,operation){
 const parent=path.join(root,'runtime',id,operation);await mkdir(parent,{recursive:true,mode:0o700});
 const stamp=new Date().toISOString().replace(/[:.]/g,'-');
 for(let n=1;;n++){const dir=path.join(parent,`${stamp}-${n}`);try{await mkdir(dir,{mode:0o700});return dir}catch(e){if(e.code!=='EEXIST')throw e}}
}
const save=(dir,name,value)=>writeFile(path.join(dir,name),typeof value==='string'?value:JSON.stringify(value,null,2),{mode:0o600});

export async function submitRequest({root,id,session,payload,endpoint,operation,resolveClient=nativeClient}){
 const page=session.context.pages().find(p=>p.url().startsWith('https://erp.91miaoshou.com/order/'));
 if(!page)return {state:'not_sent',error:'请先打开妙手订单页面'};
 let client,dir;
 try{client=await resolveClient(page);dir=await attemptDirectory(root,id,operation);await save(dir,'request.json',payload);await save(dir,'request.meta.json',{endpoint,method:'POST'})}
 catch(e){return {state:'not_sent',error:e.message}}
 const archive=path.relative(root,dir);let sent=false,response,requestSaved=false;
 const url='https://erp.91miaoshou.com'+endpoint;
 // Store the exact form produced by ERP before allowing the write to leave.
 const route=async route=>{
  const request=route.request();
  if(request.method()!=='POST')return route.fallback();
  try{await save(dir,'request.form.txt',request.postData()||'');requestSaved=true;sent=true;await route.fallback()}
  catch{await route.abort('failed')}
 };
 const onResponse=r=>{if(r.url()===url&&r.request().method()==='POST')response=r};
 page.on('response',onResponse);
 let result;
 try{
  await page.route(url,route);
  try{await page.evaluate(async({client,payload,endpoint})=>{
   const module=await import(client.url);
   await module[client.name]({url:endpoint.replace(/^\/api/,''),method:'POST',data:payload,timeout:45000});
  },{client,payload,endpoint})}catch{/* HTTP/business failures still have a raw response. */}
  if(response){
   const body=(await response.body()).toString('utf8');
   await save(dir,'response.txt',body);
   // Keep JSON byte-for-byte, even when ERP reports a business failure.
   try{JSON.parse(body);await save(dir,'response.json',body)}catch(e){if(!(e instanceof SyntaxError))throw e}
   result={status:response.status(),body,archive};
  }else result={state:sent?'unknown':'not_sent',error:sent?'请求已发出但未收到妙手响应，请在妙手核对后再操作。':requestSaved?'网页未完成请求，请重新打开妙手登录页面。':'请求未发出，请检查网页登录状态或请求存档。',archive};
 }catch{result={state:sent?'unknown':'not_sent',error:sent?'提交结果或响应存档未完成，请在妙手核对后再操作。':'提交准备失败，请检查浏览器服务。',archive}}
 finally{page.off('response',onResponse);await page.unroute(url,route).catch(()=>{})}
 try{await save(dir,'result.json',result)}catch{result.archiveError='提交结果文件保存失败'}
 return result;
}
