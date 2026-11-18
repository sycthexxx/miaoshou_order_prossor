import {formatOrderTime} from '../lib/procurement-orders.js';

export default {
 props:['order'],
 emits:['price'],
 data(){return {expanded:false}},
 methods:{formatOrderTime},
 template:`<div class="purchase-order-shortcut">
   <div class="purchase-line-price">
     <span>{{order.sequence}}. 订单号：{{order.orderNo||'未提供'}}</span>
     <button type="button" class="text-button" :aria-expanded="expanded" @click="expanded=!expanded">{{expanded?'收起详情':'订单详情'}}</button>
     <button v-if="order.packages.length===1" type="button" class="text-button order-price-link" @click="$emit('price',order.packages[0])">查看价格</button>
   </div>
   <div v-for="pkg in order.packages" :key="pkg.packageId" class="purchase-order-package">
     <div v-if="order.packages.length>1 || expanded" class="purchase-line-price">
       <span>包裹号：{{pkg.packageNo}}</span>
       <button v-if="order.packages.length>1" type="button" class="text-button" @click="$emit('price',pkg)">查看价格</button>
     </div>
     <template v-if="expanded">
       <div v-for="line in pkg.lines" :key="line.itemId" class="purchase-order-item">
         {{line.originalSpec||line.spec}} · {{line.quantity}} 件<span v-if="line.gift"> · 赠品</span><small>SKU {{line.skuId}}</small>
       </div>
     </template>
     <p v-for="note in pkg.notes" :key="note" class="purchase-local-note">本地备注：{{note}}</p>
   </div>
   <template v-if="expanded">
     <dl class="purchase-order-metadata">
       <div><dt>下单时间</dt><dd>{{formatOrderTime(order.packages[0].orderedAt)}}</dd></div>
       <div><dt>付款时间</dt><dd>{{formatOrderTime(order.packages[0].paidAt)}}</dd></div>
       <div><dt>支付方式</dt><dd>{{order.packages[0].paymentMethod||'—'}}</dd></div>
     </dl>
     <small>店铺 {{order.shopId}}</small>
   </template>
 </div>`
};
