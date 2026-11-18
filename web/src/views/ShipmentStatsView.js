import {computed,ref,watch} from 'vue/dist/vue.esm-bundler.js';
import ProcurementDateField from '../components/ProcurementDateField.js';

import ProductImage from '../components/ProductImage.js';

function localDay(offset){
 const day=new Date();
 day.setDate(day.getDate()+offset);
 return [day.getFullYear(),String(day.getMonth()+1).padStart(2,'0'),String(day.getDate()).padStart(2,'0')].join('-');
}

export default {
 components:{ProcurementDateField,ProductImage},
 props:['api','account'],
 setup(props){
  const start=ref(localDay(-6)),end=ref(localDay(0));
  const groups=ref([]),loading=ref(false),error=ref('');
  const total=computed(()=>groups.value.reduce((sum,g)=>sum+g.quantity,0));
  function products(skus){
   const grouped=new Map();
   for(const sku of skus){
    if(!grouped.has(sku.title))grouped.set(sku.title,{title:sku.title,quantity:0,skus:[]});
    const product=grouped.get(sku.title);
    product.quantity+=sku.quantity;
    product.skus.push(sku);
   }
   return [...grouped.values()].sort((a,b)=>b.quantity-a.quantity);
  }
  watch([()=>props.account,start,end],async ([account,from,to],_,onCleanup)=>{
   let current=true;
   onCleanup(()=>{current=false});
   groups.value=[];error.value='';loading.value=false;
   if(!account)return;
   if((from||to)&&(!from||!to||from>to)){error.value='请选择完整有效的日期范围';return}
   loading.value=true;
   try{
    const query=new URLSearchParams({start:from,end:to});
    const result=await props.api('/accounts/'+account+'/shipment-stats?'+query);
    if(current)groups.value=result;
   }catch(e){if(current)error.value=e.message}
   finally{if(current)loading.value=false}
  },{immediate:true});
  return {start,end,groups,products,total,loading,error};
 },
 template:`<section class="shipment-stats">
  <div class="toolbar">
   <div class="procurement-date-range"><ProcurementDateField label="下单日期从" v-model="start" :max="end"/><span>至</span><ProcurementDateField label="下单日期至" v-model="end" :min="start"/></div>
   <button class="secondary" :aria-pressed="!start&&!end" @click="start='';end=''">全部历史</button>
   <strong v-if="account&&!loading&&!error">合计 {{total}} 件</strong>
  </div>
  <p class="scope-note">按下单日期汇总已同步的全部订单，包含赠品、取消及退款订单，件数按原订单数量计算。</p>
  <p v-if="error" class="error" role="alert">{{error}}</p>
  <p v-else-if="loading" role="status">正在统计…</p>
  <p v-else-if="!groups.length" class="empty">{{account?'所选周期没有符合条件的订单':'请先选择账户'}}</p>
  <details v-for="group in groups" :key="group.id" class="shipment-factory">
   <summary><strong>{{group.name}}</strong><span>{{group.skus.length}} 个 SKU · {{group.quantity}} 件</span></summary>
   <section v-for="product in products(group.skus)" :key="product.title" class="sales-product">
    <h3>{{product.title||'商品名未提供'}}</h3>
    <table><thead><tr><th>规格</th><th>按下单日期统计</th><th>合计</th></tr></thead>
     <tbody><tr v-for="(sku,index) in product.skus" :key="index">
      <td><div class="shipment-sku-product"><ProductImage :key="sku.picUrlThumb||sku.picUrl||sku.originalPicUrl" :item="sku"/><span>{{sku.spec||'规格未提供'}}</span></div></td>
      <td><div class="sales-day" v-for="day in Object.keys(sku.daily||{}).sort()" :key="day"><time>{{day}}</time><b>{{sku.daily[day]}} 件</b></div></td>
      <td><b>{{sku.quantity}}</b></td>
     </tr></tbody>
    </table>
   </section>
  </details>
 </section>`
};
