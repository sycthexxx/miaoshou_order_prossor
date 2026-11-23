(()=>{
 const offerId=url=>{try{const u=new URL(url);return u.protocol==='https:'&&u.hostname==='detail.1688.com'?u.pathname.match(/^\/offer\/(\d+)\.html$/)?.[1]||'':''}catch{return ''}};
 const normalize=s=>String(s??'').normalize('NFKC').toLowerCase().replace(/\s+/g,'').trim();
 const tokens=spec=>[...new Set(String(spec).split(/[,，;；|/]+/).map(normalize).filter(Boolean))];
 function snapshot(p,allowEmpty=false){
  if(p?.version!==1||!offerId(p.url)||offerId(p.url)!==p.offerId||!Array.isArray(p.variants)||(!allowEmpty&&!p.variants.length)||p.variants.length>1000)throw Error('采购清单格式无效');
  const variants=new Map(),images=new Map();
  for(const v of p.variants){if(typeof v.spec!=='string'||!v.spec.trim()||v.spec.length>300||!Number.isSafeInteger(v.quantity)||v.quantity<=0)throw Error('采购规格或数量无效');variants.set(v.spec,(variants.get(v.spec)||0)+v.quantity);images.set(v.spec,[...new Set([...(images.get(v.spec)||[]),...(Array.isArray(v.images)?v.images:[])])].filter(u=>{try{const url=new URL(u);return ['http:','https:'].includes(url.protocol)&&!url.username&&!url.password}catch{return false}}).slice(0,6))}
  if(!Number.isFinite((typeof p.createdAt==='number'?p.createdAt:Date.parse(p.createdAt))))throw Error('缺少采购汇总时间，请刷新工作台');
  return {version:1,offerId:p.offerId,url:`https://detail.1688.com/offer/${p.offerId}.html`,account:String(p.account||'').slice(0,80),title:String(p.title||'待采购商品').slice(0,500),createdAt:p.createdAt,sourceStatus:{state:['valid','invalid'].includes(p.sourceStatus?.state)?p.sourceStatus.state:'unknown',reason:typeof p.sourceStatus?.reason==='string'?p.sourceStatus.reason.slice(0,500):'',checkedAt:typeof p.sourceStatus?.checkedAt==='string'?p.sourceStatus.checkedAt.slice(0,60):'',source:['manual','plugin'].includes(p.sourceStatus?.source)?p.sourceStatus.source:''},variants:[...variants].map(([spec,quantity])=>({spec,quantity,...(images.get(spec)?.length?{images:images.get(spec)}:{})}))};
 }
 function quantity(value){const s=String(value??'').trim();if(!/^\d+$/.test(s))return null;const n=Number(s);return Number.isSafeInteger(n)?n:null}
 function comparison(required,selected){if(selected===null)return {kind:'unknown',text:'待核对'};if(selected===required)return {kind:'match',text:'数量一致'};return selected>required?{kind:'over',text:`多选 ${selected-required} 件`}:{kind:'short',text:`还差 ${required-selected} 件`}}
 // Per-document ledger: retain hidden colors, but replace visible readings,
 // invalidate changed mappings and never reuse a ledger for another offer.
 function reconcile(variants, readings, ledger, active, bindings={}, valid=true){
  if(!valid){ledger.clear();return new Map(variants.map(v=>[v.spec,{quantity:null,cached:false}]))}
  const keys=new Map(),out=new Map();
  for(const v of variants){const key=tokens(bindings[v.spec]?.alias||v.spec).sort().join('|');keys.set(key,(keys.get(key)||0)+1)}
  for(const v of variants){
   const parts=tokens(bindings[v.spec]?.alias||v.spec),key=parts.slice().sort().join('|');
   let old=ledger.get(v.spec);if(old?.key!==key){ledger.delete(v.spec);old=null}
   const n=readings.get(v.spec)??null;
   if(keys.get(key)>1){ledger.delete(v.spec);out.set(v.spec,{quantity:null,cached:false});continue}
   if(n!==null){ledger.set(v.spec,{key,quantity:n});out.set(v.spec,{quantity:n,cached:false});continue}
   // A failed read of the currently selected color is unknown, not old success.
   if(parts.length<2||active.has(parts[0])){ledger.delete(v.spec);old=null}
   out.set(v.spec,{quantity:old?.quantity??null,cached:!!old});
  }
  return out;
 }
 function quantityTracker(){
  let previous=new Map(),previousContext=null;
  return (readings,context)=>{
   const changed=[];
   if(context===previousContext)for(const [spec,n] of readings){const before=previous.get(spec);if(n!==null&&before!==null&&before!==undefined&&n!==before)changed.push(spec)}
   previous=new Map(readings);previousContext=context;return changed;
  };
 }
 globalThis.OPProcurement={offerId,normalize,tokens,snapshot,quantity,comparison,reconcile,quantityTracker};
})();
