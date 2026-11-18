import {mkdir,open,rename,readFile} from 'node:fs/promises';
import path from 'node:path';
let sequence=0;
export const endpoints = new Set([
 '/api/order/package/render_list/getOpOrderPackageIdAndOutboundInfoMap',
 '/api/platform/tiktok/item/item/searchItemList',
 '/api/order/package/render_list/searchOrderPackageList',
 '/api/order/package/getTranslatedOpOrderPackageList',
 '/api/order/purchase/purchase_item_source/getList'
]);
// Exact response bytes are retained; credentials and arbitrary request headers are never archived.
export async function archiveResponse(root,accountId,{endpoint,status,contentType,requestBody,requestContext,body}){
 if(!/^(?:[a-f0-9]{32}|[1-9][0-9]*)$/.test(accountId)||!endpoints.has(endpoint))throw new Error('非法存档来源');
 const capturedAt=new Date().toISOString();
 const record={version:1,accountId,capturedAt,endpoint,status,contentType,requestBody,requestContext,bytes:body.length,bodyBase64:body.toString('base64')};
 const dir=path.join(root,'raw',accountId);await mkdir(dir,{recursive:true,mode:0o700});
 const filename=capturedAt.replaceAll(':','-')+'-'+(++sequence)+'.json';
 const temp=path.join(dir,filename+'.tmp');const handle=await open(temp,'wx',0o600);
 try{await handle.writeFile(JSON.stringify(record));await handle.sync()}finally{await handle.close()}
 await rename(temp,path.join(dir,filename));
 const directory=await open(dir,'r');try{await directory.sync()}finally{await directory.close()}
 return {record,filename};
}
export async function readArchive(file){const r=JSON.parse(await readFile(file,'utf8'));const bytes=Buffer.from(r.bodyBase64,'base64');return {record:r,json:JSON.parse(bytes.toString('utf8'))}}
