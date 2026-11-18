import {formatOrderTime} from '../lib/procurement-orders.js';
import PurchaseFields from './PurchaseFields.js';
import {ref,reactive,computed,onMounted} from 'vue/dist/vue.esm-bundler.js';
import ProductImage from './ProductImage.js';
import {sourceLines,filteredLines,selectionStats,groupLinesBySpec,filterOptions} from '../lib/packing-selection.js';
export default {
 components:{ProductImage,PurchaseFields},props:['group','account','api','gifts'],emits:['close','created'],
 setup(props,{emit}){
  const dialog=ref(),busy=ref(false),uncertain=ref(false),error=ref('');
  const lines=sourceLines(props.group),selected=ref(new Set()),filters=reactive({sources:[],colors:[],sizes:[],query:''});
  const colors=computed(()=>filterOptions(lines,filters,'color')),sizes=computed(()=>filterOptions(lines,filters,'size'));
  const visible=computed(()=>filteredLines(lines,filters)),stats=computed(()=>selectionStats(lines,selected.value));
  const variants=computed(()=>groupLinesBySpec(visible.value));
  const appliedVariants=computed(()=>groupLinesBySpec(lines.filter(l=>selected.value.has(l.key))));
  function toggle(list,checked){const next=new Set(selected.value);for(const l of list)checked?next.add(l.key):next.delete(l.key);selected.value=next}
  const allSelected=list=>list.length>0&&list.every(l=>selected.value.has(l.key));
  const mode=ref('waybill'),company=ref(''),number=ref(''),amount=ref(''),autoSync=ref(true),interval=ref(5);
  const suffix=Math.floor(100000+Math.random()*900000);
  const name=computed(()=>`${props.group.supplier?.name||'未指定厂家'}-${stats.value.quantity}件-${suffix}`);
  const purchase=reactive({platform:'1688',orderNumber:'',buyer:'',seller:'',payment:''});
  onMounted(()=>dialog.value.showModal());
  function close(){if(!busy.value)emit('close')}
  async function save(){
   if(busy.value||uncertain.value||!stats.value.lines)return;busy.value=true;error.value='';
   const selections=lines.filter(l=>selected.value.has(l.key)).map(l=>({packageId:String(l.packageId),itemId:String(l.itemId)}));
   const info={amount:amount.value,autoSyncAmount:mode.value==='purchase'&&autoSync.value,...(mode.value==='purchase'?{purchase:{...purchase}}:{waybills:[{company:company.value,number:number.value}]})};
   try{const result=await props.api(`/accounts/${props.account}/packing-jobs`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name.value,sourceKey:props.group.key,gifts:props.gifts,intervalSeconds:Number(interval.value),selections,info})});emit('created',result.id);emit('close')}
   catch(e){error.value=e.message;if(!e.status||e.status>=500){uncertain.value=true;error.value+='。请关闭表单并查看任务列表，确认是否已保存，避免重复创建。';emit('created',null)}}finally{busy.value=false}
  }
  return{formatOrderTime,dialog,busy,error,uncertain,lines,selected,filters,colors,sizes,visible,variants,appliedVariants,stats,toggle,allSelected,mode,company,number,amount,autoSync,interval,name,purchase,close,save};
 },template:`<dialog ref="dialog" class="packing-dialog batch-packing-dialog" aria-labelledby="batch-packing-title" @cancel.prevent="close"><form @submit.prevent="save">
 <header><div><h2 id="batch-packing-title">{{group.sources?'厂家批量提交代打包':'货源批量提交代打包'}}</h2><p v-if="group.sources">{{group.supplier.name}} · {{group.sources.length}} 个货源 · 不使用货代</p><p v-else>{{group.source.source}} · {{group.source.sourceItemId}} · 不使用货代</p></div><button type="button" class="secondary" :disabled="busy" @click="close">关闭</button></header>
 <fieldset :disabled="busy||uncertain" class="batch-info"><legend>填写打包信息</legend><div v-if="!group.sources" class="batch-product"><ProductImage :item="group.image"/><div><strong>{{group.title}}</strong><p>{{group.supplier?.name||'未指定厂家'}}</p></div></div>
 <div class="packing-modes"><label><input type="radio" value="waybill" v-model="mode">录入快递单号</label><label><input type="radio" value="purchase" v-model="mode">录入采购信息（自动同步快递单号）</label></div>
 <div v-if="mode==='waybill'" class="packing-fields"><label>国内快递单号<input v-model.trim="number" required placeholder="请输入国内快递单号"></label><label>物流公司<input v-model.trim="company" required placeholder="请选择或输入物流公司" list="batch-logistics"></label><datalist id="batch-logistics"><option>中通快递</option><option>圆通速递</option><option>申通快递</option><option>韵达快递</option><option>顺丰速运</option><option>极兔速递</option><option>邮政快递包裹</option><option>京东物流</option></datalist></div>
 <div v-else><PurchaseFields :purchase="purchase" :account="account" :api="api"/><label class="packing-auto"><input v-model="autoSync" type="checkbox">自动同步商品采购金额</label></div>
 <label v-if="mode==='waybill'||!autoSync" class="batch-amount">每条订单商品采购金额（元，可选）<input v-model="amount" type="number" min="0" step="0.01" placeholder="产品实付"></label><p class="packing-help">以上信息应用到勾选的{{group.sources?'不同货品的':''}}订单商品。采购金额按商品行填写，不会把本次采购总额分摊到订单。同包裹赠品会自动使用这份信息一起提交，其他普通商品不会自动加入。</p></fieldset>
 <fieldset :disabled="busy||uncertain"><legend>一份打包信息，应用到{{group.sources?'多个货品、':''}}规格和订单</legend><p class="packing-help">可同时勾选不同颜色、尺码的多个规格。下方全部已选订单共用上面这一份打包信息，保存为一个任务。</p>
 <div v-if="group.sources" class="batch-source-options"><strong>货品筛选（多选，不选则显示全部）</strong><label v-for="source in group.sources" :key="source.key"><input type="checkbox" v-model="filters.sources" :value="source.key"><ProductImage :item="source.image||{}"/><span><strong>{{source.title}}</strong><small>{{source.source.sourceItemId}} · {{source.quantity}} 件</small></span></label></div>
 <div class="batch-multi-filters"><div><strong>颜色筛选（多选）</strong><div class="batch-filter-options"><label v-for="c in colors" :key="c.value"><input type="checkbox" v-model="filters.colors" :value="c.value">{{c.value}}<sup class="batch-filter-count" :title="c.packages+' 个订单'">{{c.packages}}单</sup></label></div></div><div><strong>尺码筛选（多选）</strong><div class="batch-filter-options"><label v-for="s in sizes" :key="s.value"><input type="checkbox" v-model="filters.sizes" :value="s.value">{{s.value}}<sup class="batch-filter-count" :title="s.quantity+' 件'">{{s.quantity}}件</sup></label></div></div><label>规格 / SKU / 包裹号<input v-model="filters.query" placeholder="搜索完整规格、SKU 或包裹号"></label><button type="button" class="text-button" @click="filters.sources=[];filters.colors=[];filters.sizes=[];filters.query=''">清除筛选，显示全部规格</button></div>
 <div class="batch-selection-tools"><button type="button" class="secondary" @click="toggle(visible,true)" :disabled="!visible.length">勾选筛选结果（{{visible.length}} 条）</button><button type="button" class="secondary" @click="toggle(visible,false)" :disabled="!visible.length">取消筛选结果</button><button type="button" class="text-button" @click="selected=new Set()">清空全部选择</button><span>切换筛选会保留已选订单</span></div>
 <div v-if="appliedVariants.length" class="batch-applied"><strong>共用这份打包信息：已选 {{appliedVariants.length}} 个规格</strong><div><span v-for="v in appliedVariants" :key="v.key">{{group.sources?v.sourceItemId+' · ':''}}{{v.spec}} · {{v.lines.length}} 条 · {{v.quantity}} 件<button type="button" class="text-button" :aria-label="'取消应用 '+v.spec" @click="toggle(v.lines,false)">×</button></span></div></div>
 <div class="batch-spec-options" aria-label="选择应用规格"><label v-for="v in variants" :key="v.key"><input type="checkbox" :checked="allSelected(v.lines)" :indeterminate="!allSelected(v.lines)&&v.lines.some(l=>selected.has(l.key))" @change="toggle(v.lines,$event.target.checked)"><span><small v-if="group.sources" class="batch-variant-product">货源 {{v.sourceItemId}} · {{v.title}}</small><strong>{{v.spec}}</strong><small>{{v.lines.length}} 条订单商品 · {{v.quantity}} 件</small></span></label><p v-if="!variants.length" class="packing-help">没有符合筛选条件的规格。已选的其他规格仍会应用。</p></div>
 <details class="batch-individual-orders"><summary>按包裹逐条勾选（可选）</summary>
 <div class="batch-order-list"><section v-for="variant in variants" :key="variant.key" class="batch-variant"><header><label><input type="checkbox" :checked="allSelected(variant.lines)" :indeterminate="!allSelected(variant.lines)&&variant.lines.some(l=>selected.has(l.key))" @change="toggle(variant.lines,$event.target.checked)"><strong>{{group.sources?variant.sourceItemId+' · ':''}}{{variant.spec}}</strong></label><span>{{variant.lines.length}} 条商品 · {{variant.lines.reduce((n,l)=>n+Number(l.quantity),0)}} 件</span></header><label v-for="line in variant.lines" :key="line.key" class="batch-order-line"><input type="checkbox" :checked="selected.has(line.key)" @change="toggle([line],$event.target.checked)"><span class="batch-order-summary">
  <b>{{line.packageNo||line.packageId}}</b>
  <span class="batch-order-number">订单号：{{line.orderNo||'—'}}</span>
  <small>店铺 {{line.shopId}} · SKU {{line.skuId}}<template v-if="line.gift"> · 赠品</template></small>
  <small>下单时间：{{formatOrderTime(line.orderedAt)}}</small>
  <small>付款时间：{{formatOrderTime(line.paidAt)}}</small>
  <span v-if="line.localNote" class="batch-order-note">本地备注：{{line.localNote}}</span>
 </span><span>{{line.quantity}} 件</span></label></section><p v-if="!visible.length" class="packing-help">没有符合筛选条件的订单商品。</p></div></details></fieldset>
 <fieldset :disabled="busy||uncertain"><legend>执行安排</legend><div class="packing-fields"><label>任务名称（自动生成）<output class="batch-task-name">{{name}}</output></label><label>每个包裹完成后的等待时间（秒）<input v-model="interval" type="number" min="3" max="300" required></label></div><p class="packing-help">保存后在后台逐个执行，同一包裹的商品合并提交。上一包裹返回并保存后，再等待 {{interval}} 秒提交下一包裹。可以关闭页面，在任务列表查看、暂停或继续。</p></fieldset>
 <p v-if="error" class="error" role="alert">{{error}}</p><footer><strong>共用一份信息 · {{appliedVariants.length}} 个规格 · {{stats.packages}} 个包裹 · {{stats.lines}} 条商品 · {{stats.quantity}} 件（同包裹赠品自动随单提交）</strong><button type="button" class="secondary" :disabled="busy" @click="close">取消</button><button type="submit" :disabled="busy||uncertain||!stats.lines">{{busy?'正在保存…':'保存并开始任务'}}</button></footer>
 </form></dialog>`
};
