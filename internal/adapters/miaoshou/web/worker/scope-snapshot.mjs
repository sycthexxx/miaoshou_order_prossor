import {mkdir,open,rename} from 'node:fs/promises';
import path from 'node:path';
let sequence=0;

// An exact, complete pass is evidence; a union of changing passes is not.
export function completeScope(scope,pages){
 const first=pages.get(1);if(!first)return null;
 const total=Number(first.total),size=Number(first.pageSize),count=Math.max(1,Math.ceil(total/size));
 if(!Number.isSafeInteger(total)||total<0||!Number.isSafeInteger(size)||size<1||pages.size!==count)return null;
 const ids=new Set(),archives=[];
 for(let page=1;page<=count;page++){
  const d=pages.get(page);
  if(!d||!d.archiveFile||Number(d.page)!==page||Number(d.total)!==total||Number(d.pageSize)!==size)return null;
  if(d.packageList.length!==Math.min(size,total-(page-1)*size))return null;
  archives.push(d.archiveFile);
  for(const p of d.packageList){const id=String(p.opOrderPackageId||'');if(!id||ids.has(id))return null;ids.add(id)}
 }
 return ids.size===total?{scope,total,archives,packageIds:[...ids]}:null;
}

export async function writeScopeSnapshot(root,accountId,snapshot){
 const dir=path.join(root,'snapshots',accountId);await mkdir(dir,{recursive:true,mode:0o700});
 const capturedAt=new Date().toISOString();
 const file=path.join(dir,capturedAt.replaceAll(':','-')+'-'+(++sequence)+'.json');
 const handle=await open(file+'.tmp','wx',0o600);
 try{await handle.writeFile(JSON.stringify({version:1,accountId,capturedAt,...snapshot}));await handle.sync()}finally{await handle.close()}
 await rename(file+'.tmp',file);
 const directory=await open(dir,'r');try{await directory.sync()}finally{await directory.close()}
}
