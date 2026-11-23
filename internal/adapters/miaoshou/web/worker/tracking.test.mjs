import assert from 'node:assert/strict';
import {test} from 'node:test';
import {getTracking} from './tracking.mjs';

test('uses the read-only domestic and TikTok routes and normalizes their steps', async () => {
  const page={url:()=> 'https://erp.91miaoshou.com/order/package/index',evaluate:(fn,arg)=>fn(arg)};
  const client={url:'data:text/javascript,'+encodeURIComponent(`export function read({url,method,params}){
    if(method!=='GET')throw Error('query must be read only');
    if(url==='/order/purchase/purchase_order/getTraceInfo'&&params.purchaseOrderFilterId==='55'&&params.waybillCode==='YT998')
      return {data:{logisticsSteps:[{acceptTime:'2026-11-23 09:00',remark:'已揽收'}]}};
    if(url==='/order/purchase/getLogisticsTracking'&&params.opOrderPackageId==='29')
      return {data:{newestTrackingNo:'DCS124'},trackingInfo:[{gmtCreate:'2026-11-23 10:00',descriptionCn:'已交运',description:'Dispatched'}]};
    throw Error('unexpected query');
  }`),name:'read'};
  const run=async payload=>JSON.parse((await getTracking({payload,session:{context:{pages:()=>[page]}},resolveClient:()=>client})).body);
  const domestic=await run({kind:'domestic',traceId:'55',number:'YT998'});
  assert.deepEqual(domestic.steps,[{time:'2026-11-23 09:00',description:'已揽收',detail:''}]);
  const platform=await run({kind:'platform',packageId:'29',number:'DCS123'});
  assert.equal(platform.number,'DCS124');
  assert.equal(platform.steps[0].detail,'Dispatched');
});
