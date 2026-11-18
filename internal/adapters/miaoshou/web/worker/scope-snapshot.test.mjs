import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,readFile,readdir,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {completeScope,writeScopeSnapshot} from './scope-snapshot.mjs';
const page=(n,ids,total=3)=>({page:n,pageSize:2,total,archiveFile:`raw-${n}.json`,packageList:ids.map(opOrderPackageId=>({opOrderPackageId}))});
test('authority requires every raw page and exact unique membership, not partial or changing pages',()=>{
 const valid=new Map([[1,page(1,['a','b'])],[2,page(2,['c'])]]);
 assert.equal(completeScope('waitPurchase',valid).total,3);
 assert.equal(completeScope('waitPurchase',new Map([[1,page(1,['a','b'])]])),null);
 for(const bad of [page(2,['a']),page(2,[],2),{...page(2,['c']),archiveFile:null}])
  assert.equal(completeScope('waitPurchase',new Map([[1,page(1,['a','b'])],[2,bad]])),null);
});
test('completed manifest is independently durable and leaves raw archives untouched',async()=>{
 const root=await mkdtemp(path.join(tmpdir(),'scope-evidence-'));
 try{
  await writeScopeSnapshot(root,'1',{scope:'waitPurchase',total:0,archives:['empty.json'],packageIds:[]});
  const dir=path.join(root,'snapshots','1'),files=await readdir(dir);assert.equal(files.length,1);assert.ok(files[0].endsWith('.json'));
  const d=JSON.parse(await readFile(path.join(dir,files[0])));assert.equal(d.accountId,'1');assert.equal(d.version,1);assert.equal(d.total,0);assert.ok(d.capturedAt);
  assert.deepEqual(await readdir(root),['snapshots']);
 }finally{await rm(root,{recursive:true,force:true})}
});
