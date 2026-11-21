import {ref,inject,watch,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';

export default {
 props:['accounts','api'],
 emits:['orders-changed'],
 setup(props,{emit}){
  const notices=ref([]);
  const noticeHost=inject('order-notice-host',ref(null));
  const observed=new Map(),inFlight=new Set(),timeouts=new Map();
  const openedAt=Date.now();
  const eventFeeds=new Map();
  const voiceBlocked=ref(false),voiceQueue=[];
  let audio=null,timer,disposed=false;
  const kinds={
   new_order:{label:'新订单',sound:'new_order'},
   in_cancel:{label:'取消中',sound:'after_sales_cancelled'},
   cancelled:{label:'已取消',sound:'after_sales_cancelled'}
  };
  function notify(key,text,persistent=false){
   notices.value.push({key,text});
   if(!persistent)timeouts.set(key,setTimeout(()=>dismiss(key),15000));
  }
  function playNext(){
   if(disposed||audio||!voiceQueue.length)return;
   const sound=voiceQueue.shift(),player=new Audio('/audio/'+sound+'.mp3');
   audio=player;
   function finish(){
    if(audio!==player)return;
    audio=null;
    playNext();
   }
   player.onended=finish;
   player.onerror=finish;
   player.play().catch(e=>{
    if(audio!==player||disposed)return;
    if(e.name!=='NotAllowedError'){finish();return}
    audio=null;
    if(!voiceQueue.includes(sound))voiceQueue.unshift(sound);
    voiceBlocked.value=true;
   });
  }
  function enableVoice(){voiceBlocked.value=false;playNext()}
  function announceChanges(account,events){
   const groups=new Map();
   for(const event of events){
    if(!Object.hasOwn(kinds,event.kind))continue;
    if(!groups.has(event.kind))groups.set(event.kind,[]);
    groups.get(event.kind).push(event);
   }
   for(const [kind,items] of groups){
    const {sound}=kinds[kind];
    const bySupplier=new Map();
    for(const item of items){
     for(const supplier of item.suppliers?.length?item.suppliers:['未指定厂家']){
      if(!bySupplier.has(supplier))bySupplier.set(supplier,new Set());
      bySupplier.get(supplier).add(item.orderNumber);
     }
    }
    for(const [supplier,orders] of bySupplier){
     const count=orders.size===1?'一单':orders.size+' 单';
     const change=kind==='new_order'?'新增'+count:kind==='cancelled'?'取消'+count:count+'正在取消';
     notify('order/'+account.id+'/'+items[0].id+'/'+supplier,account.name+' · '+supplier+' '+change,true);
    }
    if(!voiceQueue.includes(sound))voiceQueue.push(sound);
   }
   if(!voiceBlocked.value)playNext();
   emit('orders-changed',account.id);
  }
  function connectEvents(account){
   const feed={cursor:null,socket:null,retry:null,attempt:0};
   eventFeeds.set(account.id,feed);
   const active=()=>!disposed&&eventFeeds.get(account.id)===feed;
   function connect(){
    if(!active())return;
    const url=new URL('/api/accounts/'+encodeURIComponent(account.id)+'/order-events/ws',location.origin);
    url.protocol=url.protocol==='https:'?'wss:':'ws:';
    if(feed.cursor!==null)url.searchParams.set('after',feed.cursor);
    const socket=new WebSocket(url);
    feed.socket=socket;
    socket.onopen=()=>{feed.attempt=0};
    socket.onmessage=({data})=>{
     if(!active())return;
     let result;
     try{result=JSON.parse(data)}catch{return}
     if(!Number.isSafeInteger(result.cursor)||!Array.isArray(result.events))return;
     const fresh=result.events.filter(event=>event.id>(feed.cursor??0));
     feed.cursor=result.cursor;
     if(fresh.length)announceChanges(props.accounts.find(a=>a.id===account.id)||account,fresh);
    };
    socket.onclose=()=>{
     if(active())feed.retry=setTimeout(connect,Math.min(15000,1000*2**Math.min(feed.attempt++,4)));
    };
   }
   connect();
  }
  function disconnectEvents(id){
   const feed=eventFeeds.get(id);
   eventFeeds.delete(id);
   clearTimeout(feed.retry);
   feed.socket?.close();
  }

  function dismiss(key){
   clearTimeout(timeouts.get(key));
   timeouts.delete(key);
   notices.value=notices.value.filter(n=>n.key!==key);
  }
  async function check(account){
   const channel=account.id;
   if(inFlight.has(channel))return;
   inFlight.add(channel);
   try{
    const response=await props.api('/accounts/'+account.id+'/status');
    if(disposed)return;
    const job=response.sync;
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
    const sourceName=job.scope==='pending'?'网页待处理订单':'网页全部订单';
    notify(key,account.name+' · '+sourceName+'同步完成');
   }catch{
    // Keep the last observed run and retry transient status failures on the next poll.
   }finally{inFlight.delete(channel)}
  }
  function poll(){
   for(const account of props.accounts){
    void check(account);
   }
  }
  watch(()=>props.accounts.map(a=>a.id).join(','),()=>{
   const ids=new Set(props.accounts.map(a=>a.id));
   for(const id of eventFeeds.keys())if(!ids.has(id))disconnectEvents(id);
   for(const account of props.accounts)if(!eventFeeds.has(account.id))connectEvents(account);
   poll();
  },{immediate:true});
  onMounted(()=>{timer=setInterval(poll,4000)});
  onUnmounted(()=>{disposed=true;for(const id of eventFeeds.keys())disconnectEvents(id);clearInterval(timer);audio?.pause();audio=null;for(const t of timeouts.values())clearTimeout(t)});
  return {noticeHost,notices,dismiss,voiceBlocked,enableVoice};
 },
 template:`<Teleport :to="noticeHost || 'body'" :disabled="!noticeHost"><div class="sync-notifications" aria-live="polite" aria-relevant="additions">
  <div v-if="voiceBlocked"><strong>订单语音提醒待启用</strong><button type="button" @click="enableVoice">启用语音</button></div>
  <div v-for="notice in notices" :key="notice.key"><strong>{{notice.text}}</strong><button class="text-button" type="button" aria-label="关闭通知" @click="dismiss(notice.key)">×</button></div>
 </div></Teleport>`
};
