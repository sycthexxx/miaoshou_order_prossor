import {nativeClient} from './native-request.mjs';

export async function lookupLogisticsCompany({session,payload}){
 const page=session.context.pages().find(page=>page.url().startsWith('https://erp.91miaoshou.com/order/'));
 if(!page)return {error:'请先打开妙手订单页'};
 const client=await nativeClient(page);
 const company=await page.evaluate(async({client,number})=>{
  const module=await import(client.url);
  const result=await module[client.name]({url:'/order/forwarder/getLogisticsCompanyByLogisticsNo',method:'GET',params:{logisticsNo:number},timeout:10000});
  return typeof result.logisticsCompany==='string'?result.logisticsCompany.trim():'';
 },{client,number:payload.number});
 return {status:200,body:JSON.stringify({company})};
}
