import {test} from 'node:test';
import assert from 'node:assert/strict';
import {checkLock} from '../../scripts/prepare-lock.mjs';
const manifest={dependencies:{playwright:'1.63.0'}};
const docker='FROM mcr.microsoft.com/playwright:v1.63.0-noble';
const pkg=(version,extra={})=>({version,resolved:'https://registry.npmjs.org/fixture.tgz',integrity:'sha512-fixture',...extra});
function lock(){return {lockfileVersion:3,packages:{'':{},'node_modules/playwright':pkg('1.63.0'),'node_modules/playwright-core':pkg('1.63.0')}}}
test('missing and host-only recovered locks must be repaired before Docker build',()=>{
 assert.ok(checkLock(manifest,undefined,docker).length);
 const l=lock();l.packages['node_modules/playwright']={version:'1.63.0'};assert.match(checkLock(manifest,l,docker).join(),/校验信息/);
});
test('rejects mismatched browser binary and missing cross-platform optional packages',()=>{
 assert.deepEqual(checkLock(manifest,lock(),docker),[]);
 assert.match(checkLock(manifest,lock(),docker.replace('1.63.0','1.62.0')).join(),/镜像版本/);
 const l=lock();l.packages['node_modules/native-parent']=pkg('1.0.0',{optionalDependencies:{'native-linux':'1.0.0'}});
 assert.match(checkLock(manifest,l,docker).join(),/跨平台/);
 l.packages['node_modules/native-linux']=pkg('1.0.0');assert.deepEqual(checkLock(manifest,l,docker),[]);
});
