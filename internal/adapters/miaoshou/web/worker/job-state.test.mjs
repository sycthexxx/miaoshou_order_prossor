import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,rm} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {jobWriter,readJob} from './job-state.mjs';
import {requestContext} from './request-context.mjs';

test('recover interrupted jobs; persist latest update in order; isolate accounts',async()=>{
 const root=await mkdtemp(path.join(os.tmpdir(),'order-job-'));
 try{
  const write=jobWriter(root,'1');await write({state:'running',page:4,collected:600});
  const job=await readJob(root,'1');assert.equal(job.state,'failed');assert.equal(job.collected,600);
  assert.equal(await readJob(root,'2'),null);
  await Promise.all([write({state:'running',page:5}),write({state:'completed',page:6})]);
  assert.deepEqual(await readJob(root,'1'),{state:'completed',page:6});
 }finally{await rm(root,{recursive:true,force:true})}
});
test('archive header whitelist excludes credentials and strips URL secrets',()=>{
 const result=requestContext({method:()=> 'POST',headers:()=>({cookie:'secret-cookie',authorization:'secret-auth','x-secret-token':'secret-custom','user-agent':'Chrome','sec-ch-ua-platform':'Linux',origin:'https://erp.91miaoshou.com',referer:'https://erp.91miaoshou.com/order?token=secret-query#secret-fragment'})});
 const saved=JSON.stringify(result);for(const secret of ['secret-cookie','secret-auth','secret-custom','secret-query','secret-fragment'])assert.ok(!saved.includes(secret));
 assert.equal(result.headers.referer,'https://erp.91miaoshou.com/order');assert.equal(result.headers['user-agent'],'Chrome');
});
