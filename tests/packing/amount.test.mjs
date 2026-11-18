import test from 'node:test';
import assert from 'node:assert/strict';
import {orderAmountColumns} from '../../web/src/lib/order-amount.js';
test('amount dialog uses authoritative profit and returns the ERP detail columns and signs',()=>{
 const view=orderAmountColumns({order:{currency:'MXN',site:'MX'},rate:{exchangeRate:'2.76873'},amount:{exchangeRate:'2.76873',payAmount:'273.28',CNYPayAmount:98.7,sellerDiscount:'344.09',CNYSellerDiscount:124.28,escrowAmount:'208.32',CNYEscrowAmount:75.24,purchasePrice:'108.26',CNYPurchasePrice:'39.10',orderProfit:'36.14',costProfitRatio:'92.43%',salesProfitRatio:'48.03%',incomeDetail:{originalTotalProductPrice:'598.19',platformDiscount:'50.82',buyerPaidShippingFee:70,originalShippingFee:'129',shippingFeePlatformDiscount:'59',estimatedCommissionFee:'-19.45',actualShippingFeeAmount:'-70',serviceAmount:'-20.33',estFeeTaxAmount:'-45.78'},costDetail:{}}});
 const find=label=>view.columns.flat().find(r=>r.label===label);
 assert.equal(find('商家承担的费用').original,-115.78);
 assert.equal(find('调整和其他').original,-6);
 assert.equal(find('TikTok运费折扣').original,-59);
 assert.equal(find('商家产品折扣').cny,124.28);
 assert.equal(find('采购金额').cny,'39.10');
 assert.equal(find('预估回款金额').cny,75.24);
 assert.equal(view.profit.cny,'36.14');
 assert.deepEqual(view.columns.map(c=>c[0].label),['买家支付的金额','商家承担的费用','预估回款金额']);
 assert.equal(view.costProfitRatio,'92.43%');
});

test('settled non-US bill retains settlement layout and platform amounts',()=>{
 const v=orderAmountColumns({order:{currency:'MXN',site:'MX',settled:true},rate:{exchangeRate:2},amount:{orderProfit:10,incomeDetail:{hasSettleAmount:true,revenueAmount:100,settlementAmount:80,subtotalBeforeDiscountAmount:120,sellerDiscountAmount:-20,subtotalAfterSellerDiscounts:100,actualSellerShippingCost:-10,estimatedCommissionFee:-10}}});
 assert.deepEqual(v.columns.map(c=>c[0].label),['总收入','总费用','回款金额']);
 assert.equal(v.columns[1][0].original,-20);assert.equal(v.columns[2][0].original,80);
 assert.equal(v.columns[1].find(r=>r.label==='其他费用').original,0);
});
