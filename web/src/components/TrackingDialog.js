import {onMounted,onUnmounted,ref} from 'vue/dist/vue.esm-bundler.js';

export default {
  props:['api','account','entry'],
  emits:['close','updated'],
  setup(props,{emit}){
    const dialog=ref(null), result=ref(null), pending=ref(false), error=ref('');
    let controller;
    async function load(){
      controller?.abort();
      const request=new AbortController();
      controller=request;
      pending.value=true;
      error.value='';
      const {kind,packageId,number}=props.entry;
      const query=new URLSearchParams({kind,number});
      try {
        result.value=await props.api('/accounts/'+encodeURIComponent(props.account)+'/packages/'+encodeURIComponent(packageId)
          +'/tracking?'+query,{signal:request.signal});
        if(!request.signal.aborted && result.value.latest)emit('updated',result.value);
      } catch(e){
        if(!request.signal.aborted)error.value=e.message;
      } finally{
        if(!request.signal.aborted)pending.value=false;
      }
    }
    onMounted(()=>{dialog.value.showModal();void load();});
    onUnmounted(()=>controller?.abort());
    return {dialog,result,pending,error,load};
  },
  template:`<Teleport to="body"><dialog ref="dialog" class="tracking-dialog" aria-labelledby="tracking-title" @close="$emit('close')">
    <header><h2 id="tracking-title">{{entry.kind==='platform'?'TikTok 物流进度':'国内物流轨迹'}}</h2>
      <button type="button" class="secondary" @click="dialog.close()">关闭</button>
    </header>
    <div class="tracking-content" :aria-busy="pending">
      <p class="tracking-waybill"><span>{{entry.company || '运单号'}}</span><strong>{{result?.number || entry.number}}</strong>
        <span v-if="result?.latest" class="tracking-state">{{result.latest.status}}</span></p>
      <p v-if="pending" role="status">正在读取妙手物流轨迹…</p>
      <div v-else-if="error" role="alert" class="tracking-error"><p>{{error}}</p><button class="secondary" type="button" @click="load">重试</button></div>
      <ol v-else-if="result?.steps?.length" class="tracking-timeline">
        <li v-for="(step,index) in result.steps" :key="index">
          <strong v-if="index===0">最新物流动态</strong>
          <time v-if="step.time">{{step.time}}</time>
          <p v-if="step.description">{{step.description}}</p>
          <small v-if="step.place">{{step.place}}</small>
          <small v-if="step.detail">{{step.detail}}</small>
        </li>
      </ol>
      <p v-else role="status" class="scope-note">妙手暂无此运单的物流轨迹。</p>
    </div>
  </dialog></Teleport>`
};
