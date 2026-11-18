const number=value=>value==null||value===''||value==='--'?0:Number(value);
const sum=(...values)=>values.reduce((n,v)=>n+number(v),0);
const round=value=>Number(Number(value).toFixed(2));

// TikTok's amount dialog combines the package's authoritative totals with
// income/cost detail rows. Detail conversions use the ERP currency settings.
export function orderAmountColumns({amount:a,order,rate}){
 const i=a.incomeDetail||{},cost=a.costDetail||{};
 const exchange=number(rate?.customExchangeRate||rate?.exchangeRate||a.exchangeRate)||1;
 const cny=value=>round(number(value)/exchange);
 const row=(label,value,{cnyValue,indent=false,help='',input=false,originNote=false}={})=>({label,original:value==null?'--':round(number(value)),cny:cnyValue??(value==null?'--':cny(value)),indent,help,input,originNote});
 const money=(label,key,options)=>row(label,i[key],options);
 const fee=sum(i.transactionFeeAmount,i.creditCardHandlingFeeAmount);
 const commission=number(i.platformCommission)||number(i.estimatedCommissionFee);
 const shipping=sum(i.actualShippingFeeAmount,i.internationalLegLogisticsAmount,i.shippingFeeDiscountAmount,i.returnShippingFeeAmount,i.shippingFeeSubsidyAmount);
 const affiliate=sum(i.affiliatePartnerCommissionAmount,i.affiliateCommissionAmount,i.affiliateAdsCommissionAmount);
 const tax=number(i.taxAmount),service=number(i.serviceAmount),referral=order.site==='US'?number(i.referralFeeAmount):0;
 const adjustment=round(sum(i.estFeeTaxAmount,i.estShippingCostAmount,i.estAdjustmentAmount)-sum(fee,commission,shipping,affiliate,tax,service,i.buyerPaidShippingFee,referral));
 const merchant=sum(fee,commission,shipping,affiliate,tax,service,adjustment,referral);
 const shippingRows=[row('实际运费',number(i.actualShippingFeeAmount),{indent:true}),...(number(i.internationalLegLogisticsAmount)?[money('国际段运费','internationalLegLogisticsAmount',{indent:true})]:[]),...(number(i.shippingFeeDiscountAmount)?[money('平台包邮运费','shippingFeeDiscountAmount',{indent:true})]:[]),row('实际退货运费',number(i.returnShippingFeeAmount),{indent:true}),row('运费补贴',number(i.shippingFeeSubsidyAmount),{indent:true})];
 const left=[row('买家支付的金额',a.payAmount,{cnyValue:a.CNYPayAmount,help:'产品实付价 + 买家实付运费 + 买家税费'}),row('产品实付价',sum(i.originalTotalProductPrice,-number(a.sellerDiscount),-number(i.platformDiscount)),{help:'产品原价 − 商家产品折扣 − TikTok产品折扣'}),money('产品原价','originalTotalProductPrice',{indent:true}),row('商家产品折扣',a.sellerDiscount,{cnyValue:a.CNYSellerDiscount??i.CNYSellerDiscount,indent:true}),money('TikTok产品折扣','platformDiscount',{indent:true}),row('买家实付运费',number(i.buyerPaidShippingFee),{help:'运费原价 − 商家运费折扣 − TikTok运费折扣'}),money('运费原价','originalShippingFee',{indent:true}),row('商家运费折扣',number(i.shippingFeeSellerDiscount),{indent:true}),row('TikTok运费折扣',-Math.abs(number(i.shippingFeePlatformDiscount)),{indent:true}),row('买家税费',number(i.buyTaxAmount))];
 const middle=[row('商家承担的费用',merchant,{help:'交易手续费 + 平台佣金 + 商家运费 + 联盟佣金 + 税费 + 服务费 + 调整和其他'}),row('交易手续费',fee,{help:'订单交易手续费 + 买家信用卡支付手续费'}),row('TikTok平台佣金',commission,{help:'平台接口返回的佣金'}),...(order.site==='US'?[money('推荐费','referralFeeAmount')]:[]),row('商家运费',shipping,{help:'实际运费、退货运费及运费补贴合计'}),...shippingRows,row('联盟佣金',affiliate,{help:'合作伙伴佣金 + 达人佣金 + 联盟商店广告佣金'}),row('税费',tax,{help:'平台返回的商家税费'}),row('服务费',service,{help:'平台返回的服务费用合计'}),row('调整和其他',adjustment,{help:'其他费用和税费调整，以平台账单为准'})];
 const cnyCost=(label,key,input=true,originNote=false)=>{const value=number(cost[key]??a[key]);return row(label,round(value*exchange),{cnyValue:value,input,originNote,help:label})};
 const right=[row('预估回款金额',a.escrowAmount,{cnyValue:a.CNYEscrowAmount,help:'妙手接口返回的预估回款金额'}),row('采购金额',a.purchasePrice,{cnyValue:a.CNYPurchasePrice??0,input:true}),cnyCost('运费成本','CNYShippingFeeCost',false),cnyCost('仓库操作费','CNYForwarderFreight'),cnyCost('包材费','CNYPackagingCost'),cnyCost('其他成本','CNYOtherCost'),cnyCost('广告成本','CNYAdCost',false,true),cnyCost('运营成本','CNYMarketingCost',false,true)];
 const settled=order.settled||i.hasSettleAmount===true;
 if(settled){
  const total=number(i.revenueAmount??sum(i.subtotalAfterSellerDiscounts,i.refundSubtotalAfterSellerDiscounts));
  const settlement=number(i.settlementAmount),totalFee=round(settlement-total);
  const us=order.site==='US';
  left.splice(0,left.length,...(us?[row('总收入',total),row('买家付款',sum(i.subtotalBeforeDiscountAmount,i.sellerDiscountAmount,i.customerPaidShippingFeeAmount,i.salesTaxPaymentAmount,i.customerRefundAmount)),row('商家折扣后价格',sum(i.subtotalBeforeDiscountAmount,i.sellerDiscountAmount),{indent:true}),money('买家实付运费','customerPaidShippingFeeAmount',{indent:true}),money('缴纳销售税','salesTaxPaymentAmount',{indent:true})]:[row('总收入',total),money('享受商家折扣后小计','subtotalAfterSellerDiscounts'),money('商家折扣前小计','subtotalBeforeDiscountAmount',{indent:true}),money('商家折扣','sellerDiscountAmount',{indent:true})]));
  const sellerShipping=us?shipping:number(i.actualSellerShippingCost);
  const other=round(totalFee-sum(fee,commission,referral,sellerShipping,affiliate,tax,us?i.retailDeliveryFeeAmount:service,i.adjustmentAmount));
  middle.splice(0,middle.length,row('总费用',totalFee),row('交易手续费',fee),row('TikTok平台佣金',commission),...(us?[money('推荐费','referralFeeAmount')]:[]),row('商家运费',sellerShipping),row('实际运费',number(i.actualShippingFeeAmount),{indent:true}),...(number(i.internationalLegLogisticsAmount)?[money('国际段运费','internationalLegLogisticsAmount',{indent:true})]:[]),...(number(i.shippingFeeDiscountAmount)?[money('平台运费减免','shippingFeeDiscountAmount',{indent:true})]:[]),...(us?[row('实际退货运费',number(i.returnShippingFeeAmount),{indent:true})]:[money('买家实付运费','customerPaidShippingFeeAmount',{indent:true}),row('运费补贴',number(i.shippingFeeSubsidyAmount),{indent:true})]),row('联盟佣金',affiliate),row('税费',tax),...(us?[money('零售配送费','retailDeliveryFeeAmount')]:[row('服务费',service)]),money('调整金额','adjustmentAmount'),row('其他费用',other));
  right[0]=money(us?'预估回款金额':'回款金额','settlementAmount');
 }
 return {columns:[left,middle,right],settled,exchange,profit:row('预估利润',round(number(a.orderProfit)*exchange),{cnyValue:a.orderProfit,help:'回款金额扣除采购及其他成本'}),costProfitRatio:a.costProfitRatio,salesProfitRatio:a.salesProfitRatio};
}
