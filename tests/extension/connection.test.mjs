import {test} from 'node:test';
import assert from 'node:assert/strict';
import {workbenchOrigin,hostPattern,suggestedOrigin,registerWorkbench} from '../../extensions/procurement-helper/connection.js';
test('use actual active LAN workbench instead of employee localhost',()=>{
 assert.equal(suggestedOrigin('http://192.168.1.104:17381/#/procurement','http://localhost:17381'),'http://192.168.1.104:17381');
 assert.equal(suggestedOrigin('https://detail.1688.com/offer/100.html','http://192.168.1.104:17381'),'http://192.168.1.104:17381');
 assert.equal(suggestedOrigin('chrome://newtab'), '');
 assert.equal(hostPattern(workbenchOrigin('http://192.168.1.104:17381')),'http://192.168.1.104/*');
 assert.throws(()=>workbenchOrigin('http://192.168.1.104:17381/secret'));assert.throws(()=>workbenchOrigin('http://user:pass@192.168.1.104'));
});
test('dynamic registration supports LAN and https localhost; static http localhost does not duplicate',async()=>{
 const writes=[];const mock={scripting:{getRegisteredContentScripts:async()=>[{id:'workbench'}],unregisterContentScripts:async x=>writes.push(['remove',x]),registerContentScripts:async x=>writes.push(['add',x])}};
 await registerWorkbench(mock,'http://192.168.1.104:17381');assert.equal(writes[1][1][0].matches[0],'http://192.168.1.104/*');
 writes.length=0;await registerWorkbench(mock,'https://localhost:17381');assert.equal(writes[1][1][0].matches[0],'https://localhost/*');
 writes.length=0;await registerWorkbench(mock,'http://localhost:17381');assert.equal(writes.length,1);
});
