import {createApp,ref,computed,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';
import TasksView from './views/TasksView.js';
import APIManagementView from './views/APIManagementView.js';
import OpenAPISync from './components/OpenAPISync.js';
import OrdersView from './views/OrdersView.js';
import AccountsView from './views/AccountsView.js';
import ArchivesView from './views/ArchivesView.js';
import ProcurementView from './views/ProcurementView.js';
import SuppliersView from './views/SuppliersView.js';
import ProductsView from './views/ProductsView.js';
import './style.css';
import './controls.css';
createApp({components:{TasksView,APIManagementView,OpenAPISync,OrdersView,AccountsView,ArchivesView,ProcurementView,ProductsView,SuppliersView},setup(){
 const menu=[{id:'suppliers',label:'采购卖家',icon:'♧',description:'订单中提取的卖家名录与已指定货源'},{id:'products',label:'店铺产品',icon:'▧',description:'同步产品与货源，关联订单中的平台产品 ID'},{id:'procurement',label:'采购汇总',icon:'▦',description:'从待采购订单按货源、产品和规格汇总件数'},{id:'tasks',label:'任务管理',icon:'☷',description:'查看代打包任务进度，管理执行队列和异常包裹'},{id:'orders',label:'订单记录',icon:'▤',description:'按包裹查看商品、规格及物流信息'},{id:'accounts',label:'账户管理',icon:'▣',description:'管理妙手登录和账户连接'},{id:'api',label:'API 管理',icon:'⚙',description:'保存 API 凭证，选择当前账户使用的 Key'},{id:'archives',label:'原始响应',icon:'▥',description:'查看已留存的接口响应记录'}];
 const route=ref('orders'),accounts=ref([]),selected=ref(''),name=ref(''),error=ref(''),busy=ref(''),states=ref({}),archives=ref([]),orders=ref([]),total=ref(0),page=ref(1),query=ref(''),orderStatus=ref('waitProcess'),purchaseTab=ref('waitPurchase'),orderCounts=ref({}),loading=ref(false);let timer;let serial=0;let refreshing=false;
 const current=computed(()=>menu.find(m=>m.id===route.value));
 const labels={closed:'浏览器已关闭',opening:'正在打开浏览器',waiting_login:'等待登录 / 验证',verified:'订单读取已验证',archive_error:'原始响应保存失败'};
 async function api(url,options){const r=await fetch('/api'+url,options);if(!(r.headers.get('content-type')||'').includes('application/json'))throw new Error('后端需要更新：请停止旧服务后重新运行 go run .');const d=await r.json();if(!r.ok)throw Object.assign(new Error(d.error||'请求失败'),{status:r.status});return d}
 async function loadOrders(){if(!selected.value)return;const ticket=++serial,id=selected.value;loading.value=true;try{const d=await api(`/accounts/${id}/orders?page=${page.value}&status=${encodeURIComponent(orderStatus.value)}&purchaseTab=${orderStatus.value==='waitProcess'?purchaseTab.value:''}&q=${encodeURIComponent(query.value)}`);if(d.status!==orderStatus.value||!d.counts||d.purchaseTab!==(orderStatus.value==='waitProcess'?purchaseTab.value:'')){if(ticket===serial)throw new Error('订单状态筛选需要更新后端：请重启 go run .');return}if(ticket===serial&&id===selected.value){orders.value=d.items;total.value=d.total;orderCounts.value=d.counts||{}}}finally{if(ticket===serial)loading.value=false}}
 async function refresh(){if(refreshing)return;refreshing=true;try{accounts.value=await api('/accounts');if(!selected.value&&accounts.value.length)selected.value=accounts.value[0].id;if(route.value==='orders'){await loadOrders();if(selected.value)states.value[selected.value]=await api(`/accounts/${selected.value}/status`);}else if(route.value==='products'&&selected.value){const id=selected.value;states.value[id]=await api(`/accounts/${id}/status`)}else if(route.value==='archives'&&selected.value){const id=selected.value;const result=await api(`/accounts/${id}/archives`);if(id===selected.value)archives.value=result}else if(route.value==='accounts'){await Promise.all(accounts.value.map(async a=>{states.value[a.id]=await api(`/accounts/${a.id}/status`)}))}error.value=''}catch(e){error.value=e.message}finally{refreshing=false}}
 // The order list is read only on entry, account changes, manual search or pagination.
 // While a sync job runs, poll its progress without querying or replacing list data.
 let pollingStatus=false;
 async function poll(){
  if(route.value!=='orders'){await refresh();return}
  const id=selected.value;
  if(!id||states.value[id]?.sync?.state!=='running'||pollingStatus)return;
  pollingStatus=true;
  try{const status=await api(`/accounts/${id}/status`);if(id===selected.value)states.value[id]=status}
  catch(e){if(id===selected.value)error.value=e.message}
  finally{pollingStatus=false}
 }
 function readRoute(){const id=location.hash.replace('#/','');route.value=menu.some(m=>m.id===id)?id:'orders';refresh()}
 function navigate(id){location.hash='/'+id;if(route.value===id)refresh()}
 async function selectAccount(){++serial;orders.value=[];archives.value=[];total.value=0;page.value=1;query.value='';orderStatus.value='waitProcess';purchaseTab.value='waitPurchase';orderCounts.value={};await refresh()}
 async function select(a,view){selected.value=a.id;await selectAccount();navigate(view)}
 async function add(){busy.value='add';error.value='';try{await api('/accounts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name.value})});name.value='';await refresh()}catch(e){error.value=e.message}finally{busy.value=''}}
 async function action(a,kind){busy.value=a.id;error.value='';try{states.value[a.id]=await api(`/accounts/${a.id}/${kind}`,{method:'POST'})}catch(e){error.value=e.message}finally{busy.value=''}}
 async function search(){page.value=1;try{await loadOrders();error.value=''}catch(e){error.value=e.message}}
 async function filterOrders(status,purchase='waitPurchase'){orderStatus.value=status;purchaseTab.value=purchase;page.value=1;orders.value=[];total.value=0;try{await loadOrders();error.value=''}catch(e){error.value=e.message}}
 async function turn(n){page.value+=n;try{await loadOrders()}catch(e){error.value=e.message}}
 onMounted(()=>{readRoute();window.addEventListener('hashchange',readRoute);timer=setInterval(poll,5000)});onUnmounted(()=>{clearInterval(timer);window.removeEventListener('hashchange',readRoute)});
 return{api,menu,route,current,accounts,selected,name,error,busy,states,archives,orders,total,page,query,orderStatus,purchaseTab,orderCounts,filterOrders,loading,labels,navigate,selectAccount,select,add,action,search,turn,refresh};
},template:`<div class="app-shell"><aside class="sidebar"><div class="brand"><div class="brand-mark">OP</div><div><strong>订单工作台</strong><small>Order Processor</small></div></div><div class="nav-caption">工作空间</div><nav aria-label="主菜单"><a v-for="m in menu" :key="m.id" :href="'#/'+m.id" :class="{active:route===m.id}" :aria-current="route===m.id?'page':undefined"><span class="nav-icon" aria-hidden="true">{{m.icon}}</span>{{m.label}}<span v-if="route===m.id" class="nav-indicator"></span></a></nav><div class="sidebar-footer"><span class="dot"></span>公司内部 · 本地部署</div></aside>
<div class="workspace" :class="{'orders-workspace':route==='orders'}"><header class="topbar"><div class="breadcrumb">工作空间 <span>/</span> <b>{{current.label}}</b></div><div class="account-picker"><label for="account-select">当前账户</label><select id="account-select" v-model="selected" @change="selectAccount"><option v-if="!accounts.length" value="">暂无账户</option><option v-for="a in accounts" :key="a.id" :value="a.id">{{a.name}}</option></select></div></header><main><div class="page-heading"><div><h1>{{current.label}}</h1><p v-if="route!=='orders'">{{current.description}}</p></div><div v-if="route==='orders'" class="order-heading-actions"><button :disabled="!selected||busy||states[selected]?.sync?.state==='running'" @click="action({id:selected},'sync')">网页采集全部订单</button><button :disabled="!selected||busy||states[selected]?.sync?.state==='running'" @click="action({id:selected},'sync-pending')">网页采集待处理订单</button><button v-if="states[selected]?.sync?.state==='running'" class="secondary" @click="action({id:selected},'stop-sync')" :disabled="busy">停止</button></div><button v-if="route!=='orders'&&route!=='tasks'" class="secondary" @click="refresh" :disabled="loading">刷新本地数据</button></div><div v-if="error" class="error" role="alert">{{error}}</div>
<details v-if="route==='orders'" class="order-sync-details"><summary>同步详情<span v-if="states[selected]?.sync"> · {{({running:'采集中',completed:'已完成',failed:'失败',cancelled:'已停止'})[states[selected].sync.state]}} · 第 {{states[selected].sync.page}} / {{states[selected].sync.pages}} 页</span><span v-if="states[selected]?.sync?.error" class="failure"> · {{states[selected].sync.error}}</span></summary><div class="order-sync-content"><p>全部订单包含历史各状态；待处理包含未出货和已出货。每页 200 条采集，间隔 3～5 秒。</p><p v-if="states[selected]?.sync">{{states[selected].sync.scope==='products'?'店铺产品':states[selected].sync.scope==='pending'?'待处理订单':'全部订单'}} · {{states[selected].sync.group||'准备中'}} · {{({running:'采集中',completed:'采集完成，入库后台处理中',failed:'采集中断',cancelled:'已停止'})[states[selected].sync.state]}} · 第 {{states[selected].sync.page}} / {{states[selected].sync.pages}} 页 · 当前分组 {{states[selected].sync.total}} 个包裹 · 累计采集 {{states[selected].sync.collected}} 个包裹</p><p v-if="states[selected]?.sync?.notice">{{states[selected].sync.notice}}</p><p v-if="states[selected]?.sync?.pageSize">采集每页 {{states[selected].sync.pageSize}} 条 · 第 {{states[selected].sync.pass||1}} 轮</p><p v-if="states[selected]?.sync?.step">当前步骤：{{states[selected].sync.step}}</p><p v-if="states[selected]?.sync?.error" class="failure">{{states[selected].sync.error}}</p><details v-if="states[selected]?.sync?.errorDetail"><summary>查看失败详情</summary><pre class="sync-error-detail">{{states[selected].sync.errorDetail}}</pre></details></div></details>
<OpenAPISync v-if="route==='orders'&&selected" :key="selected" :account="selected" :api="api" @completed="refresh"/>
<OrdersView v-if="route==='orders'" :account="selected" :api="api" :status="orderStatus" :purchase-tab="purchaseTab" :counts="orderCounts" @filter="filterOrders" :orders="orders" :total="total" :page="page" :loading="loading" v-model:query="query" @search="search" @turn="turn" @submitted="refresh"/>
<TasksView v-else-if="route==='tasks'" :key="selected" :account="selected" :api="api"/>
<APIManagementView v-else-if="route==='api'" :key="selected" :account="selected" :api="api"/>
<AccountsView v-else-if="route==='accounts'" :accounts="accounts" :states="states" :busy="busy" :labels="labels" v-model:name="name" @add="add" @action="action" @select="select"/>
<ProductsView v-else-if="route==='products'" :account="selected" :api="api" :state="states[selected]" :busy="busy" @action="action"/><ProcurementView v-else-if="route==='procurement'" :account="selected" :api="api"/><SuppliersView v-else-if="route==='suppliers'" :account="selected" :api="api"/><ArchivesView v-else :archives="archives"/>
</main></div></div>`}).mount('#app');
