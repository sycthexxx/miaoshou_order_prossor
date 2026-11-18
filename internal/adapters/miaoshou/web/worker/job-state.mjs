import {mkdir,readFile,writeFile,rename} from 'node:fs/promises';
import path from 'node:path';
export async function readJob(root,id){
 try{const job=JSON.parse(await readFile(path.join(root,'runtime',id,'sync.json'),'utf8'));
  if(job.state==='running'){job.state='failed';job.error='浏览器进程已重启，本次同步中断；已保存的数据保留，请重新同步。';job.finishedAt=new Date().toISOString()}
  return job;
 }catch(e){if(e.code==='ENOENT')return null;throw e}
}
export function jobWriter(root,id){
 let pending=Promise.resolve(),last='';
 return job=>{const body=JSON.stringify(job);if(!body||body===last)return pending;last=body;
  pending=pending.catch(()=>{}).then(async()=>{const dir=path.join(root,'runtime',id);await mkdir(dir,{recursive:true,mode:0o700});const file=path.join(dir,'sync.json');await writeFile(file+'.tmp',body,{mode:0o600});await rename(file+'.tmp',file)});
  return pending;
 };
}
