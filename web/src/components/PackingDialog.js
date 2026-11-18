import WaybillFields from './WaybillFields.js';
import PurchaseFields from './PurchaseFields.js';
import {ref,reactive,computed,onMounted} from 'vue/dist/vue.esm-bundler.js';
export default {
 components:{WaybillFields,PurchaseFields},
 props:['order','account','api'],emits:['close','submitted'],
 setup(props,{emit}){
  const dialog=ref(),busy=ref(false),error=ref(''),result=ref(null),syncStarted=ref(false);
  const mode=ref('purchase'),selected=ref(props.order.items.filter(i=>!Number(i.isTiktokGift??i.isGift??0)).map(i=>String(i.opOrderItemId)));
  const hasMain=computed(()=>props.order.items.some(i=>selected.value.includes(String(i.opOrderItemId))&&!Number(i.isTiktokGift??i.isGift??0)));
  const appliedSelected=computed(()=>[...new Set([...selected.value,...(hasMain.value?props.order.items.filter(i=>Number(i.isTiktokGift??i.isGift??0)).map(i=>String(i.opOrderItemId)):[])])]);
  function toggleItem(id,checked){selected.value=checked?[...selected.value,id]:selected.value.filter(value=>value!==id)}
  const purchase=reactive({platform:'1688',orderNumber:'',buyer:'',seller:'',payment:'',recipient:'',mobile:'',address:'',createdAt:'',refund:'',status:'',shippedAt:''});
  const company=ref(''),number=ref(''),autoSync=ref(true),amounts=reactive({});
  const locked=computed(()=>busy.value||['completed','partial','unknown'].includes(result.value?.state));
  const gift=i=>Number(i.isTiktokGift??i.isGift??0)===1;
  onMounted(()=>dialog.value.showModal());
  function close(){if(!busy.value)emit('close')}
  async function submit(){
   if(locked.value||!selected.value.length)return;
   busy.value=true;error.value='';result.value=null;
   const items=appliedSelected.value.map(id=>({orderItemId:id,amount:amounts[id]||'',autoSyncAmount:mode.value==='purchase'&&autoSync.value,...(mode.value==='purchase'?{purchase:{...purchase}}:{waybills:[{company:company.value,number:number.value}]})}));
   try{result.value=await props.api(`/accounts/${props.account}/packages/${props.order.opOrderPackageId}/packing`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({items})});emit('submitted')}
   catch(e){error.value=e.message;/* A lost HTTP response must not encourage duplicate writes. */if(!e.status||e.status>=500)result.value={state:'unknown',message:'未收到提交结果，请先在妙手核对。'}}
   finally{busy.value=false}
  }
  async function sync(){busy.value=true;error.value='';try{await props.api(`/accounts/${props.account}/sync-pending`,{method:'POST'});syncStarted.value=true;emit('submitted')}catch(e){error.value=e.message}finally{busy.value=false}}
  return{dialog,busy,error,result,mode,selected,appliedSelected,hasMain,toggleItem,purchase,company,number,autoSync,amounts,locked,gift,submit,close,sync,syncStarted};
 },
 template:`<dialog ref="dialog" class="packing-dialog" aria-labelledby="packing-title" @cancel.prevent="close">
 <form @submit.prevent="submit"><header><div><h2 id="packing-title">提交代打包</h2><p>包裹 {{order.appPackageNo||order.opOrderPackageId}} · 不使用货代</p></div><button type="button" class="secondary" :disabled="busy" @click="close" aria-label="关闭打包表单">关闭</button></header>
 <p class="packing-help">选择本次要关联的商品，填写采购信息或国内快递单号。选择普通商品时，同包裹赠品自动使用同一份信息随单提交。包裹采购信息不会推送给货代。</p>
 <fieldset :disabled="locked"><legend>订单商品</legend><label v-for="item in order.items" :key="item.opOrderItemId" class="packing-item"><input type="checkbox" :checked="appliedSelected.includes(String(item.opOrderItemId))" :disabled="gift(item)&&hasMain" @change="toggleItem(String(item.opOrderItemId),$event.target.checked)"><span><b v-if="gift(item)" class="status-pill">赠品</b>{{item.title||item.cnTitle}}<small>{{item.skuSubCnName||item.skuSubName||'—'}} · {{item.quantity}} 件</small></span><span class="packing-amount" v-if="mode==='waybill'||!autoSync">采购金额<input v-model="amounts[String(item.opOrderItemId)]" type="number" min="0" step="0.01" placeholder="产品实付" :aria-label="'商品 '+item.opOrderItemId+' 采购金额'"></span></label></fieldset>
 <fieldset :disabled="locked"><legend>录入方式</legend><div class="packing-modes"><label><input type="radio" value="purchase" v-model="mode">录入采购信息</label><label><input type="radio" value="waybill" v-model="mode">录入国内快递单号</label></div>
 <div v-if="mode==='purchase'"><p class="packing-help">以下 1688 采购信息应用到所选的 {{appliedSelected.length}} 条商品。</p><PurchaseFields :purchase="purchase" :account="account" :api="api"/><label class="packing-auto"><input type="checkbox" v-model="autoSync">自动同步商品采购金额</label>
 <details><summary>补充采购信息（可选）</summary><div class="packing-fields"><label>收件人<input v-model.trim="purchase.recipient"></label><label>联系电话<input v-model.trim="purchase.mobile"></label><label>收件地址<input v-model.trim="purchase.address"></label><label>采购时间<input v-model.trim="purchase.createdAt" placeholder="2026-11-17 16:44:52"></label><label>退款金额（元）<input type="number" min="0" step="0.01" v-model="purchase.refund"></label><label>采购状态<select v-model="purchase.status"><option value="">不填写</option><option value="wait_send">待发货</option></select></label></div></details></div>
 <div v-else class="packing-fields"><WaybillFields :api="api" :account="account" v-model:number="number" v-model:company="company"/></div>
 </fieldset>
 <p v-if="error" class="error" role="alert">{{error}}</p><section v-if="result" class="packing-result" role="status"><strong>{{result.message}}</strong><p v-if="result.taskId"><a href="#/tasks">查看任务 #{{result.taskId}} ↗</a></p><pre v-if="result.details?.length">{{JSON.stringify(result.details,null,2)}}</pre><small v-if="result.archive">本地记录：{{result.archive}}</small><p v-if="syncStarted">已开始同步待处理订单，完成后点击订单列表“查询”刷新。</p><button v-else-if="result.state==='completed'||result.state==='partial'" type="button" class="secondary" :disabled="busy" @click="sync">同步待处理订单</button></section>
 <footer><span>已选 {{appliedSelected.length}} 条商品</span><button type="button" class="secondary" :disabled="busy" @click="close">{{result?'关闭':'取消'}}</button><button type="submit" :disabled="locked||!selected.length">{{busy?'正在处理…':'提交到妙手'}}</button></footer></form></dialog>`
};
