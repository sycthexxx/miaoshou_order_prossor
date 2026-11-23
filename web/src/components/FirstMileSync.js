import DomesticTrackingSync from './DomesticTrackingSync.js';
import DomesticTrackingRules from './DomesticTrackingRules.js';

export default {
  components:{DomesticTrackingRules,DomesticTrackingSync},
  props:['api','account'],
  data(){
    return {settings:null,runs:[],next:'',loaded:false,loading:false,saving:false,notice:'',error:'',
      runDetails:{},detailErrors:{},openRun:'',stopId:'',poll:null,starting:false};
  },
  beforeUnmount(){clearInterval(this.poll);},
  computed:{
    endpoint(){return '/accounts/'+this.account.id+'/first-mile';},
    running(){return this.runs.some(run=>run.state==='running'||run.state==='stopping');}
  },
  methods:{
    time(value){return value ? new Date(value).toLocaleString() : '—';},
    runState(state){
      return {running:'同步中',stopping:'正在停止',stopped:'已停止',completed:'已完成',
        partial:'部分失败',failed:'失败',interrupted:'已中断'}[state] || state;
    },
    itemState(state){return {updated:'已回写',failed:'失败',skipped:'未刷新'}[state] || state;},
    purchaseState(state){
      return {wait_pay:'待付款',wait_send:'待发货',part_send:'部分发货',has_send:'已发货',
        has_sign:'已签收',cancel:'已取消'}[state] || state || '—';
    },
    async open(event){
      clearInterval(this.poll);
      if (!event.currentTarget.open) return;
      this.poll=setInterval(()=>this.loadRuns('',true),5000);
      if(!this.loaded) {
        try {this.settings=await this.api(this.endpoint+'/settings');this.loaded=true;}
        catch(e){this.error=e.message;return;}
      }
      await this.loadRuns('',true);
    },
    async save(){
      this.error='';this.notice='';this.saving=true;
      try {
        this.settings=await this.api(this.endpoint+'/settings',{
          method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(this.settings)
        });
        this.notice='设置已保存';
      } catch(e){this.error=e.message;}
      finally{this.saving=false;}
    },
    async loadRuns(before='',keepOlder=false){
      if(this.loading)return;
      this.loading=true;
      if(!keepOlder)this.error='';
      try {
        const data=await this.api(this.endpoint+'/runs'+(before?'?before='+before:''));
        const hadOlder=keepOlder && this.runs.length>20;
        if(before)this.runs=[...this.runs,...data.runs];
        else if(hadOlder){
          const ids=new Set(data.runs.map(run=>run.id));
          this.runs=[...data.runs,...this.runs.filter(run=>!ids.has(run.id))];
        }else this.runs=data.runs;
        if(!hadOlder)this.next=data.next;
        if(this.openRun)await this.loadDetail(this.openRun);
      } catch(e){this.error=e.message;}
      finally{this.loading=false;}
    },
    async runNow(){
      this.starting=true;
      this.error='';this.notice='';
      try {
        const run=await this.api(this.endpoint+'/runs',{method:'POST'});
        if(!this.runs.some(item=>item.id===run.id)){
          this.runs=[{...run,trigger:'manual',canStop:true,updated:0,failed:0,skippedSigned:0},...this.runs];
        }
        this.notice='手动同步已开始';
        await this.loadRuns('',true);
      } catch(e){this.error=e.message;}
      finally{this.starting=false;}
    },
    async stopRun(run){
      this.stopId=run.id;
      this.error='';
      try {
        await this.api(this.endpoint+'/runs/'+run.id+'/stop',{method:'POST'});
        const current=this.runs.find(item=>item.id===run.id);
        if(current?.state==='running')current.state='stopping';
      } catch(e){this.error=e.message;}
      finally{this.stopId='';}
    },
    async toggleRun(id){
      this.openRun=this.openRun===id?'':id;
      if(this.openRun)await this.loadDetail(id);
    },
    async loadDetail(id){
      this.detailErrors[id]='';
      try {this.runDetails[id]=await this.api(this.endpoint+'/runs/'+id);}
      catch(e){this.detailErrors[id]=e.message;}
    }
  },
  template:`<details class="panel" @toggle="open">
    <summary>{{account.name}}</summary>
    <p class="scope-note">同步范围为妙手「待处理 → 已出货」里的未签收 1688 采购单。</p>
    <form v-if="settings" @submit.prevent="save">
      <label><input v-model="settings.enabled" type="checkbox"> 启用自动同步</label>
      <label>间隔（分钟） <input v-model.number="settings.intervalMinutes" type="number"
        min="1" max="1440" step="1" required></label>
      <button :disabled="saving">保存设置</button>
    </form>
    <DomesticTrackingSync :api="api" :account="account"/>
    <DomesticTrackingRules :api="api" :account="account"/>
    <p v-if="notice" role="status">{{notice}}</p>
    <p v-if="error" role="alert" class="failure">{{error}}</p>
    <button type="button" :disabled="starting||running" @click="runNow">{{starting?'正在启动…':'立即同步'}}</button>
    <button type="button" class="secondary" :disabled="loading" @click="loadRuns()">{{loading?'正在读取…':'刷新同步记录'}}</button>
    <p v-if="!loading && !runs.length" class="scope-note">暂无同步记录</p>
    <div v-if="runs.length" class="order-notification-table" tabindex="0" role="region" :aria-label="account.name+' 头程物流同步记录'">
      <table>
        <thead><tr><th>开始时间</th><th>结束时间</th><th>来源</th><th>状态</th><th>已回写</th><th>失败 / 未刷新</th><th>跳过已签收</th><th>明细</th></tr></thead>
        <tbody v-for="run in runs" :key="run.id">
          <tr><td>{{time(run.startedAt)}}</td><td>{{time(run.finishedAt)}}</td><td>{{run.trigger==='manual'?'手动':'自动'}}</td><td>{{runState(run.state)}}</td>
            <td>{{run.updated}}</td><td>{{run.failed}}</td><td>{{run.skippedSigned}}</td>
            <td><button type="button" class="text-button" :aria-expanded="openRun===run.id" @click="toggleRun(run.id)">
              {{openRun===run.id?'收起':'查看明细'}}</button>
              <button v-if="run.state==='running' && run.canStop" type="button" class="secondary" :disabled="stopId===run.id"
                @click="stopRun(run)">停止本轮</button></td>
          </tr>
          <tr v-if="openRun===run.id"><td colspan="8">
            <p v-if="detailErrors[run.id]" role="alert" class="failure">{{detailErrors[run.id]}}</p>
            <template v-else-if="runDetails[run.id]">
              <p v-if="runDetails[run.id].error" class="failure">{{runDetails[run.id].error}}</p>
              <p v-if="!runDetails[run.id].items.length" class="scope-note">{{['running','stopping'].includes(run.state)?'本轮结束后显示明细':'本轮暂无逐单明细'}}</p>
              <table v-else :aria-label="'采购单同步明细 '+time(run.startedAt)">
                <thead><tr><th>1688 采购单号</th><th>刷新前状态</th><th>刷新后状态</th><th>结果</th><th>原因</th></tr></thead>
                <tbody><tr v-for="item in runDetails[run.id].items" :key="item.id">
                  <td>{{item.number}}</td><td>{{purchaseState(item.beforeStatus)}}</td><td>{{purchaseState(item.afterStatus)}}</td>
                  <td>{{itemState(item.state)}}</td><td>{{item.error || '—'}}</td>
                </tr></tbody>
              </table>
            </template>
            <p v-else class="scope-note">正在读取明细…</p>
          </td></tr>
        </tbody>
      </table>
    </div>
    <button v-if="next" type="button" class="secondary" :disabled="loading" @click="loadRuns(next)">查看更多记录</button>
  </details>`
};
