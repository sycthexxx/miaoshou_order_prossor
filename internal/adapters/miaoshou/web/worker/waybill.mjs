import {submitRequest} from './native-request.mjs';

export const waybillEndpoints={
 inspect:'/api/order/package/getOrderPackageDetailInfo',
 check:'/api/order/waybill/batchCheckOrdersHasCancelWaybill',
 apply:'/api/order/package/logistics/batchApplyTrackingNo',
};

export function requestWaybill(options){
 const {action,packageId}=options.payload;
 if(!waybillEndpoints[action]||!/^\d+$/.test(packageId))throw new Error('运单请求参数不正确');
 return submitRequest({...options,endpoint:waybillEndpoints[action],operation:'waybill',payload:action==='inspect'?{opOrderPackageId:packageId}:{opOrderPackageIds:[packageId]}});
}
