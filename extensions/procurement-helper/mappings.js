(()=>{
 const {normalize}=OPProcurement;
 const parts=value=>String(value||'').split(/[,，;；|/]+/).map(s=>s.trim()).filter(Boolean);
 const colorKey=spec=>{const p=parts(spec);return p.length>=2?normalize(p[0]):''};
 const empty=()=>({version:2,colors:{},skus:{}});
 const equivalent=(a,b)=>parts(a).map(normalize).join('|')===parts(b).map(normalize).join('|');
 function migrate(raw){
  if(raw?.version===2){
   const clean=empty();
   for(const [key,value] of Object.entries(raw.colors||{}))if(typeof value==='string')clean.colors[key]=value;
   for(const [spec,entry] of Object.entries(raw.skus||{})){
    const item={};if(Array.isArray(entry?.tail))item.tail=entry.tail.slice();
    for(const key of ['exception','legacyAlias'])if(typeof entry?.[key]==='string')item[key]=entry[key];
    if(Object.keys(item).length)clean.skus[spec]=item;
   }
   return clean;
  }
  const state=empty(),candidates=new Map();
  for(const [spec,binding] of Object.entries(raw||{})){
   if(!binding||typeof binding!=='object')continue;
   const source=parts(spec),target=parts(binding.alias||spec),key=colorKey(spec);
   if(!key||target.length<2){if(binding.alias&&binding.alias!==spec)state.skus[spec]={legacyAlias:binding.alias};continue}
   if(!candidates.has(key))candidates.set(key,new Map());candidates.get(key).set(normalize(target[0]),target[0]);
   const item={};if(!equivalent(source.slice(1).join(','),target.slice(1).join(',')))item.tail=target.slice(1);
   if(binding.alias)item.legacyAlias=binding.alias;
   if(Object.keys(item).length)state.skus[spec]=item;
  }
  for(const [key,values] of candidates){
   if(values.size!==1)continue;
   state.colors[key]=[...values.values()][0];
   for(const spec of Object.keys(state.skus))if(colorKey(spec)===key){delete state.skus[spec].legacyAlias;if(!Object.keys(state.skus[spec]).length)delete state.skus[spec]}
  }
  // DOM selectors are deliberately not migrated into shared business data.
  return state;
 }
 function aliasFor(state,spec){
  const sku=state.skus[spec]||{},source=parts(spec),key=colorKey(spec);
  if(sku.exception)return sku.exception;
  if(key&&state.colors[key])return [state.colors[key],...(sku.tail||source.slice(1))].join(',');
  if(sku.legacyAlias)return sku.legacyAlias;
  return key&&sku.tail?[source[0],...sku.tail].join(','):spec;
 }
 function effective(state,variants,selectors={}){
  return Object.fromEntries(variants.map(v=>{
   const alias=aliasFor(state,v.spec),binding=selectors[v.spec];
   return [v.spec,{alias,...(binding?.selector&&equivalent(binding.alias,alias)?{selector:binding.selector}:{})}];
  }));
 }
 function apply(state,spec,alias,onlyThis=false){
  alias=String(alias||'').trim();if(!alias||alias.length>300)throw Error('请输入有效的1688规格（最多300字）');
  const key=colorKey(spec),source=parts(spec),target=parts(alias);
  if(onlyThis||!key||target.length<2){state.skus[spec]={exception:alias};return {scope:'sku',spec}}
  state.colors[key]=target[0];
  // Old whole-SKU aliases must never silently override a new shared color.
  for(const name of Object.keys(state.skus))if(colorKey(name)===key)delete state.skus[name].legacyAlias;
  const item={};if(!equivalent(source.slice(1).join(','),target.slice(1).join(',')))item.tail=target.slice(1);
  if(Object.keys(item).length)state.skus[spec]=item;else delete state.skus[spec];
  return {scope:'color',color:key,spec};
 }
 function reset(state,spec,allColor=false){
  const key=colorKey(spec);
  if(allColor&&key){delete state.colors[key];for(const name of Object.keys(state.skus))if(colorKey(name)===key)delete state.skus[name];return {scope:'color',color:key,spec}}
  delete state.skus[spec];return {scope:'sku',spec};
 }
 globalThis.OPMappings={parts,colorKey,empty,migrate,aliasFor,effective,apply,reset,equivalent};
})();
