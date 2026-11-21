export default {props:['api','accounts','states','busy','name','labels'],emits:['add','remove','action','select','update:name'],data(){return {settings:null,saving:false,notice:'',restartId:null,restartMessages:{}}},
 watch:{'accounts.length':{immediate:true,async handler(){
   try{this.settings=await this.api('/plugin-query-settings')}catch(e){this.notice=e.message}
 }}},
 methods:{restart(a){
   if(this.busy || !window.confirm('重启「'+a.name+'」的浏览器容器？远程桌面将断开，未保存的输入会丢失。'))return
   this.restartId=a.id
   this.restartMessages[a.id]=''
   this.$emit('action',a,'restart-container',(ok,message)=>{
     this.restartId=null
     this.restartMessages[a.id]=ok?'容器已重启，可以重新打开远程桌面。':message
   })
 },async saveSettings(){
   this.saving=true
   try{await this.api('/plugin-query-settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(this.settings)});this.notice='已保存'}
   catch(e){this.notice=e.message}finally{this.saving=false}
 }},
 template:`<section class="panel"><div class="panel-heading"><h2>妙手账户</h2><p>各账户独立保存登录状态，采集的数据按账户隔离。</p></div><form @submit.prevent="$emit('add')"><input :value="name" @input="$emit('update:name',$event.target.value)" required maxlength="60" placeholder="账户名称，例如：公司主账号" aria-label="账户名称"><button :disabled="!!busy||!name.trim()">添加账户</button></form><article class="account-row" v-for="a in accounts" :key="a.id"><div class="account-avatar">{{a.name.slice(0,1)}}</div><div class="account-info"><h3>{{a.name}} <small>#{{a.id}}</small></h3><span :class="['account-state',states[a.id]?.state==='verified'?'verified':'']">{{labels[states[a.id]?.state]||'连接中'}}</span><p v-if="states[a.id]?.lastVerifiedAt">最近验证：{{new Date(states[a.id].lastVerifiedAt).toLocaleString()}}</p><p v-if="states[a.id]?.browserMode">{{states[a.id].browserMode==='docker'?'容器模式 · 可在其他电脑通过远程桌面登录':'本机模式 · 登录窗口仅显示在服务端电脑'}}</p><p class="failure" v-if="states[a.id]?.error">{{states[a.id].error}}</p></div><div class="actions"><button :disabled="!!busy" @click="$emit('action',a,'login')">{{busy===a.id&&!restartId?'处理中…':states[a.id]?.browserMode==='docker'?'打开远程浏览器':'打开本机登录窗口'}}</button><a v-if="states[a.id]?.desktopURL" :href="states[a.id].desktopURL" target="_blank" rel="noopener noreferrer" class="text-button">打开远程桌面</a><button class="secondary" :disabled="!!busy" @click="$emit('action',a,'close')">关闭浏览器</button><button v-if="states[a.id]?.browserMode==='docker'" class="secondary" :disabled="!!busy||states[a.id]?.sync?.state==='running'" @click="restart(a)">{{restartId===a.id?'重启中…':'重启 Docker 容器'}}</button><button class="text-button" @click="$emit('select',a,'orders')">查看订单</button><button class="text-button" type="button" :disabled="!!busy" @click="$emit('remove',a)">删除账户</button><small v-if="restartMessages[a.id]" role="status">{{restartMessages[a.id]}}</small></div></article><section v-if="accounts.length" class="order-notification-records" aria-labelledby="order-notification-title">
 <h2 id="order-notification-title">订单通知接收记录</h2>
 <details v-for="a in accounts" :key="a.id" class="panel">
  <summary>{{a.name}} · {{states[a.id]?.orderEvents?.batches?.length || 0}} 批记录</summary>
  <p v-if="!states[a.id]?.orderEvents?.batches?.length" class="scope-note">暂无接收记录</p>
  <div v-else class="order-notification-table" tabindex="0" role="region" :aria-label="a.name+' 通知记录'">
   <table>
    <thead><tr><th scope="col">接收时间</th><th scope="col">来源</th><th scope="col">新增</th><th scope="col">已取消</th><th scope="col">取消中</th><th scope="col">入队消息</th></tr></thead>
    <tbody><tr v-for="(batch,index) in states[a.id].orderEvents.batches" :key="batch.receivedAt+'-'+batch.source+'-'+index">
     <td>{{new Date(batch.receivedAt).toLocaleString()}}</td>
     <td>{{batch.source==='websocket'?'WebSocket':'HTTP 查询'}}</td>
     <td>{{batch.newOrders}} 单</td><td>{{batch.cancelled}} 单</td><td>{{batch.cancelling}} 单</td><td>{{batch.accepted}} 条</td>
    </tr></tbody>
   </table>
  </div>
 </details>
</section><div v-if="!accounts.length" class="empty">添加账户后，在专用浏览器登录妙手并查询订单。</div><div class="panel-heading"><h2>采购助手默认查询账户</h2><p>连接此后台地址的插件，直接打开 1688 货源时使用此账户。</p></div><form v-if="settings && accounts.length" @submit.prevent="saveSettings" @change="notice=''"><label>查询账户 <select v-model="settings.account" :disabled="saving"><option v-for="a in accounts" :key="a.id" :value="a.id">{{a.name}} · #{{a.id}}</option></select></label><label><input type="checkbox" v-model="settings.gifts" :disabled="saving"> 包含赠品</label><button :disabled="saving">{{saving?'保存中…':'保存设置'}}</button></form><p v-if="notice" role="status">{{notice}}</p><div class="help-note">账户名称是本地标签，请核对浏览器中的实际登录账户。关闭浏览器后，登录状态仍保留在运行采集服务的机器上。Mac / Linux 容器模式均通过“打开远程桌面”登录；开始同步前关闭远程桌面标签页，同步期间不能连接该账户桌面。</div></section>`};
