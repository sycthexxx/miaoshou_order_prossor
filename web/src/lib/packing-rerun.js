// Group shared information for editing while retaining exact saved item identities.
export function rerunGroups(job){
 const groups=new Map();
 for(const item of job.items)for(const submitted of item.request.items){
  const {orderItemId,...info}=submitted,key=JSON.stringify(info);
  if(!groups.has(key))groups.set(key,{info:JSON.parse(JSON.stringify(info)),mode:info.purchase?'purchase':'waybill',lines:[]});
  const line=item.lines.find(l=>l.itemId===orderItemId);
  groups.get(key).lines.push({...line,packageId:item.request.packageId,itemId:orderItemId});
 }
 return [...groups.values()];
}
export function rerunRequests(groups){
 const requests=new Map();
 for(const group of groups)for(const line of group.lines){
  if(!requests.has(line.packageId))requests.set(line.packageId,{packageId:line.packageId,items:[]});
  const info=JSON.parse(JSON.stringify(group.info));
  if(group.mode==='purchase')delete info.waybills;else{delete info.purchase;info.autoSyncAmount=false}
  requests.get(line.packageId).items.push({...info,orderItemId:line.itemId});
 }
 return [...requests.values()];
}
