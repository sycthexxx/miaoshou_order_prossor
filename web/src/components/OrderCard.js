import {ref} from 'vue/dist/vue.esm-bundler.js';
import SearchSelect from './SearchSelect.js';
import ProductImage from './ProductImage.js';
export default {
 components:{ProductImage,SearchSelect},
 props:['order','suppliers','sellerMap','sourceKey','saving'],
 emits:['assign','pack'],
 setup(props,{emit}){
  const editing=ref({});
  const sellerName=source=>props.suppliers.find(s=>s.id===props.sellerMap[props.sourceKey(source)])?.name||'已指定卖家';
  function selectSeller(source,value){emit('assign',source,value,()=>{delete editing.value[props.sourceKey(source)]})}

  const money=(value,currency='')=>value===null||value===undefined||value===''?'—':`${currency} ${Number.isFinite(Number(value))?Number(value).toFixed(2):value}`;
  const image=i=>{const value=i.picUrlThumb||i.picUrl||i.originalPicUrl;return /^https?:\/\//i.test(value||'')?value:''};
  const date=v=>v?String(v).replace('T',' ').replace(/\.\d+Z$/,''): '—';
  const safe=url=>/^https?:\/\//i.test(url||'')?url:null;
  return {money,image,date,safe,editing,sellerName,selectSeller};
 },
 template:`<article class="order-card">
 <header class="order-heading"><span class="status-pill">{{order.appPackageStatusText||order.appPackageStatus||'未知状态'}}</span><strong>[{{order.platformName||order.platform}}<template v-if="order.siteName"> · {{order.siteName}}</template>]</strong><strong>{{order.shopName||order.platformShopName}}</strong><span class="order-number">{{order.platformOrderSn||order.orderInfo?.platformOrderSn}}</span><span class="payment">支付途径：{{order.orderInfo?.paymentMethod||'—'}}</span><span class="record-id">本地 #{{order.id}}</span></header>
 <div class="order-packing-action"><button class="secondary" @click="$emit('pack',order)">提交代打包</button></div>
 <div class="order-content">
 <div class="products"><div v-for="item in order.items" :key="item.opOrderItemId" class="product-row">
 <ProductImage :item="item"/>
 <div class="product-description"><div class="product-title"><span :title="item.title">{{item.title||item.cnTitle||'未命名商品'}}</span><strong>×{{item.quantity}}</strong></div><dl><div><dt>产品规格(中)</dt><dd class="spec">{{item.skuSubCnName||'—'}}</dd></div><div><dt>产品规格(原)</dt><dd>{{item.skuSubName||'—'}}</dd></div><div><dt>产品单价</dt><dd>{{money(item.discountedPrice??item.originalPrice,order.currency)}}</dd></div><div><dt>平台SKU</dt><dd class="sku">{{item.platformSkuId||'—'}}</dd></div><div v-if="item.platformOuterSkuId"><dt>商家编码</dt><dd>{{item.platformOuterSkuId}}</dd></div></dl><div class="order-sources"><span v-if="item.sourceState==='not_synced'">货源：产品尚未同步</span><span v-else-if="item.sourceState==='empty'">货源：未填写</span><span v-else-if="item.sourceState==='unknown'">货源：字段未返回</span><div v-for="(source,index) in item.sourceList" :key="index"><a v-if="safe(source.sourceItemUrl)" :href="source.sourceItemUrl" target="_blank" rel="noopener noreferrer">{{source.source}}货源 · {{source.sourceItemId}}</a><span v-else>货源 {{source.sourceItemId}}（无链接）</span><div v-if="source.sourceItemId" class="order-supplier"><span>指定卖家</span><template v-if="sellerMap[sourceKey(source)]&&!editing[sourceKey(source)]"><strong class="assigned-seller">{{sellerName(source)}}</strong><button type="button" class="seller-edit" :disabled="saving" @click="editing[sourceKey(source)]=true">修改</button></template><template v-else><SearchSelect :model-value="sellerMap[sourceKey(source)]||0" :options="suppliers.filter(s=>s.platform===source.source).map(s=>({value:s.id,label:s.name}))" :disabled="saving" label="指定卖家" @change="selectSeller(source,$event)"/><button v-if="editing[sourceKey(source)]" type="button" class="seller-edit" :disabled="saving" @click="delete editing[sourceKey(source)]">取消</button></template><small v-if="!suppliers.length&&!saving">暂无卖家，请先同步全部订单提取。</small></div></div></div></div>
 </div></div>
 <div class="info-column amounts"><dl><div><dt>订单金额</dt><dd>{{money(order.orderPackageAmountDetail?.orderAmount??order.orderInfo?.orderAmount,order.currency)}}<small v-if="order.orderPackageAmountDetail?.CNYOrderAmount!=null">({{money(order.orderPackageAmountDetail.CNYOrderAmount,'CNY')}})</small></dd></div><div><dt>预估回款</dt><dd>{{money(order.orderPackageAmountDetail?.escrowAmount??order.orderInfo?.escrowAmount,order.currency)}}<small v-if="order.orderPackageAmountDetail?.CNYEscrowAmount!=null">({{money(order.orderPackageAmountDetail.CNYEscrowAmount,'CNY')}})</small></dd></div></dl></div>
 <div class="info-column"><dl><div><dt>收件地区</dt><dd>{{order.consigneeInfo?.countryName||order.orderInfo?.buyerCountryName||'—'}}</dd></div><div><dt>姓名</dt><dd>{{order.consigneeInfo?.name||'—'}}</dd></div><div><dt>省州/邮编</dt><dd>{{order.consigneeInfo?.state||'—'}} / {{order.consigneeInfo?.zipcode||'—'}}</dd></div></dl><div class="buyer-message"><span>买家留言</span><p>{{order.orderInfo?.buyerMessage||'无'}}</p></div></div>
 <div class="info-column logistics"><dl><div><dt>包裹号</dt><dd>{{order.appPackageNo||'—'}}</dd></div><div><dt>物流方式</dt><dd>{{order.logisticsAgentProductInfo?.productName||order.logisticsMethod||'—'}}</dd></div><div><dt>运单号</dt><dd>{{order.logisticsNo||'未获取'}}</dd></div><div v-if="order.headLogisticsNo"><dt>头程单号</dt><dd>{{order.headLogisticsNo}}</dd></div></dl><p v-if="order.applyTrackingNoFailReason" class="failure">{{order.applyTrackingNoFailReason}}</p></div>
 <div class="info-column times"><dl><div><dt>下单</dt><dd>{{date(order.orderInfo?.gmtOrderStart)}}</dd></div><div><dt>付款</dt><dd>{{date(order.orderInfo?.gmtPay)}}</dd></div><div><dt>发货截止</dt><dd>{{date(order.gmtLastDelivery||order.orderInfo?.gmtLastDelivery)}}</dd></div><div><dt>取消截止</dt><dd>{{date(order.gmtLastCancel)}}</dd></div><div class="capture-time"><dt>本地采集</dt><dd>{{new Date(order.capturedAt).toLocaleString()}}</dd></div></dl></div>
 </div></article>`
};
