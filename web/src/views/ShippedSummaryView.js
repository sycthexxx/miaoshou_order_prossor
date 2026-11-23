import {computed,onMounted,onUnmounted,ref,watch} from 'vue';
import ProductImage from '../components/ProductImage.js';
import TrackingDialog from '../components/TrackingDialog.js';

export default {
  components:{ProductImage,TrackingDialog},
  props:['api','account','orderDataRevision'],
  setup(props) {
    const factories=ref([]), loading=ref(false), error=ref(''), statusFilter=ref('all'), tracking=ref(null);
    const offerBridgeReady=ref(false);
    function readOfferBridge(){
      const [major,minor,patch]=(document.documentElement.dataset.opProcurementBridgeReady||'').split('.').map(Number);
      offerBridgeReady.value=major>0||minor>1||(minor===1&&patch>=19);
    }
    function offerHandoff(offerId,title){
      if(!offerBridgeReady.value||!offerId)return undefined;
      return JSON.stringify({version:1,account:props.account,offerId,
        url:'https://detail.1688.com/offer/'+offerId+'.html',title,createdAt:Date.now(),variants:[]});
    }
    const missingFactories=computed(()=>factories.value.filter(f=>f.missing).length);
    const missingGroups=computed(()=>factories.value.reduce((count,f)=>count+f.missing,0));
    const statusOptions=computed(()=>{
      const statuses=new Set(['已打包','已揽收','运输中','已送达']);
      for(const factory of factories.value) for(const group of factory.groups) for(const shipment of group.shipments){
        if(shipment.tracking?.status)statuses.add(shipment.tracking.status);
      }
      if(statusFilter.value.startsWith('status:'))statuses.add(statusFilter.value.slice(7));
      return [...statuses];
    });
    function matchesStatus(group){
      const filter=statusFilter.value;
      if(filter==='all')return true;
      if(filter==='missing')return !group.shipments.length;
      return group.shipments.some(shipment=>filter==='untracked'
        ? !shipment.tracking?.status
        : 'status:'+shipment.tracking?.status===filter);
    }
    const visible=computed(()=>factories.value
      .map(factory=>{
        const groups=factory.groups.filter(matchesStatus)
          .sort((a,b)=>b.orders.length-a.orders.length);
        const packageCount=new Set(groups.flatMap(group=>group.orders.map(order=>order.packageId))).size;
        return {...factory,groups,packageCount};
      })
      .filter(factory=>factory.groups.length)
      .sort((a,b)=>b.packageCount-a.packageCount));
    let controller;
    async function load() {
      controller?.abort();
      const request=new AbortController();
      controller=request;
      loading.value=true;
      error.value='';
      try{
        factories.value=await props.api('/accounts/'+encodeURIComponent(props.account)+'/shipped-summary',{signal:request.signal});
      }catch(e){
        if(!request.signal.aborted)error.value=e.message;
      }finally{
        if(!request.signal.aborted)loading.value=false;
      }
    }
    function updateTracking({number,latest}) {
      for(const factory of factories.value) for(const group of factory.groups) for(const waybill of group.shipments){
        if(waybill.number===number)waybill.tracking=latest;
      }
    }
    const syncTime=new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'});
    const formatSyncTime=value=>syncTime.format(new Date(value));
    onMounted(()=>{
      readOfferBridge();
      window.addEventListener('op-procurement-bridge-status',readOfferBridge);
      void load();
    });
    watch(()=>props.orderDataRevision,load);
    onUnmounted(()=>{
      controller?.abort();
      window.removeEventListener('op-procurement-bridge-status',readOfferBridge);
    });
    return {visible,loading,error,statusFilter,statusOptions,missingFactories,missingGroups,tracking,load,updateTracking,formatSyncTime,offerHandoff};
  },
  template:`<section class="shipped-summary">
    <div class="toolbar shipped-summary-toolbar">
      <div><strong>待处理 · 已出货</strong><span class="shipped-summary-count">{{missingFactories}} 家厂家 · {{missingGroups}} 组缺快递单号</span></div>
      <label class="shipped-status-filter">物流状态
        <select v-model="statusFilter">
          <option value="all">全部状态</option>
          <option value="missing">缺快递单号</option>
          <option value="untracked">暂无物流状态</option>
          <option v-for="status in statusOptions" :key="status" :value="'status:'+status">{{status}}</option>
        </select>
      </label>
      <button class="secondary" type="button" @click="load" :disabled="loading">{{loading?'正在读取…':'刷新汇总'}}</button>
    </div>
    <p v-if="error" role="alert" class="error">{{error}}</p>
    <p v-else-if="loading && !visible.length" role="status">正在读取已出货汇总…</p>
    <p v-else-if="!visible.length" class="empty">{{statusFilter==='all'?'当前没有待处理的已出货订单':'当前没有符合所选物流状态的记录'}}</p>
    <section v-for="factory in visible" :key="factory.name" class="shipped-factory">
      <header><h2>{{factory.name}}</h2><span class="shipped-factory-count">{{factory.packageCount}} 个包裹</span><span>{{factory.groups.length}} 组</span>
        <strong v-if="factory.missing" class="missing-waybill">缺快递单号 {{factory.missing}} 组</strong>
        <a v-if="factory.chatUrl||factory.offerId" class="shipped-supplier-chat"
          :href="factory.chatUrl||'https://detail.1688.com/offer/'+factory.offerId+'.html'"
          :data-op-procurement="factory.chatUrl?undefined:offerHandoff(factory.offerId,factory.name)"
          :title="factory.chatUrl?'联系 1688 客服':'打开 1688 关联货源并查找客服'"
          :aria-label="factory.name+(factory.chatUrl?' · 联系 1688 客服':' · 打开 1688 关联货源并查找客服')"
          target="_blank" rel="noopener noreferrer">
          <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round" stroke-linecap="round">
            <path d="M20 11.5a7.5 7.5 0 0 1-7.5 7.5H5l1.6-3.1A7.5 7.5 0 1 1 20 11.5Z"/><path d="M9 11.5h7m-7 3h4"/>
          </svg>
          <span>{{factory.chatUrl?'1688 客服':'货源 · 查找客服'}}</span>
        </a>
      </header>
      <details v-for="group in factory.groups" :key="group.key" class="shipped-group">
        <summary>
          <span v-if="group.numbers?.length" class="shipped-number">采购单 {{group.numbers.join('、')}}</span>
          <template v-if="group.numbers?.length">
            <a v-for="offer in group.offerIds" :key="offer" class="shipped-offer-link"
              :href="'https://detail.1688.com/offer/'+offer+'.html'" target="_blank" rel="noopener noreferrer"
              :data-op-procurement="offerHandoff(offer,factory.name)"
              :aria-label="'打开 1688 商品货源 '+offer" @click.stop>货源{{group.offerIds.length>1?' '+offer:''}} ↗</a>
          </template>
          <template v-if="group.shipments.length">
            <span v-for="shipment in group.shipments" :key="shipment.number">
              <button type="button" class="text-button tracking-link shipped-waybill"
                @click.stop.prevent="tracking={...shipment,kind:'domestic'}">{{[shipment.company,shipment.number].filter(Boolean).join(' ')}}</button>
              <small v-if="shipment.tracking" class="tracking-state" :data-status="shipment.tracking.status">{{shipment.tracking.status}}</small>
              <small v-if="shipment.tracking?.syncedAt" class="shipped-sync-time">上次同步 <time :datetime="shipment.tracking.syncedAt">{{formatSyncTime(shipment.tracking.syncedAt)}}</time></small>
              <small v-else-if="shipment.traceId && shipment.traceId !== '0'" class="shipped-sync-time">尚未同步</small>
            </span>
          </template>
          <strong v-else class="missing-waybill">缺快递单号</strong>
          <span class="shipped-order-count">{{group.orders.length}} 个包裹</span>
        </summary>
        <ul v-if="group.orders.length" class="shipped-orders">
          <li v-for="order in group.orders" :key="order.packageId">
            <div class="shipped-order-heading">
              <span>平台订单号</span><strong>{{order.orderNo || '未获取'}}</strong>
              <span v-if="!order.orderNo">包裹号 {{order.packageId}}</span>
              <span v-if="order.shop">{{order.shop}}</span>
            </div>
            <div v-for="item in order.items" :key="item.opOrderItemId" class="shipment-sku-product shipped-sku">
              <ProductImage :item="item"/>
              <div><strong>{{item.cnTitle || item.title || '商品名未提供'}}</strong>
                <small>{{item.skuSubCnName || item.skuSubName || '规格未提供'}}<template v-if="item.platformSkuId"> · SKU {{item.platformSkuId}}</template></small>
              </div>
              <b v-if="item.quantity!=null" class="shipped-quantity">×{{item.quantity}}</b>
            </div>
            <p v-if="!order.items.length" class="scope-note">暂无与该采购单关联的 SKU 明细</p>
          </li>
        </ul>
      </details>
    </section>
    <TrackingDialog v-if="tracking" :api="api" :account="account" :entry="tracking" @close="tracking=null" @updated="updateTracking"/>
  </section>`
};
