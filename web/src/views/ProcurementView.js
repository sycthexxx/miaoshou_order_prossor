import {useSupplierLeases} from '../lib/supplier-leases.js';
import ProcurementOrderLine from '../components/ProcurementOrderLine.js';
import ProcurementDateField from '../components/ProcurementDateField.js';
import {filterProcurementByOrderDate,procurementOrderRows} from '../lib/procurement-orders.js';
import OrderAmountDialog from '../components/OrderAmountDialog.js';
import {supplierPackingGroup} from '../lib/packing-selection.js';
import {ref,computed,watch,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';
import {procurementHandoff} from '../lib/procurement-handoff.js';
import {copyText} from '../lib/copy-text.js';
import ProductImage from '../components/ProductImage.js';
import BatchPackingDialog from '../components/BatchPackingDialog.js';
import PackingJobs from '../components/PackingJobs.js';
export default {components:{ProcurementOrderLine,ProcurementDateField,OrderAmountDialog,ProductImage,BatchPackingDialog,PackingJobs},props:['account','api','user'],setup(props){
 const data=ref({groups:[],quantity:0,unresolvedQuantity:0,packages:0}),gifts=ref(false),query=ref(''),loading=ref(false),error=ref('');const loadedAt=ref(''),loadedGifts=ref(false);let serial=0;
 const amountLine=ref(null);
 const dateField=ref('orderedAt'),dateStart=ref(''),dateEnd=ref('');
 const report=computed(()=>filterProcurementByOrderDate(data.value,dateStart.value,dateEnd.value,dateField.value));
 const dateActive=computed(()=>Boolean(dateStart.value||dateEnd.value));
 function clearDates(){dateStart.value='';dateEnd.value=''}
 const packingGroup=ref(null),openWithoutList=ref('');
 const {owners,held,pending,ready,acquire,release}=useSupplierLeases(props,id=>{
  if(packingGroup.value && Number(id)===(packingGroup.value.supplier?.id||0))packingGroup.value=null;
  openWithoutList.value='';
 },message=>error.value=message);
 async function startPurchase(id){
  error.value='';openWithoutList.value='';
  const lease=await acquire(id);
  if(lease && !isSellerExpanded({id}))toggleSeller({id});
  return lease;
 }
 async function packSupplier(seller){if(await acquire(seller.id))packingGroup.value=supplierPackingGroup(seller,report.value.groups)}
 async function packSource(group){if(await acquire(group.supplier?.id||0))packingGroup.value=group}
 function closePacking(){packingGroup.value=null}

 watch(()=>props.account,()=>{packingGroup.value=null;amountLine.value=null});
 const copyStates=ref({});
 async function copyTitle(g){const key=sourceStateKey(g);copyStates.value[key]='正在复制…';try{await copyText(g.title);copyStates.value[key]='已复制'}catch{copyStates.value[key]='复制失败，请手动选择标题复制'}}
 const expandedSellers=ref(new Set()),expandedSources=ref(new Set()),panelIds=new Map();let nextPanelId=0;
 const sourceStateKey=g=>`${props.account}:${g.key}`;
 function isCollapsed(g){return !expandedSources.value.has(sourceStateKey(g))}
 function toggleSource(g){const next=new Set(expandedSources.value),key=sourceStateKey(g);next.has(key)?next.delete(key):next.add(key);expandedSources.value=next}
 function isSellerExpanded(seller){return expandedSellers.value.has(seller.id)}
 function toggleSeller(seller){const next=new Set(expandedSellers.value);next.has(seller.id)?next.delete(seller.id):next.add(seller.id);expandedSellers.value=next}
 function sellerPanelId(seller){return `procurement-seller-${props.account}-${seller.id}`}
 function panelId(g){const key=sourceStateKey(g);if(!panelIds.has(key))panelIds.set(key,++nextPanelId);return `procurement-source-${props.account}-${panelIds.get(key)}`}
 function retainSourceState(groups){const sellers=new Set(groups.map(g=>g.supplier?.id||0));expandedSellers.value=new Set([...expandedSellers.value].filter(id=>sellers.has(id)));const keys=new Set(groups.map(sourceStateKey));expandedSources.value=new Set([...expandedSources.value].filter(key=>keys.has(key)));for(const key of panelIds.keys())if(!keys.has(key))panelIds.delete(key)}
 const bridgeReady=ref(''),bridgeError=ref(''),workbenchOrigin=location.origin;
 function bridgeState(){bridgeReady.value=document.documentElement.dataset.opProcurementBridgeReady||'';bridgeError.value=document.documentElement.dataset.opProcurementBridgeError||'';if(bridgeReady.value&&error.value.startsWith('采购助手未连接')){error.value='';openWithoutList.value=''}}
 onMounted(()=>{bridgeState();window.addEventListener('op-procurement-bridge-status',bridgeState);window.dispatchEvent(new CustomEvent('op-procurement-check-bridge'))});
 onUnmounted(()=>window.removeEventListener('op-procurement-bridge-status',bridgeState));
 function openSource(event,g){if(event.button!==0&&event.button!==1)return;if(!handoff(g))return;event.preventDefault();openWithoutList.value=g.url;bridgeState();error.value=bridgeError.value||`采购助手未连接 ${workbenchOrigin}。请在当前电脑点击 Chrome 的采购助手图标，保存并授权这个地址，看到“采购助手已连接”后再打开货源。`}
 const filtered=computed(()=>report.value.groups.filter(g=>JSON.stringify(g).toLowerCase().includes(query.value.toLowerCase())));
 const orderDetails=computed(()=>new Map(filtered.value.map(group=>{
  const rows=procurementOrderRows(group);
  let count=0;
  for(const row of rows){
   for(const order of row.orders) order.sequence=++count;
  }
  return [group.key,{rows,count}];
 })));
 const sellerGroups=computed(()=>{const groups=new Map();for(const g of filtered.value){const id=g.supplier?.id||0;if(!groups.has(id))groups.set(id,{id,name:g.supplier?.name||'未指定厂家',quantity:0,products:[]});const s=groups.get(id);s.quantity+=g.quantity;s.products.push(g)}const result=[...groups.values()];for(const seller of result)seller.products.sort((a,b)=>b.quantity-a.quantity||a.key.localeCompare(b.key));return result.sort((a,b)=>b.quantity-a.quantity||a.name.localeCompare(b.name)||a.id-b.id)});
 const heldOutsideList=computed(()=>{
  const visible=new Set(sellerGroups.value.map(seller=>seller.id));
  return Object.values(held.value).filter(lease=>!visible.has(lease.supplierId));
 });
 function canPack(id){return ready.value&&(!owners.value[id]||!!held.value[id])}
 async function load(refreshCache=false){const ticket=++serial,requestedGifts=gifts.value;loading.value=true;error.value='';try{if(!props.account){data.value={groups:[],quantity:0,unresolvedQuantity:0,packages:0};return}const d=await props.api(`/accounts/${props.account}/procurement?gifts=${requestedGifts?1:0}${refreshCache===true?'&refresh=1':''}`);if(ticket===serial){retainSourceState(d.groups);data.value=d;loadedAt.value=d.createdAt;loadedGifts.value=requestedGifts}}catch(e){if(ticket===serial)error.value=e.message}finally{if(ticket===serial)loading.value=false}}
 watch(()=>props.account,()=>{loadedAt.value='';loadedGifts.value=false;copyStates.value={};expandedSellers.value=new Set();expandedSources.value=new Set();panelIds.clear();nextPanelId=0;query.value='';dateField.value='orderedAt';clearDates();data.value={groups:[],quantity:0,unresolvedQuantity:0,packages:0};load()},{immediate:true});
 function handoff(g){const p=procurementHandoff(report.value.groups.filter(row=>(row.supplier?.id||0)===(g.supplier?.id||0)),g,props.account,loadedAt.value);return p?JSON.stringify({...p,gifts:loadedGifts.value}):null}
 return{orderDetails,report,dateField,dateStart,dateEnd,dateActive,clearDates,amountLine,packSupplier,packSource,startPurchase,closePacking,owners,held,pending,ready,release,canPack,heldOutsideList,packingGroup,isSellerExpanded,toggleSeller,sellerPanelId,loadedGifts,loadedAt,copyStates,copyTitle,sourceStateKey,isCollapsed,toggleSource,panelId,openSource,bridgeReady,bridgeError,openWithoutList,workbenchOrigin,handoff,gifts,query,filtered,sellerGroups,loading,error,load};
},template:`<section class="procurement-view"><div class="toolbar"><input v-model="query" placeholder="搜索货源、商品、规格或包裹号"><label><input type="checkbox" v-model="gifts" @change="load"> 包含赠品</label><button @click="load(true)" :disabled="loading">{{loading?'正在汇总…':'刷新采购汇总'}}</button>
 <div class="procurement-date-filter" role="group" aria-label="日期筛选">
  <div class="procurement-date-type">
   <select v-model="dateField" aria-label="筛选日期类型">
    <option value="orderedAt">下单日期</option>
    <option value="paidAt">付款日期</option>
   </select>
  </div>
  <div class="procurement-date-range">
   <ProcurementDateField label="开始日期" v-model="dateStart" :max="dateEnd"/>
   <span class="procurement-date-separator" aria-hidden="true">—</span>
   <ProcurementDateField label="结束日期" v-model="dateEnd" :min="dateStart"/>
  </div>
  <strong class="procurement-total">需求件数 {{report.quantity}}</strong>
  <span class="procurement-total">待采购包裹 {{report.packages}}</span>
  <span class="procurement-total">货源待确认件数 {{report.unresolvedQuantity}}</span>
  <button v-if="dateActive" type="button" class="text-button" @click="clearDates">清除日期</button>
 </div>
 </div><p v-if="dateStart&&dateEnd&&dateStart>dateEnd" class="error" role="status">开始日期不能晚于结束日期。</p><details v-if="report.pendingRefundLines?.length" class="error"><summary>{{report.pendingRefundLines.length}} 条商品存在退款信息，但接口未提供退款件数，暂未计入下方需求；请网页采集待处理订单后刷新汇总。</summary><p v-for="line in report.pendingRefundLines" :key="line.packageId+':'+line.itemId">包裹 {{line.packageNo||line.packageId}} · {{line.title}} · 订单数量 {{line.quantity}}</p></details><PackingJobs :key="account" :account="account" :api="api"/><p class="scope-note">卖家从历史订单提取，在订单记录中指定货源与卖家的关系；一个卖家可以对应多个货源产品。同一货源跨店铺合并，规格保留颜色和尺码；请采购前核对货源规格，系统尚无 1688 SKU 映射。</p><div v-if="error" class="error">{{error}} <a v-if="openWithoutList" :href="openWithoutList" target="_blank" rel="noopener noreferrer">仅打开货源（不带采购清单）</a></div><div v-for="lease in heldOutsideList" :key="lease.supplierId" class="supplier-held-outside"><strong>{{lease.supplierName||(lease.supplierId?'厂家 #'+lease.supplierId:'未指定厂家')}}</strong><span class="supplier-operator">{{lease.username}} 正在采购</span><button type="button" class="text-button" :disabled="pending[lease.supplierId]" @click="release(lease.supplierId)">结束采购</button></div><section v-for="(seller,sellerIndex) in sellerGroups" :key="seller.id" class="supplier-group"><h2 class="supplier-heading" :class="{'is-purchasing': owners[seller.id]}"><button type="button" class="supplier-heading-toggle" :aria-expanded="isSellerExpanded(seller)" :aria-controls="sellerPanelId(seller)" @click="toggleSeller(seller)"><span class="supplier-heading-number">{{sellerIndex+1}}.</span><span class="supplier-heading-label">厂家</span><span class="supplier-heading-name">{{seller.name}}</span><small>{{seller.products.length}} 个货源 · {{seller.quantity}} 件</small><span class="supplier-toggle-label"><span aria-hidden="true">{{isSellerExpanded(seller)?'▾':'▸'}}</span> {{isSellerExpanded(seller)?'收起':'展开'}}</span></button><small v-if="owners[seller.id]" class="supplier-operator">{{owners[seller.id]}} 正在采购</small><button v-if="held[seller.id]" type="button" class="text-button" :disabled="pending[seller.id]" @click="release(seller.id)">结束采购</button><button v-if="seller.id&&canPack(seller.id)" type="button" :disabled="pending[seller.id]" class="supplier-packing-button" :aria-label="seller.name+' 提交代打包'" @click="packSupplier(seller)">提交代打包</button></h2><div class="supplier-sources" :id="sellerPanelId(seller)" v-show="isSellerExpanded(seller)"><article v-for="g in seller.products" :key="g.key" :class="['purchase-card',{'is-collapsed':isCollapsed(g)}]"><header><div><strong>{{g.reason?'待确认货源':(g.source.source+' 货源商品 '+g.source.sourceItemId)}}</strong><small>{{g.reason||(g.supplier?.name||'未指定厂家')}}</small></div><b>共 <span class="source-quantity">{{g.quantity}}</span> 件 · <span class="source-quantity">{{orderDetails.get(g.key).count}}</span> 个订单</b><a v-if="g.url&&held[seller.id]" :href="g.url" :data-supplier-id="seller.id" :data-supplier-lease="held[seller.id].leaseId" :data-op-procurement="handoff(g)" @click="openSource($event,g)" @auxclick="openSource($event,g)" target="_blank" rel="noopener noreferrer">打开货源 ↗</a><button v-else-if="g.url&&ready&&!owners[seller.id]" type="button" class="text-button" :disabled="pending[seller.id]" @click="startPurchase(seller.id)">开始货源采购</button><button v-if="!g.reason&&canPack(seller.id)" type="button" :disabled="pending[seller.id]" @click="packSource(g)">提交代打包</button><button type="button" class="source-collapse-toggle" :aria-expanded="!isCollapsed(g)" :aria-controls="panelId(g)" :aria-label="(isCollapsed(g)?'展开':'收起')+'货源 '+(g.source.sourceItemId||g.title||'待确认货源')" @click="toggleSource(g)"><span aria-hidden="true">{{isCollapsed(g)?'▸':'▾'}}</span>{{isCollapsed(g)?'展开':'收起'}}</button></header><div :id="panelId(g)" class="purchase-card-content" v-show="!isCollapsed(g)"><div class="purchase-product"><ProductImage :item="g.image"/><div class="purchase-product-info"><div class="purchase-title-row"><strong>{{g.title||'未命名商品'}}</strong><button type="button" class="copy-product-title" :disabled="!g.title||copyStates[sourceStateKey(g)]==='正在复制…'" title="复制完整商品标题" aria-label="复制完整商品标题" @click.stop="copyTitle(g)">复制</button></div><span v-if="copyStates[sourceStateKey(g)]" class="copy-title-feedback" role="status">{{copyStates[sourceStateKey(g)]}}</span><details><summary>关联店铺产品（{{g.products.length}}）</summary><p v-for="p in g.products" :key="p">{{p}}</p></details></div></div><table><thead><tr><th>颜色 / 尺码</th><th>采购需求</th><th>订单来源</th></tr></thead><tbody><tr v-for="row in orderDetails.get(g.key).rows" :key="row.key"><td><div v-for="v in row.specs" :key="v.spec" class="purchase-spec"><div v-for="sku in v.skus" :key="sku.itemId" class="procurement-sku"><ProductImage :key="sku.itemId" :item="sku.image||{}"/><strong>{{v.spec}}</strong><small v-if="sku.skuId">SKU {{sku.skuId}}</small></div><small v-if="row.specs.length>1">{{v.quantity}} 件</small></div></td><td><b>{{row.quantity}} 件</b></td><td><div class="purchase-order-shortcuts"><ProcurementOrderLine v-for="order in row.orders" :key="order.key" :order="order" @price="amountLine=$event"/></div></td></tr></tbody></table></div></article></div></section><div v-if="!filtered.length&&!loading&&!error" class="empty">{{dateActive?'所选日期内没有符合条件的采购需求。':'暂无符合条件的采购需求。请先同步待处理订单和店铺产品。'}}</div><OrderAmountDialog v-if="amountLine" :line="amountLine" :account="account" :api="api" @close="amountLine=null"/><BatchPackingDialog v-if="packingGroup" :group="packingGroup" :account="account" :api="api" :gifts="loadedGifts" :lease-id="held[packingGroup.supplier?.id||0]?.leaseId" @close="closePacking"/></section>`};
