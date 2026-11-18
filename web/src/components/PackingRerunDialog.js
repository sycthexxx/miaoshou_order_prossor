import PurchaseFields from './PurchaseFields.js';
import {ref,reactive,onMounted} from 'vue/dist/vue.esm-bundler.js';
import ProductImage from './ProductImage.js';
import {rerunGroups,rerunRequests} from '../lib/packing-rerun.js';
export default {
 components:{ProductImage,PurchaseFields},
 props:['job','account','api'],emits:['close','created'],
 setup(props,{emit}){
  const dialog=ref(),busy=ref(false),uncertain=ref(false),error=ref(''),interval=ref(props.job.intervalSeconds);
  const groups=reactive(rerunGroups(props.job));
  function modeChanged(group){if(group.mode==='purchase'){group.info.purchase??={platform:'1688',orderNumber:'',buyer:'',seller:''}}else{group.info.waybills??=[{company:'',number:''}]}}
  function close(){if(!busy.value)emit('close')}
  async function save(){
   if(busy.value||uncertain.value)return;busy.value=true;error.value='';
   try{const result=await props.api(`/accounts/${props.account}/packing-jobs/${props.job.id}/rerun`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({intervalSeconds:Number(interval.value),requests:rerunRequests(groups)})});emit('created',result.id);emit('close')}
   catch(e){error.value=e.message;if(!e.status||e.status>=500){uncertain.value=true;error.value+='。请关闭表单查看任务列表，确认是否已保存。';emit('created',null)}}finally{busy.value=false}
  }
  onMounted(()=>dialog.value.showModal());
  return{dialog,busy,uncertain,error,interval,groups,modeChanged,close,save};
 },template:`<dialog ref="dialog" class="packing-dialog batch-packing-dialog" aria-labelledby="rerun-title" @cancel.prevent="close"><form @submit.prevent="save"><header><div><h2 id="rerun-title">修改并重新执行</h2><p>原任务 #{{job.id}} · {{job.name}}</p></div><button type="button" class="secondary" :disabled="busy" @click="close">关闭</button></header>
 <p class="packing-help">保存后会创建新任务，再次向妙手提交以下 {{job.items.length}} 个包裹；原任务与原提交记录保留。同包裹赠品已自动加入，共用主商品的提交信息。所有包裹依次执行。</p>
 <fieldset v-for="(group,index) in groups" :key="index" :disabled="busy||uncertain"><legend>打包信息 {{index+1}} · 应用 {{group.lines.length}} 条商品</legend>
 <details class="rerun-scope"><summary>查看应用包裹、规格和赠品</summary><div v-for="line in group.lines" :key="line.packageId+':'+line.itemId" class="rerun-sku-line"><ProductImage :item="line.image||{}"/><div><strong>{{line.spec||'规格未提供'}} · {{line.quantity}} 件<span v-if="line.gift" class="status-pill">赠品</span></strong><small>包裹 {{line.packageNo||line.packageId}}</small><small>SKU {{line.sku||'未提供'}}</small></div></div></details>
 <div class="packing-modes"><label><input type="radio" value="purchase" v-model="group.mode" @change="modeChanged(group)">录入采购信息</label><label><input type="radio" value="waybill" v-model="group.mode" @change="modeChanged(group)">录入快递单号</label></div>
 <template v-if="group.mode==='purchase'"><PurchaseFields :purchase="group.info.purchase" :account="account" :api="api"/><label class="packing-auto"><input type="checkbox" v-model="group.info.autoSyncAmount">自动同步商品采购金额</label>
 <details><summary>补充采购信息</summary><div class="packing-fields"><label>收件人<input v-model="group.info.purchase.recipient"></label><label>联系电话<input v-model="group.info.purchase.mobile"></label><label>收件地址<input v-model="group.info.purchase.address"></label><label>采购时间<input v-model="group.info.purchase.createdAt"></label><label>退款金额<input type="number" step="0.01" min="0" v-model="group.info.purchase.refund"></label></div></details></template>
 <template v-else><div v-for="(waybill,n) in group.info.waybills" :key="n" class="packing-fields"><label>国内快递单号<input v-model.trim="waybill.number" required></label><label>物流公司<input v-model.trim="waybill.company" required></label><button v-if="group.info.waybills.length>1" type="button" class="text-button" @click="group.info.waybills.splice(n,1)">移除快递单</button></div><button type="button" class="text-button" @click="group.info.waybills.push({company:'',number:''})">添加快递单</button></template>
 <label v-if="group.mode==='waybill'||!group.info.autoSyncAmount" class="batch-amount">每条商品采购金额（元，可选）<input type="number" min="0" step="0.01" v-model="group.info.amount"></label><label>备注<input v-model="group.info.note"></label></fieldset>
 <fieldset :disabled="busy||uncertain"><legend>执行安排</legend><label>每个包裹完成后的等待时间（秒）<input type="number" min="3" max="300" v-model="interval" required></label></fieldset><p v-if="error" class="error" role="alert">{{error}}</p><footer><strong>{{job.items.length}} 个包裹 · {{groups.reduce((n,g)=>n+g.lines.length,0)}} 条商品（含赠品）</strong><button type="button" class="secondary" @click="close" :disabled="busy">取消</button><button type="submit" :disabled="busy||uncertain">{{busy?'正在保存…':'保存为新任务并执行'}}</button></footer></form></dialog>`
};
