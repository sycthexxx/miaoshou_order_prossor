import FirstMileSync from '../components/FirstMileSync.js';

const sections=[
  {id:'browsers',label:'妙手账户',href:'#/accounts'},
  {id:'first-mile',label:'头程物流',href:'#/accounts/first-mile'},
  {id:'notifications',label:'通知记录',href:'#/accounts/notifications'},
  {id:'procurement-helper',label:'采购助手',href:'#/accounts/procurement-helper'}
];

export default {
  components:{FirstMileSync},
  props:['api','accounts','states','busy','name','labels','section'],
  emits:['add','remove','action','select','update:name'],
  data(){
    return {sections,settings:null,saving:false,notice:'',settingsError:'',restartId:null,restartMessages:{}};
  },
  computed:{
    settingsAccountKey(){
      return this.section==='procurement-helper'?this.accounts.map(a=>a.id).join(','):null;
    }
  },
  watch:{
    settingsAccountKey:{
      immediate:true,
      async handler(key){
        if(key===null)return;
        this.settings=null;this.settingsError='';this.notice='';
        try{
          const settings=await this.api('/plugin-query-settings');
          if(this.settingsAccountKey===key)this.settings=settings;
        }catch(e){if(this.settingsAccountKey===key)this.settingsError=e.message;}
      }
    }
  },
  methods:{
    restart(account){
      if(this.busy||!window.confirm('重启「'+account.name+'」的浏览器容器？远程桌面将断开，未保存的输入会丢失。'))return;
      this.restartId=account.id;
      this.restartMessages[account.id]='';
      this.$emit('action',account,'restart-container',(ok,message)=>{
        this.restartId=null;
        this.restartMessages[account.id]=ok?'容器已重启，可以重新打开远程桌面。':message;
      });
    },
    async saveSettings(){
      this.saving=true;this.settingsError='';this.notice='';
      try{
        await this.api('/plugin-query-settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(this.settings)});
        this.notice='已保存';
      }catch(e){this.settingsError=e.message;}
      finally{this.saving=false;}
    }
  },
  template:`<section class="accounts-view">
    <nav class="accounts-subnav" aria-label="账户管理子菜单">
      <a v-for="item in sections" :key="item.id" :href="item.href"
        :aria-current="section===item.id?'page':undefined">{{item.label}}</a>
    </nav>

    <section v-if="section==='browsers'" class="panel accounts-panel" aria-labelledby="accounts-browsers-title">
      <header class="accounts-panel-heading"><h2 id="accounts-browsers-title">妙手账户</h2></header>
      <form class="account-add-form" @submit.prevent="$emit('add')">
        <input :value="name" @input="$emit('update:name',$event.target.value)" required maxlength="60"
          placeholder="账户名称，例如公司主账号" aria-label="账户名称">
        <button :disabled="!!busy||!name.trim()">添加账户</button>
      </form>
      <p v-if="!accounts.length" class="empty accounts-empty">添加账户后，在专用浏览器登录妙手并查询订单。</p>
      <div v-else class="accounts-list">
        <article v-for="account in accounts" :key="account.id" class="account-card">
          <header class="account-card-header">
            <div class="account-avatar" aria-hidden="true">{{account.name.slice(0,1)}}</div>
            <div class="account-card-identity">
              <h3>{{account.name}} <small>#{{account.id}}</small></h3>
              <span v-if="states[account.id]?.browserMode" class="account-mode">{{states[account.id].browserMode==='docker'?'容器模式':'本机模式'}}</span>
            </div>
            <span :class="['account-state',{'verified':states[account.id]?.state==='verified'}]">{{labels[states[account.id]?.state]||'连接中'}}</span>
          </header>
          <p v-if="states[account.id]?.lastVerifiedAt" class="scope-note account-verification">最近验证 {{new Date(states[account.id].lastVerifiedAt).toLocaleString()}}</p>
          <p v-if="states[account.id]?.error" role="alert" class="failure account-verification">{{states[account.id].error}}</p>
          <div class="account-main-actions">
            <button :disabled="!!busy" @click="$emit('action',account,'login')">{{busy===account.id&&!restartId?'处理中…':states[account.id]?.browserMode==='docker'?'打开远程浏览器':'打开本机登录窗口'}}</button>
            <a v-if="states[account.id]?.desktopURL" :href="states[account.id].desktopURL" target="_blank" rel="noopener noreferrer" class="account-desktop-link">打开远程桌面</a>
            <button class="secondary" @click="$emit('select',account,'orders')">查看订单</button>
          </div>
          <details class="account-more">
            <summary>更多操作</summary>
            <div class="account-main-actions">
              <button class="secondary" :disabled="!!busy" @click="$emit('action',account,'close')">关闭浏览器</button>
              <button v-if="states[account.id]?.browserMode==='docker'" class="secondary"
                :disabled="!!busy||states[account.id]?.sync?.state==='running'" @click="restart(account)">{{restartId===account.id?'重启中…':'重启 Docker 容器'}}</button>
              <button class="text-button account-delete" type="button" :disabled="!!busy" @click="$emit('remove',account)">删除账户</button>
            </div>
          </details>
          <p v-if="restartMessages[account.id]" class="scope-note account-verification" role="status">{{restartMessages[account.id]}}</p>
        </article>
      </div>
      <p class="help-note">账户名称是本地标签，请核对浏览器中的实际登录账户。各账户分别保存登录状态。使用容器时，通过远程桌面登录，开始同步前请关闭远程桌面标签页。</p>
    </section>

    <section v-else-if="section==='first-mile'" class="panel accounts-panel" aria-labelledby="first-mile-sync-title">
      <header class="accounts-panel-heading"><h2 id="first-mile-sync-title">头程物流自动同步</h2></header>
      <p v-if="!accounts.length" class="empty accounts-empty">请先在「妙手账户」中添加账户。</p>
      <div v-else class="accounts-detail-list">
        <FirstMileSync v-for="account in accounts" :key="account.id" :api="api" :account="account"/>
      </div>
    </section>

    <section v-else-if="section==='notifications'" class="panel accounts-panel" aria-labelledby="order-notification-title">
      <header class="accounts-panel-heading"><h2 id="order-notification-title">订单通知接收记录</h2></header>
      <p v-if="!accounts.length" class="empty accounts-empty">请先在「妙手账户」中添加账户。</p>
      <div v-else class="accounts-detail-list">
        <details v-for="account in accounts" :key="account.id" class="panel">
          <summary>{{account.name}} · {{states[account.id]?.orderEvents?.batches?.length || 0}} 批记录</summary>
          <p v-if="!states[account.id]?.orderEvents?.batches?.length" class="scope-note">暂无接收记录</p>
          <div v-else class="order-notification-table" tabindex="0" role="region" :aria-label="account.name+' 通知记录'">
            <table>
              <thead><tr><th scope="col">接收时间</th><th scope="col">来源</th><th scope="col">新增</th><th scope="col">已取消</th><th scope="col">取消中</th><th scope="col">入队消息</th></tr></thead>
              <tbody><tr v-for="(batch,index) in states[account.id].orderEvents.batches" :key="batch.receivedAt+'-'+batch.source+'-'+index">
                <td>{{new Date(batch.receivedAt).toLocaleString()}}</td>
                <td>{{batch.source==='websocket'?'WebSocket':'HTTP 查询'}}</td>
                <td>{{batch.newOrders}} 单</td><td>{{batch.cancelled}} 单</td><td>{{batch.cancelling}} 单</td><td>{{batch.accepted}} 条</td>
              </tr></tbody>
            </table>
          </div>
        </details>
      </div>
    </section>

    <section v-else class="panel accounts-panel" aria-labelledby="procurement-helper-title">
      <header class="accounts-panel-heading">
        <h2 id="procurement-helper-title">采购助手默认查询账户</h2>
        <p>采购助手直接打开 1688 货源时，会使用这里选择的账户查询。</p>
      </header>
      <p v-if="!accounts.length" class="empty accounts-empty">请先在「妙手账户」中添加账户。</p>
      <form v-else-if="settings" class="account-plugin-form" @submit.prevent="saveSettings" @change="notice=''">
        <label>查询账户
          <select v-model="settings.account" :disabled="saving">
            <option v-for="account in accounts" :key="account.id" :value="account.id">{{account.name}} · #{{account.id}}</option>
          </select>
        </label>
        <label class="account-gifts"><input type="checkbox" v-model="settings.gifts" :disabled="saving"> 包含赠品</label>
        <button :disabled="saving||!settings.account">{{saving?'保存中…':'保存设置'}}</button>
      </form>
      <p v-else-if="!settingsError" role="status" class="scope-note">正在读取采购助手设置…</p>
      <p v-if="settingsError" role="alert" class="failure">{{settingsError}}</p>
      <p v-if="notice" role="status">{{notice}}</p>
    </section>
  </section>`
};
