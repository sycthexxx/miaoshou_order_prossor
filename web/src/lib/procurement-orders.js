export const formatOrderTime = value => value ? value.slice(0, 19).replace('T', ' ') : '—';

export function filterProcurementByOrderDate(report, start, end, dateField = 'orderedAt') {
 if (!start && !end) return report;

 function matches(line) {
  // Use the calendar day from the order record, without converting it to browser time.
  const day = line[dateField]?.slice(0, 10);
  return day && (!start || day >= start) && (!end || day <= end);
 }

 const groups = [];
 const packages = new Set();
 let quantity = 0;
 let unresolvedQuantity = 0;
 for (const group of report.groups) {
  const variants = [];
  let groupQuantity = 0;
  for (const variant of group.variants) {
   const lines = variant.lines.filter(matches);
   if (!lines.length) continue;
   const variantQuantity = lines.reduce((sum, line) => sum + line.quantity, 0);
   variants.push({...variant, lines, quantity: variantQuantity});
   groupQuantity += variantQuantity;
   for (const line of lines) packages.add(String(line.packageId));
  }
  if (!variants.length) continue;
  groups.push({...group, variants, quantity: groupQuantity});
  quantity += groupQuantity;
  if (group.reason) unresolvedQuantity += groupQuantity;
 }

 return {
  ...report,
  groups,
  packages: packages.size,
  quantity,
  unresolvedQuantity,
  pendingRefundLines: report.pendingRefundLines?.filter(matches),
 };
}

// Display groups only: leave the SKU lines used by procurement and packing intact.
export function procurementOrderRows(group) {
 const orders=new Map();
 for(const variant of group.variants||[])for(const line of variant.lines||[]){
  const key=JSON.stringify([line.shopId,line.orderNo||line.packageId]);
  if(!orders.has(key))orders.set(key,{key,orderNo:line.orderNo,shopId:line.shopId,packages:[],specs:new Map()});
  const order=orders.get(key);
  let pkg=order.packages.find(p=>p.packageId===line.packageId);
  if(!pkg){pkg={...line,lines:[],notes:[]};order.packages.push(pkg)}
  pkg.lines.push({...line,spec:variant.spec});
  if(line.localNote&&!pkg.notes.includes(line.localNote))pkg.notes.push(line.localNote);
  order.specs.set(variant.spec,(order.specs.get(variant.spec)||0)+line.quantity);
 }
 const rows=new Map();
 for(const order of orders.values()){
  const specs=[...order.specs.keys()].sort(),key=JSON.stringify(specs);
  if(!rows.has(key)){
   const lines=order.packages.flatMap(pkg=>pkg.lines);
   const representatives=specs.map(spec=>{
    const skus=new Map();
    for(const line of lines){
     if(line.spec===spec&&!skus.has(line.skuId))skus.set(line.skuId,line);
    }
    return {spec,quantity:0,skus:[...skus.values()]};
   });
   rows.set(key,{key,specs:representatives,quantity:0,orders:[]});
  }
  const row=rows.get(key);row.orders.push(order);
  for(const spec of row.specs){const quantity=order.specs.get(spec.spec);spec.quantity+=quantity;row.quantity+=quantity}
 }
 return [...rows.values()].sort((a,b)=>a.key.localeCompare(b.key));
}
