import {submitRequest} from './native-request.mjs';
export {requestClientExport} from './native-request.mjs';
export const packingEndpoint='/api/order/purchase/manualRelateOrderPackagePurchaseOrders';
export function submitPacking(options){return submitRequest({...options,endpoint:packingEndpoint,operation:'packing'})}
