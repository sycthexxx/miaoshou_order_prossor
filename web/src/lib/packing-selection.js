export const lineKey=line=>JSON.stringify([String(line.packageId),String(line.itemId)]);
export function splitSpec(spec){
 const parts=String(spec||'').split(/[,，]/).map(s=>s.trim()).filter(Boolean);
 return parts.length>1?{color:parts.slice(0,-1).join(','),size:parts.at(-1)}:{color:'',size:''};
}
export function supplierPackingGroup(supplier,groups){
 return {key:`supplier:${supplier.id}`,supplier:{id:supplier.id,name:supplier.name},sources:groups.filter(g=>g.supplier?.id===supplier.id&&!g.reason)};
}
export function sourceLines(group){
 const lines=new Map();
 for(const source of group.sources||[group])for(const variant of source.variants||[])for(const line of variant.lines||[]){
  const key=lineKey(line);if(!line.packageId||!line.itemId)continue;
  lines.set(key,{...line,key,sourceKey:source.key,title:source.title,image:source.image,sourceItemId:source.source?.sourceItemId,spec:variant.spec,...splitSpec(variant.spec)});
 }
 return [...lines.values()];
}
export function filteredLines(lines,{sources=[],colors=[],sizes=[],query=''}={}){
 const q=query.trim().toLowerCase();return lines.filter(l=>(!sources.length||sources.includes(l.sourceKey))&&(!colors.length||colors.includes(l.color))&&(!sizes.length||sizes.includes(l.size))&&(!q||[l.title,l.sourceItemId,l.spec,l.originalSpec,l.skuId,l.packageNo,l.orderNo,l.shopId].join(' ').toLowerCase().includes(q)));
}
export function groupLinesBySpec(lines){
 const groups=new Map();for(const line of lines){const key=JSON.stringify([line.sourceKey,line.spec]);if(!groups.has(key))groups.set(key,{key,sourceKey:line.sourceKey,title:line.title,image:line.image,sourceItemId:line.sourceItemId,spec:line.spec,lines:[],quantity:0});const g=groups.get(key);g.lines.push(line);g.quantity+=Number(line.quantity)}return [...groups.values()];
}
export function selectionStats(lines,selected){
 const chosen=lines.filter(l=>selected.has(l.key));return {lines:chosen.length,packages:new Set(chosen.map(l=>String(l.packageId))).size,quantity:chosen.reduce((sum,l)=>sum+Number(l.quantity),0)};
}

// Count each facet against the other filters, so multiple choices remain available.
export function filterOptions(lines,filters,field){
 const matching=filteredLines(lines,{...filters,[field==='color'?'colors':'sizes']:[]});
 const options=new Map();
 for(const line of matching){
  const value=line[field];if(!value)continue;
  if(!options.has(value))options.set(value,{value,packages:new Set(),quantity:0});
  const option=options.get(value);option.packages.add(String(line.packageId));option.quantity+=Number(line.quantity);
 }
 return [...options.values()].filter(o=>o.quantity>0).sort((a,b)=>a.value.localeCompare(b.value,'zh-CN')).map(o=>({...o,packages:o.packages.size}));
}
