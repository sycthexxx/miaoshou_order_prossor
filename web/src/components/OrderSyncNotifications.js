import {ref,watch,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';

export default {
 props:['accounts','api'],
 setup(props){
  const notices=ref([]);
  const observed=new Map(),inFlight=new Set(),timeouts=new Map();
  const openedAt=Date.now();
  let timer,disposed=false;

  function dismiss(key){
   clearTimeout(timeouts.get(key));
   timeouts.delete(key);
   notices.value=notices.value.filter(n=>n.key!==key);
  }
  async function check(account,source){
   const channel=account.id+'/'+source;
   if(inFlight.has(channel))return;
   inFlight.add(channel);
   try{
    const web=source==='status';
    const response=await props.api('/accounts/'+account.id+'/'+source);
    if(disposed)return;
    const job=web?response.sync:response;
    if(!job || job.state!=='completed' || job.scope==='products'){
     if(!observed.has(channel))observed.set(channel,'');
     return;
    }
    const run=job.finishedAt||job.startedAt;
    if(!run)return;
    const previous=observed.get(channel);
    observed.set(channel,run);
    if(previous===run || (previous===undefined && Date.parse(run)<openedAt))return;
    const key=channel+'/'+run;
    const sourceName=web?(job.scope==='pending'?'网页待处理订单':'网页全部订单'):'开放 API 订单';
    notices.value.push({key,text:account.name+' · '+sourceName+'同步完成'});
    timeouts.set(key,setTimeout(()=>dismiss(key),15000));
   }catch{
    // Keep the last observed run and retry transient status failures on the next poll.
   }finally{inFlight.delete(channel)}
  }
  function poll(){
   for(const account of props.accounts){
    void check(account,'status');
    void check(account,'openapi-status');
   }
  }
  watch(()=>props.accounts.map(a=>a.id).join(','),poll,{immediate:true});
  onMounted(()=>{timer=setInterval(poll,4000)});
  onUnmounted(()=>{disposed=true;clearInterval(timer);for(const t of timeouts.values())clearTimeout(t)});
  return {notices,dismiss};
 },
 template:`<div class="sync-notifications" aria-live="polite" aria-relevant="additions">
  <div v-for="notice in notices" :key="notice.key"><strong>{{notice.text}}</strong><button class="text-button" type="button" aria-label="关闭通知" @click="dismiss(notice.key)">×</button></div>
 </div>`
};
