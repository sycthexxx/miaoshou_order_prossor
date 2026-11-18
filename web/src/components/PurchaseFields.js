import {ref,watch,onBeforeUnmount} from 'vue/dist/vue.esm-bundler.js';

export default {
 props:['purchase','account','api'],
 setup(props){
  const loading=ref(false),message=ref(''),failed=ref(false);
  let version=0;
  watch(()=>props.purchase.orderNumber,()=>{
   version++;loading.value=false;message.value='';failed.value=false;
   props.purchase.buyer='';props.purchase.seller='';
  },{flush:'sync'});
  onBeforeUnmount(()=>{version++});
  async function lookup(){
   const number=props.purchase.orderNumber.trim();
   if(!number||loading.value)return;
   const current=++version;loading.value=true;message.value='正在查询采购账号和卖家账号…';failed.value=false;
   const buyer=props.purchase.buyer,seller=props.purchase.seller;
   try{
    const result=await props.api(`/accounts/${props.account}/purchase-info?orderNumber=${encodeURIComponent(number)}`);
    if(current!==version)return;
    // Preserve edits made while the lookup was running.
    if(props.purchase.buyer===buyer)props.purchase.buyer=result.buyer;
    if(props.purchase.seller===seller)props.purchase.seller=result.seller;
    message.value='已查询采购账号和卖家账号，可继续修改。';
   }catch(e){if(current===version){failed.value=true;message.value=e.message}}
   finally{if(current===version)loading.value=false}
  }
  return {loading,message,failed,lookup};
 },
 template:`<div class="purchase-fields"><div class="packing-fields"><label>下单平台<input value="1688" disabled></label><label>采购单号<input v-model.trim="purchase.orderNumber" @blur="lookup" required placeholder="输入采购单号后自动查询"></label><label>采购账号<input v-model.trim="purchase.buyer" required></label><label>卖家账号<input v-model.trim="purchase.seller" required></label><label>采购单总实付（元，可选）<input v-model="purchase.payment" type="number" min="0" step="0.01"></label></div><div class="purchase-lookup"><button type="button" class="secondary" @click="lookup" :disabled="loading||!purchase.orderNumber">{{loading?'正在查询…':'查询采购账号和卖家账号'}}</button><span role="status" :class="{'purchase-lookup-error':failed}">{{message||'填写采购单号后自动查询，也可手动填写账号。'}}</span></div></div>`
};
