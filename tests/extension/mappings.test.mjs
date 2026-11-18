import {test} from 'node:test';
import assert from 'node:assert/strict';
import '../../extensions/procurement-helper/core.js';
import '../../extensions/procurement-helper/mappings.js';
const {empty,migrate,apply,reset,aliasFor,effective}=OPMappings;
const variants=['浅咖啡,L','浅咖啡,M','浅咖啡,S','酒红色,L'].map(spec=>({spec,quantity:1}));
test('one color edit covers all sizes, reload and newly added variants',()=>{
 const state=empty();apply(state,'浅咖啡,L','浅咖,L');
 assert.deepEqual(variants.map(v=>aliasFor(state,v.spec)),['浅咖,L','浅咖,M','浅咖,S','酒红色,L']);
 const reloaded=migrate(JSON.parse(JSON.stringify(state)));assert.equal(aliasFor(reloaded,'浅咖啡,XS'),'浅咖,XS');
 assert.equal(aliasFor(empty(),'浅咖啡,L'),'浅咖啡,L');
});
test('manual full specification changes color globally but special size only locally',()=>{
 const state=empty();apply(state,'浅咖啡,L','浅咖,1XL');
 assert.equal(aliasFor(state,'浅咖啡,L'),'浅咖,1XL');assert.equal(aliasFor(state,'浅咖啡,M'),'浅咖,M');
 apply(state,'浅咖啡,M','浅咖色,2XL');
 assert.equal(aliasFor(state,'浅咖啡,L'),'浅咖色,1XL');assert.equal(aliasFor(state,'浅咖啡,M'),'浅咖色,2XL');assert.equal(aliasFor(state,'浅咖啡,S'),'浅咖色,S');
});
test('explicit SKU exceptions are visible exceptions; reset scopes are distinct',()=>{
 const state=empty();apply(state,'浅咖啡,L','浅咖,L');apply(state,'浅咖啡,S','米色,小码',true);
 apply(state,'浅咖啡,M','浅咖色,2XL');
 assert.equal(aliasFor(state,'浅咖啡,S'),'米色,小码');
 reset(state,'浅咖啡,M');assert.equal(aliasFor(state,'浅咖啡,M'),'浅咖色,M');
 reset(state,'浅咖啡,S');assert.equal(aliasFor(state,'浅咖啡,S'),'浅咖色,S');
 apply(state,'酒红色,L','深红色,1XL');reset(state,'浅咖啡,L',true);
 assert.equal(aliasFor(state,'浅咖啡,M'),'浅咖啡,M');assert.equal(aliasFor(state,'酒红色,L'),'深红色,1XL');
});
test('legacy aliases migrate shared color, retain size, and never share DOM selectors',()=>{
 const old={'浅咖啡,L':{alias:'浅咖,1XL',selector:'#local-dom'},'浅咖啡,M':{alias:'浅咖,M'}};
 const state=migrate(old);assert.equal(aliasFor(state,'浅咖啡,S'),'浅咖,S');assert.equal(aliasFor(state,'浅咖啡,L'),'浅咖,1XL');
 assert.ok(!JSON.stringify(state).includes('selector'));assert.equal(old['浅咖啡,L'].selector,'#local-dom');
});
test('conflicting legacy aliases are retained until new shared rule overrides all legacy colors',()=>{
 const state=migrate({'浅咖啡,L':{alias:'浅咖色,1XL'},'浅咖啡,M':{alias:'咖啡色,M'}});
 assert.equal(aliasFor(state,'浅咖啡,L'),'浅咖色,1XL');assert.equal(aliasFor(state,'浅咖啡,S'),'浅咖啡,S');
 apply(state,'浅咖啡,M','浅咖,M');
 assert.equal(aliasFor(state,'浅咖啡,L'),'浅咖,1XL');assert.equal(aliasFor(state,'浅咖啡,S'),'浅咖,S');
});
test('DOM bindings are page local and ignored whenever effective alias changed',()=>{
 const state=empty(),selectors={'浅咖啡,L':{alias:'浅咖啡,L',selector:'#old'}};
 assert.equal(effective(state,variants,selectors)['浅咖啡,L'].selector,'#old');apply(state,'浅咖啡,L','浅咖,L');
 assert.equal(effective(state,variants,selectors)['浅咖啡,L'].selector,undefined);
 const cleaned=migrate({...state,skus:{'浅咖啡,L':{exception:'米色,L',selector:'#do-not-save'}}});
 assert.deepEqual(cleaned.skus['浅咖啡,L'],{exception:'米色,L'});
});
