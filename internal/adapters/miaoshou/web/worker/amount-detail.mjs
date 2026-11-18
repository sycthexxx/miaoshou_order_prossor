import {nativeClient} from './native-request.mjs';

// Read-only requests; amount responses are intentionally not archived or stored.
export async function getAmountDetail({session,payload,resolveClient=nativeClient}){
 const page=session.context.pages().find(p=>p.url().startsWith('https://erp.91miaoshou.com/order/'));
 if(!page)return {error:'请先在账户管理打开妙手订单页面'};
 const client=await resolveClient(page);
 const data=await page.evaluate(async({client,payload})=>{
  const module=await import(client.url),request=module[client.name];
  const get=(url,params)=>request({url,method:'GET',params,timeout:30000});
  const [amountResult,orderResult,ratesResult,shippingResult]=await Promise.all([
   get('/order/package/getOrderPackageAmountDetail',{opOrderPackageId:payload.packageId}),
   request({url:'/order/purchase/getOrderDetailInfo',method:'POST',data:{opOrderId:payload.orderId},timeout:30000}),
   get('/common/exchange_rate/getExchangeRateList',{}),
   get('/order/package/getEstimatedShippingFeeInfoMapByOpOrderPackageIds',{opOrderPackageIds:[payload.packageId]})
  ]);
  const amount=amountResult.orderPackageAmountDetail,order=orderResult.orderDetailInfo;
  if(!amount||!order||String(order.opOrderId)!==payload.orderId||amount.platformOrderSn!==order.platformOrderSn)throw new Error('订单金额信息不匹配');
  const rate=ratesResult.exchangeRateList?.find(r=>r.currency===order.currency);
  return {amount,order:{orderNo:order.platformOrderSn,shopName:order.shopName||order.platformShopName,platform:order.platform,site:order.site,isCb:order.isCb,currency:order.currency,settled:!!order.gmtSettlement},rate,estimatedShipping:shippingResult.opOrderPackageIdAndEstimatedShippingFeeInfoMap?.[payload.packageId]||null};
 },{client,payload});
 return {status:200,body:JSON.stringify(data)};
}
