import draggableTaskTools from './draggableTaskTools.js';
import {ref,computed,watch,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';
import ProductImage from './ProductImage.js';
import PackingInformation from './PackingInformation.js';
import PackingRerunDialog from './PackingRerunDialog.js';
export default {
 directives:{draggableTaskTools,modal:{mounted(el){el.showModal()}}},
 components:{PackingRerunDialog,ProductImage,PackingInformation},
 props:{account:{},api:{},standalone:Boolean},
 setup(props){
  const editJob=ref(null),jobs=ref([]),detail=ref(null),selected=ref(null),error=ref(''),busy=ref(false),expanded=ref(props.standalone),syncMessage=ref(''),query=ref(''),status=ref(''),loading=ref(false);let timer,serial=0,polling=false;
  const labels={review:'需要核对',queued:'等待执行',running:'正在提交',paused:'已暂停',completed:'已完成',completed_errors:'完成，有失败项',cancelled:'已取消',pending:'待提交',failed:'失败',unknown:'结果待核对',not_sent:'未发出',partial:'部分成功',skipped:'已跳过'};
  const active=computed(()=>jobs.value.filter(j=>j.state==='queued'||j.state==='running'));
  const statusOptions=['review','queued','running','paused','completed','completed_errors','cancelled'];
  function metricGroup(job){
   if(job.state==='completed_errors'||['failed','not_sent','unknown','partial'].some(state=>job.counts[state]>0))return 'review';
   if(['queued','running'].includes(job.state))return 'active';
   return job.state;
  }
  const totals=computed(()=>{
   const counts={active:0,review:0,completed:0,paused:0,cancelled:0};
   for(const job of jobs.value)counts[metricGroup(job)]++;
   return counts;
  });
  const needsAttention=computed(()=>jobs.value.filter(job=>metricGroup(job)==='review'));
  const filtered=computed(()=>{
   const list=status.value==='review'?needsAttention.value:jobs.value.filter(j=>!status.value||j.state===status.value);
   const search=query.value.trim().toLowerCase();
   return list.filter(j=>[j.id,j.name].join(' ').toLowerCase().includes(search));
  });
  function showReview(){
   status.value='review';query.value='';
   if(needsAttention.value.length===1)open(needsAttention.value[0].id);
  }
  const finished=job=>(job.counts.completed||0)+(job.counts.skipped||0);
  const formatTime=value=>value?new Date(value).toLocaleString():'—';
  async function load(){if(polling||!props.account)return;polling=true;loading.value=true;const account=props.account,ticket=serial;try{const list=await props.api(`/accounts/${account}/packing-jobs`);if(ticket!==serial)return;jobs.value=list;error.value='';if(selected.value){const id=selected.value;const d=await props.api(`/accounts/${account}/packing-jobs/${id}`);if(ticket===serial&&selected.value===id)detail.value=d}}catch(e){if(ticket===serial)error.value=e.message}finally{polling=false;loading.value=false}}
  const selectedJob=computed(()=>jobs.value.find(job=>job.id===selected.value));
  const itemStatus=ref('');
  const itemStates=['pending','running','completed','failed','not_sent','unknown','partial','skipped','cancelled'];
  const visibleItems=computed(()=>{
   const items=detail.value?.items||[];
   return itemStatus.value?items.filter(item=>item.state===itemStatus.value):items;
  });
  watch(selected,()=>{itemStatus.value=''});
  async function open(id){selected.value=id;detail.value=null;await load()}
  async function action(job,action,itemId){if(busy.value)return;busy.value=true;error.value='';try{await props.api(`/accounts/${props.account}/packing-jobs/${job}/${action}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({itemId})});await load()}catch(e){error.value=e.message}finally{busy.value=false}}
  async function edit(id){busy.value=true;error.value='';const account=props.account;try{const job=await props.api(`/accounts/${account}/packing-jobs/${id}/rerun-preview`);if(account===props.account)editJob.value=job}catch(e){error.value=e.message}finally{busy.value=false}}
  async function created(id){selected.value=id||null;detail.value=null;expanded.value=true;status.value='';query.value='';await load()}
  async function sync(){if(busy.value||active.value.length||!window.confirm('确认通过妙手网页同步当前账户的待处理订单？'))return;busy.value=true;error.value='';try{await props.api(`/accounts/${props.account}/sync-pending`,{method:'POST'});syncMessage.value='已开始网页采集。请在订单记录中查看采集进度，完成后刷新采购汇总。'}catch(e){error.value=e.message}finally{busy.value=false}}
  watch(()=>props.account,()=>{serial++;editJob.value=null;jobs.value=[];detail.value=null;selected.value=null;error.value='';syncMessage.value='';query.value='';status.value='';load()},{immediate:true});
  onMounted(()=>{timer=setInterval(load,3000)});onUnmounted(()=>{serial++;clearInterval(timer)});
  return{itemStatus,itemStates,visibleItems,showReview,selectedJob,editJob,edit,created,standalone:props.standalone,query,status,statusOptions,needsAttention,totals,filtered,finished,formatTime,loading,jobs,detail,selected,error,busy,expanded,labels,active,load,open,action,sync,syncMessage};
 },template:`<div v-if="!standalone" v-draggable-task-tools class="procurement-task-tools" title="拖动可移动按钮组">
 <button type="button" :aria-expanded="expanded" @click="expanded=true;load()">代打包任务（{{jobs.length}}）</button>
 <button type="button" class="secondary" :disabled="busy||active.length>0" @click="sync">网页同步待处理订单</button>
 <span v-if="!expanded && (syncMessage||error)" role="status">{{error||syncMessage}}</span>
 </div>
 <section v-if="standalone||expanded" :class="['packing-jobs','task-management',{'procurement-task-drawer':!standalone}]" aria-label="代打包任务">
 <button v-if="!standalone" type="button" class="secondary drawer-close" @click="expanded=false">关闭任务面板</button><div class="task-metrics"><div><b>{{jobs.length}}</b><span>全部任务</span></div><div><b>{{totals.active}}</b><span>排队 / 执行中</span></div><button type="button" class="review-metric" :aria-pressed="status==='review'" @click="showReview"><b>{{needsAttention.length}}</b><span>需要核对</span></button><div><b>{{totals.completed}}</b><span>已完成</span></div><div v-if="totals.paused"><b>{{totals.paused}}</b><span>已暂停</span></div><div v-if="totals.cancelled"><b>{{totals.cancelled}}</b><span>已取消</span></div></div><header><strong>代打包任务</strong><div class="actions"><a v-if="standalone" href="#/procurement">去采购汇总创建任务 ↗</a><button class="secondary" type="button" :disabled="loading" @click="load">{{loading?'正在刷新…':'刷新任务'}}</button><button v-if="standalone" class="secondary" type="button" :disabled="busy||active.length>0" @click="sync">网页同步待处理订单</button></div></header><div class="toolbar task-filters"><label>搜索任务<input v-model="query" placeholder="任务名称或编号"></label><label>任务状态<select v-model="status" class="task-status-select"><option value="">全部状态</option><option v-for="s in statusOptions" :key="s" :value="s">{{labels[s]}}</option></select></label><span class="result-count">{{filtered.length}} 个任务 · 每 3 秒更新</span></div><p v-if="syncMessage" class="packing-help">{{syncMessage}}</p><div v-if="error" class="error" role="alert">{{error}}</div><div><p class="packing-help">所有任务串行执行。暂停或取消会等待当前包裹返回；任务完成后同步妙手订单，再刷新采购汇总。</p><div v-if="!jobs.length && status!=='review'" class="empty">{{loading?'正在读取任务…':'暂无代打包任务。从采购汇总的货源旁点击“提交代打包”，选择订单后保存。'}}</div><p v-else-if="!filtered.length" class="empty">{{status==='review'&&!needsAttention.length?'当前没有需要核对的任务。':'没有符合筛选条件的任务。'}}</p><article v-for="job in filtered" :key="job.id" class="packing-job"><header :class="{'has-packing-info':job.packingInfos?.length}"><div><strong>#{{job.id}} {{job.name}}</strong><span :class="['task-state',job.state]">{{labels[job.state]}}</span><small>{{labels[job.state]}} · 成功 {{job.counts.completed||0}} / {{job.total}} 个包裹 · 剩余 {{job.counts.pending||0}} · 间隔 {{job.intervalSeconds}} 秒</small><small>创建 {{formatTime(job.createdAt)}} · 更新 {{formatTime(job.updatedAt)}}</small></div><PackingInformation :infos="job.packingInfos||[]"/><div class="actions"><button v-if="job.state==='completed'" type="button" :disabled="busy" @click="edit(job.id)">修改并重新执行</button><button type="button" class="secondary" @click="open(job.id)">查看明细</button><button v-if="['queued','running'].includes(job.state)" type="button" class="secondary" :disabled="busy" @click="action(job.id,'pause')">暂停</button><button v-if="job.state==='paused'&&(job.counts.pending||0)>0" type="button" :disabled="busy||!!job.counts.running" @click="action(job.id,'resume')">继续剩余包裹</button><button v-if="['queued','running','paused'].includes(job.state)" type="button" class="secondary" :disabled="busy" @click="action(job.id,'cancel')">取消剩余包裹</button></div></header>
 <div class="task-progress"><progress :value="finished(job)" :max="job.total||1" :aria-label="job.name+' 完成进度'"></progress><span>成功 {{job.counts.completed||0}} · 跳过 {{job.counts.skipped||0}} · 异常 {{(job.counts.failed||0)+(job.counts.not_sent||0)+(job.counts.unknown||0)+(job.counts.partial||0)}} · 共 {{job.total}} 包裹</span></div>
 </article></div>
 <Teleport to="body"><dialog v-if="selectedJob" v-modal class="task-detail-dialog" aria-labelledby="task-detail-title" @close="selected=null;detail=null" @click.self="$event.currentTarget.close()">
 <header><strong id="task-detail-title">任务明细 · #{{selectedJob.id}} {{selectedJob.name}}</strong><label class="task-item-filter">子任务状态<select v-model="itemStatus" class="task-status-select"><option value="">全部状态</option><option v-for="state in itemStates" :key="state" :value="state">{{labels[state]}}（{{detail?.counts?.[state]||0}}）</option></select></label><button type="button" class="secondary" autofocus @click="$event.currentTarget.closest('dialog').close()">关闭</button></header>
 <p v-if="error" class="error" role="alert">{{error}}</p><div class="packing-job-detail"><p v-if="!detail">正在读取…</p><template v-else><p v-if="!visibleItems.length" class="empty">当前没有符合条件的包裹。</p><section v-for="item in visibleItems" :key="item.id"><header><strong>{{item.lines[0]?.packageNo||item.request.packageId}}</strong><span :class="['task-item-state',item.state]">{{labels[item.state]}}</span></header><div v-for="line in item.lines" :key="line.itemId" class="task-sku-line"><ProductImage :item="line.image||{}"/><div><strong>{{line.spec||'未提供规格'}}</strong><span v-if="line.gift" class="status-pill">赠品</span><p>{{line.quantity}} 件 · SKU {{line.sku||'—'}}</p><p class="task-order-number">订单号：{{line.orderNo||'未提供'}}</p></div></div><p v-for="(reference,n) in [...new Set(item.request.items.map(i=>i.purchase?'采购单 '+i.purchase.orderNumber:'快递单 '+(i.waybills||[]).map(w=>w.company+' '+w.number).join('、')))]" :key="n">{{reference}}</p><template v-if="item.state!=='completed' && item.result"><p v-if="item.result.message"><strong>{{['running','pending','skipped'].includes(item.state)?'上次异常原因':'异常原因'}}</strong> · {{item.result.message}}</p><pre v-if="item.result.details?.length">{{JSON.stringify(item.result.details,null,2)}}</pre><small v-if="item.result.archive">本地记录：{{item.result.archive}}</small></template><div v-if="['failed','not_sent','unknown','partial','skipped'].includes(item.state) && (item.state!=='skipped'||selectedJob.state!=='cancelled')" class="actions"><button v-if="selectedJob.state!=='cancelled'" type="button" :disabled="busy" @click="editJob={...detail,items:[item],retryItemId:item.id}">修改并重新排队</button><button v-if="selectedJob.state!=='cancelled'" type="button" class="secondary" :disabled="busy" @click="action(selectedJob.id,'retry',item.id)">{{['unknown','partial'].includes(item.state)?'已核对未提交，重新排队':'重新排队'}}</button><button v-if="['unknown','partial'].includes(item.state)" type="button" class="secondary" :disabled="busy" @click="action(selectedJob.id,'confirm-completed',item.id)">已核对全部成功</button><button type="button" v-if="item.state!=='skipped'" class="text-button" :disabled="busy" @click="action(selectedJob.id,'skip',item.id)">跳过该包裹</button></div></section></template></div></dialog></Teleport><PackingRerunDialog v-if="editJob" :job="editJob" :account="account" :api="api" @close="editJob=null" @created="created"/></section>`
};
