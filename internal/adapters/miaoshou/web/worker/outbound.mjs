const endpoint='/api/order/package/render_list/getOpOrderPackageIdAndOutboundInfoMap';
// Observe native UI requests only; keep the page alive until all returned
// packages have their independent outbound response available for archiving.
export function observeOutbound(page,timeout=45000){
 const ids=new Set();let wanted=null,resolve,reject,timer,stopped=false;
 const done=new Promise((yes,no)=>{resolve=yes;reject=no});done.catch(()=>{});
 const check=()=>{if(wanted&&wanted.every(id=>ids.has(id))){clearTimeout(timer);resolve()}};
 const listener=async response=>{try{
  if(new URL(response.url()).pathname!==endpoint)return;
  await response.finished();const d=await response.json();
  if(stopped)return;
  if(response.status()!==200||d.result!=='success'||!d.opOrderPackageIdAndOutboundInfoMap)throw new Error('订单出货信息读取失败');
  for(const id of Object.keys(d.opOrderPackageIdAndOutboundInfoMap))ids.add(id);check();
 }catch(e){if(!stopped){clearTimeout(timer);reject(e)}}};
 page.on('response',listener);
 return {wait(packageIDs){wanted=packageIDs.map(String);timer=setTimeout(()=>reject(new Error('等待订单出货信息超时，已保留收到的原始响应；请重新同步补齐卖家信息')),timeout);check();return done},stop(){stopped=true;clearTimeout(timer);page.off('response',listener)}};
}
