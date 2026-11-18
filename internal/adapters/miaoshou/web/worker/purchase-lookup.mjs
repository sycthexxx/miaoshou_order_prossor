import {mkdir,writeFile} from 'node:fs/promises';
import path from 'node:path';

export const purchaseLookupEndpoint='/api/order/purchase_order/checkResellerPurchaseOrderStatus';

// The same read-only lookup used by the Miaoshou extension, using this
// account's ERP session. It does not associate or submit a purchase order.
export async function lookupPurchase({root,id,session,payload}){
 const page=session.context.pages().find(p=>p.url().startsWith('https://erp.91miaoshou.com/order/'));
 if(!page)return {error:'请先在账户管理打开妙手订单页面'};
 const params={purchasePlatform:'ali1688',purchaseOrderSn:payload.orderNumber,wsyOrderDetailId:'',purchaseShopId:''};
 const url=purchaseLookupEndpoint+'?'+new URLSearchParams(params);
 const dir=path.join(root,'runtime',id,'purchase-lookup',new Date().toISOString().replace(/[:.]/g,'-'));
 await mkdir(dir,{recursive:true});
 await writeFile(path.join(dir,'request.json'),JSON.stringify({method:'GET',url},null,2));
 const result=await page.evaluate(async url=>{
  const response=await fetch(url,{credentials:'include',signal:AbortSignal.timeout(30000)});
  return {status:response.status,body:await response.text()};
 },url);
 await writeFile(path.join(dir,'response.json'),result.body);
 return {...result,archive:path.relative(root,dir)};
}
