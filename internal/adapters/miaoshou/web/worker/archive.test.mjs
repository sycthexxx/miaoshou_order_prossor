import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,readFile,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {archiveResponse,readArchive} from './archive.mjs';
test('exact raw bytes persist and replay without checksum metadata',async()=>{
 const root=await mkdtemp(path.join(tmpdir(),'order-archive-'));
 try{const body=Buffer.from('{"result":"success", "futureField":"酒红色,M"}\n');const id='a'.repeat(32);
 const {filename}=await archiveResponse(root,id,{endpoint:'/api/order/package/render_list/searchOrderPackageList',status:200,contentType:'application/json',requestBody:'page=1',body});
 const file=path.join(root,'raw',id,filename);assert.equal((await readArchive(file)).json.futureField,'酒红色,M');
 const raw=JSON.parse(await readFile(file));assert.deepEqual(Buffer.from(raw.bodyBase64,'base64'),body);assert.equal(raw.sha256,undefined);raw.bodyBase64=Buffer.from('invalid JSON').toString('base64');await writeFile(file,JSON.stringify(raw));await assert.rejects(readArchive(file),SyntaxError);
 await assert.rejects(archiveResponse(root,'../escape',{endpoint:'/',body}),/非法/);
 }finally{await rm(root,{recursive:true,force:true})}
});
