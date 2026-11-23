import {nativeClient} from './native-request.mjs';

export async function getTracking({session,payload,resolveClient=nativeClient}) {
  const page=session.context.pages().find(p=>p.url().startsWith('https://erp.91miaoshou.com/order/'));
  if(!page)return {error:'请先在账户管理打开妙手订单页面'};
  const client=await resolveClient(page);
  const data=await page.evaluate(async({client,payload})=>{
    const mod=await import(client.url);
    const domestic=payload.kind==='domestic';
    const url=domestic?'/order/purchase/purchase_order/getTraceInfo':'/order/purchase/getLogisticsTracking';
    const params=domestic
      ? {purchaseOrderFilterId:payload.traceId,waybillCode:payload.number,logisticsName:payload.company}
      : {opOrderPackageId:payload.packageId};
    const result=await mod[client.name]({url,method:'GET',params,timeout:25000,__tips:false});
    if(!result || result.result && result.result!=='success')throw new Error('妙手物流轨迹查询失败');
    const text=value=>typeof value==='string'?value:'';
    const rows=domestic?result.data?.logisticsSteps:result.trackingInfo;
    const steps=(Array.isArray(rows)?rows:[]).map(row=>domestic
      ? {time:text(row.acceptTime),description:text(row.remark),detail:text(row.stateExText)}
      : {time:text(row.gmtCreate),description:text(row.descriptionCn)||text(row.description),
        detail:row.descriptionCn&&row.description!==row.descriptionCn?text(row.description):'',place:text(row.tracePlace)})
      .filter(row=>row.description||row.detail);
    if(domestic)steps.sort((a,b)=>(Date.parse(b.time)||0)-(Date.parse(a.time)||0));
    return {number:(!domestic&&text(result.data?.newestTrackingNo))||payload.number,steps};
  },{client,payload});
  return {status:200,body:JSON.stringify(data)};
}
