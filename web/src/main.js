import SessionGate from './components/SessionGate.js';
import UsersView from './views/UsersView.js';
import OrderSyncNotifications from './components/OrderSyncNotifications.js';
import {createApp,ref,shallowRef,provide,computed,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';
import ShipmentStatsView from './views/ShipmentStatsView.js';
import TasksView from './views/TasksView.js';
import OrdersView from './views/OrdersView.js';
import AccountsView from './views/AccountsView.js';
import ArchivesView from './views/ArchivesView.js';
import ProcurementView from './views/ProcurementView.js';
import SuppliersView from './views/SuppliersView.js';
import ProductsView from './views/ProductsView.js';
import './style.css';
import './controls.css';
import './session.css';
const Workspace={props:['user','api','sessionBusy'],emits:['logout'],components:{UsersView,OrderSyncNotifications,ShipmentStatsView,TasksView,OrdersView,AccountsView,ArchivesView,ProcurementView,ProductsView,SuppliersView},setup(props){
 provide('order-notice-host',shallowRef(null));
 const api=props.api;
 const menuItems=[{id:'suppliers',label:'采购卖家',icon:'♧'},{id:'products',label:'店铺产品',icon:'▧'},{id:'procurement',label:'采购汇总',icon:'▦'},{id:'shipment-stats',label:'销售数据分析',icon:'▥'},{id:'tasks',label:'任务管理',icon:'☷'},{id:'orders',label:'订单记录',icon:'▤'},{id:'accounts',label:'账户管理',icon:'▣',admin:true},{id:'users',label:'用户管理',icon:'♙',admin:true},{id:'archives',label:'原始响应',icon:'▥',admin:true}];
 const menu=computed(()=>menuItems.filter(m=>!m.admin||props.user.isAdmin));
 const route=ref('orders'),accounts=ref([]),selected=ref(''),name=ref(''),error=ref(''),busy=ref(''),states=ref({}),archives=ref([]),orders=ref([]),total=ref(0),page=ref(1),query=ref(''),orderStatus=ref('waitProcess'),purchaseTab=ref('waitPurchase'),orderCounts=ref({}),loading=ref(false);let timer;let serial=0;let refreshVersion=0,accountsVersion=0;
 const current=computed(()=>menu.value.find(m=>m.id===route.value));
 const labels={unavailable:'浏览器服务不可用',closed:'浏览器已关闭',opening:'正在打开浏览器',waiting_login:'等待登录 / 验证',verified:'订单读取已验证',archive_error:'原始响应保存失败'};
 async function loadOrders(){if(!selected.value)return;const ticket=++serial,id=selected.value;loading.value=true;try{const d=await api(`/accounts/${id}/orders?page=${page.value}&status=${encodeURIComponent(orderStatus.value)}&purchaseTab=${orderStatus.value==='waitProcess'?purchaseTab.value:''}&q=${encodeURIComponent(query.value)}`);if(d.status!==orderStatus.value||!d.counts||d.purchaseTab!==(orderStatus.value==='waitProcess'?purchaseTab.value:'')){if(ticket===serial)throw new Error('订单状态筛选需要更新后端：请重启 go run .');return}if(ticket===serial&&id===selected.value){orders.value=d.items;total.value=d.total;orderCounts.value=d.counts||{}}}finally{if(ticket===serial)loading.value=false}}
 async function ordersChanged(account){
  if(route.value!=='orders'||account!==selected.value)return;
  try{await loadOrders()}catch(e){if(account===selected.value)error.value=e.message}
 }
 function resetAccountData(){
  ++serial;orders.value=[];archives.value=[];total.value=0;page.value=1;query.value='';orderStatus.value='waitProcess';purchaseTab.value='waitPurchase';orderCounts.value={};loading.value=false;
 }
 async function loadAccounts(){
  const ticket=++accountsVersion;
  const list=await api('/accounts');
  if(ticket!==accountsVersion)return false;
  const allowed=new Set(list.map(a=>a.id));
  accounts.value=list;
  for(const id of Object.keys(states.value))if(!allowed.has(id))delete states.value[id];
  if(!allowed.has(selected.value)){
   const next=list[0]?.id||'';
   if(next!==selected.value){resetAccountData();selected.value=next;return true}
  }
  return false;
 }
 async function refresh(){
  const version=++refreshVersion;
  try{
   await loadAccounts();
   if(version!==refreshVersion)return;
   const id=selected.value;
   if(route.value==='orders'||route.value==='products'){
    if(!id)return;
    if(route.value==='orders')await loadOrders();
    const data=await api(`/accounts/${id}/status`);
    if(version===refreshVersion&&id===selected.value)states.value[id]=data;
   }else if(route.value==='archives'&&id){
    const result=await api(`/accounts/${id}/archives`);
    if(version===refreshVersion&&id===selected.value)archives.value=result;
   }else if(route.value==='accounts'){
    await Promise.all(accounts.value.map(async a=>{
     let state;
     try{state=await api(`/accounts/${a.id}/status`)}
     catch(e){if(!e.browserMode)throw e;state={browserMode:e.browserMode,state:'unavailable',error:e.message}}
     if(version===refreshVersion)states.value[a.id]=state;
    }));
   }
   if(version===refreshVersion)error.value='';
  }catch(e){if(version===refreshVersion)error.value=e.message}
 }
 let pollingStatus=false;
 async function poll(){
  if(route.value!=='orders'){await refresh();return}
  if(pollingStatus)return;
  pollingStatus=true;
  try{
   const changed=await loadAccounts();
   const id=selected.value;
   if(!id)return;
   if(changed)await refresh();
   else if(states.value[id]?.sync?.state==='running'){
    const status=await api(`/accounts/${id}/status`);if(id===selected.value)states.value[id]=status;
   }
  }catch(e){error.value=e.message}finally{pollingStatus=false}
 }
 function readRoute(){const id=location.hash.replace('#/','');route.value=menu.value.some(m=>m.id===id)?id:'orders';if(id&&route.value!==id)history.replaceState(null,'','#/orders');refresh()}
 function navigate(id){location.hash='/'+id;if(route.value===id)refresh()}
 async function selectAccount(){resetAccountData();await refresh()}
 async function select(a,view){selected.value=a.id;await selectAccount();navigate(view)}
 async function add(){busy.value='add';error.value='';try{await api('/accounts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name.value})});name.value='';await refresh()}catch(e){error.value=e.message}finally{busy.value=''}}
 async function removeAccount(a){
  if(busy.value||!window.confirm('删除妙手账户「'+a.name+'」？分配给用户的访问权限也会移除。'))return;
  busy.value=a.id;error.value='';
  try{await api('/accounts/'+a.id,{method:'DELETE'});await refresh()}catch(e){error.value=e.message}finally{busy.value=''}
 }
 async function action(a,kind,onResult){busy.value=a.id;error.value='';try{states.value[a.id]=await api(`/accounts/${a.id}/${kind}`,{method:'POST'});onResult?.(true)}catch(e){error.value=e.message;onResult?.(false,e.message)}finally{busy.value=''}}
 async function search(){page.value=1;try{await loadOrders();error.value=''}catch(e){error.value=e.message}}
 async function filterOrders(status,purchase='waitPurchase'){orderStatus.value=status;purchaseTab.value=purchase;page.value=1;orders.value=[];total.value=0;try{await loadOrders();error.value=''}catch(e){error.value=e.message}}
 async function turn(n){page.value+=n;try{await loadOrders()}catch(e){error.value=e.message}}
 onMounted(()=>{readRoute();window.addEventListener('hashchange',readRoute);timer=setInterval(poll,5000)});onUnmounted(()=>{clearInterval(timer);window.removeEventListener('hashchange',readRoute)});
 return{api,menu,route,current,accounts,selected,name,error,busy,states,archives,orders,total,page,query,orderStatus,purchaseTab,orderCounts,filterOrders,ordersChanged,loading,labels,navigate,selectAccount,select,add,removeAccount,action,search,turn,refresh};
},template:`<div class="app-shell"><OrderSyncNotifications :accounts="accounts" :api="api" @orders-changed="ordersChanged"/><aside class="sidebar"><div class="brand"><div class="brand-mark">OP</div><div><strong>订单工作台</strong><small>Order Processor</small></div></div><div class="nav-caption">工作空间</div><nav aria-label="主菜单"><a v-for="m in menu" :key="m.id" :href="'#/'+m.id" :class="{active:route===m.id}" :aria-current="route===m.id?'page':undefined"><span class="nav-icon" aria-hidden="true">{{m.icon}}</span>{{m.label}}<span v-if="route===m.id" class="nav-indicator"></span></a></nav><div class="sidebar-footer session-user"><strong :title="user.username">{{user.username}}</strong><button class="text-button" type="button" :disabled="sessionBusy" @click="$emit('logout')">退出登录</button></div></aside>
<div class="workspace" :class="{'orders-workspace':route==='orders'}"><header class="topbar"><div class="breadcrumb">工作空间 <span>/</span> <b>{{current.label}}</b></div><div class="account-picker"><label for="account-select">当前账户</label><select id="account-select" v-model="selected" @change="selectAccount"><option v-if="!accounts.length" value="">暂无账户</option><option v-for="a in accounts" :key="a.id" :value="a.id">{{a.name}}</option></select></div></header><main><div v-if="!selected && !['accounts','users'].includes(route)" class="empty">{{user.isAdmin?'请先在账户管理添加妙手账户。':'暂无已分配的妙手账户，请联系管理员。'}}<p v-if="error" role="alert">{{error}}</p></div><template v-else><div v-if="route==='orders'" class="order-heading-actions"><button :disabled="!selected||busy||states[selected]?.sync?.state==='running'" @click="action({id:selected},'sync')">网页采集全部订单</button><button :disabled="!selected||busy||states[selected]?.sync?.state==='running'" @click="action({id:selected},'sync-pending')">网页采集待处理订单</button><button v-if="states[selected]?.sync?.state==='running'" class="secondary" @click="action({id:selected},'stop-sync')" :disabled="busy">停止</button></div><div v-if="error" class="error" role="alert">{{error}}</div>
<details v-if="route==='orders'" class="order-sync-details"><summary>同步详情<span v-if="states[selected]?.sync"> · {{({running:'采集中',completed:'已完成',failed:'失败',cancelled:'已停止'})[states[selected].sync.state]}} · 第 {{states[selected].sync.page}} / {{states[selected].sync.pages}} 页</span><span v-if="states[selected]?.sync?.error" class="failure"> · {{states[selected].sync.error}}</span></summary><div class="order-sync-content"><p>全部订单包含历史各状态；待处理包含未出货和已出货。每页 200 条采集，间隔 3～5 秒。</p><p v-if="states[selected]?.sync">{{states[selected].sync.scope==='products'?'店铺产品':states[selected].sync.scope==='pending'?'待处理订单':'全部订单'}} · {{states[selected].sync.group||'准备中'}} · {{({running:'采集中',completed:'采集完成，入库后台处理中',failed:'采集中断',cancelled:'已停止'})[states[selected].sync.state]}} · 第 {{states[selected].sync.page}} / {{states[selected].sync.pages}} 页 · 当前分组 {{states[selected].sync.total}} 个包裹 · 累计采集 {{states[selected].sync.collected}} 个包裹</p><p v-if="states[selected]?.sync?.notice">{{states[selected].sync.notice}}</p><p v-if="states[selected]?.sync?.pageSize">采集每页 {{states[selected].sync.pageSize}} 条 · 第 {{states[selected].sync.pass||1}} 轮</p><p v-if="states[selected]?.sync?.step">当前步骤：{{states[selected].sync.step}}</p><p v-if="states[selected]?.sync?.error" class="failure">{{states[selected].sync.error}}</p><details v-if="states[selected]?.sync?.errorDetail"><summary>查看失败详情</summary><pre class="sync-error-detail">{{states[selected].sync.errorDetail}}</pre></details></div></details>
<OrdersView :key="selected" v-if="route==='orders'" :account="selected" :api="api" :status="orderStatus" :purchase-tab="purchaseTab" :counts="orderCounts" @filter="filterOrders" :orders="orders" :total="total" :page="page" :loading="loading" v-model:query="query" @search="search" @turn="turn" @submitted="refresh"/>
<ShipmentStatsView :key="selected" v-else-if="route==='shipment-stats'" :account="selected" :api="api"/>
<TasksView v-else-if="route==='tasks'" :key="selected" :account="selected" :api="api"/>
<UsersView v-else-if="route==='users'" :api="api" :accounts="accounts"/><AccountsView :api="api" v-else-if="route==='accounts'" :accounts="accounts" :states="states" :busy="busy" :labels="labels" v-model:name="name" @add="add" @remove="removeAccount" @action="action" @select="select"/>
<ProductsView :key="selected" v-else-if="route==='products'" :account="selected" :api="api" :state="states[selected]" :busy="busy" @action="action"/><ProcurementView :key="selected" :user="user" v-else-if="route==='procurement'" :account="selected" :api="api"/><SuppliersView :key="selected" v-else-if="route==='suppliers'" :account="selected" :api="api"/><ArchivesView v-else :archives="archives"/>
</template></main></div></div>`};
createApp({components:{SessionGate,Workspace},template:`<SessionGate v-slot="{user,api,logout,busy}"><Workspace :key="user.id+':'+user.isAdmin" :user="user" :api="api" :session-busy="busy" @logout="logout"/></SessionGate>`}).mount('#app');
