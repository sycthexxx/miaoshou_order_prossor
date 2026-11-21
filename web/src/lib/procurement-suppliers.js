export function procurementSupplierGroups(groups){
 const normal=new Map(),invalid={id:'invalid',name:'货源无效',invalid:true,quantity:0,products:[]};
 for(const g of groups){
  if(g.sourceStatus?.state==='invalid'){invalid.quantity+=g.quantity;invalid.products.push(g);continue}
  const id=g.supplier?.id||0;if(!normal.has(id))normal.set(id,{id,name:g.supplier?.name||'未指定厂家',quantity:0,products:[]});
  const seller=normal.get(id);seller.quantity+=g.quantity;seller.products.push(g);
 }
 const result=[...normal.values()];for(const seller of [...result,invalid])seller.products.sort((a,b)=>b.quantity-a.quantity||a.key.localeCompare(b.key));
 result.sort((a,b)=>b.quantity-a.quantity||a.name.localeCompare(b.name)||a.id-b.id);
 return invalid.products.length?[invalid,...result]:result;
}
export function sourceStatusOfferId(group){
 try{const u=new URL(group.url);if(['http:','https:'].includes(u.protocol)&&u.hostname==='detail.1688.com'){const id=u.pathname.match(/^\/offer\/(\d+)\.html$/)?.[1];if(id)return id}}catch{}
 const id=String(group.source?.sourceItemId||'');return group.source?.source==='1688'&&/^\d+$/.test(id)?id:'';
}
