export default {
  props:['api','account'],
  data(){
    return {settings:null,loading:false,saving:false,error:'',notice:''};
  },
  computed:{
    endpoint(){return '/accounts/'+encodeURIComponent(this.account.id)+'/first-mile/tracking-sync-settings';}
  },
  methods:{
    async open(event){
      if(!event.currentTarget.open||this.settings||this.loading)return;
      this.loading=true;this.error='';
      try{this.settings=await this.api(this.endpoint);}
      catch(e){this.error=e.message;}
      finally{this.loading=false;}
    },
    async save(){
      this.saving=true;this.error='';this.notice='';
      try{
        this.settings=await this.api(this.endpoint,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(this.settings)});
        this.notice=this.settings.enabled?'已启用物流轨迹自动同步':'已关闭物流轨迹自动同步';
      }catch(e){this.error=e.message;}
      finally{this.saving=false;}
    }
  },
  template:`<details class="domestic-tracking-rules domestic-tracking-sync" @toggle="open">
    <summary>已出货物流轨迹自动同步</summary>
    <p class="scope-note">依次查询已打包、已揽收、运输中和未查过的国内快递单。同一快递单号每轮只查一次。请保持妙手订单页打开。</p>
    <p v-if="loading" role="status">正在读取设置…</p>
    <form v-if="settings" @submit.prevent="save" @input="notice=''">
      <label><input v-model="settings.enabled" :disabled="saving" type="checkbox"> 启用物流轨迹自动同步</label>
      <fieldset>
        <legend>轮次间隔（分钟）</legend>
        <label>最短<input v-model.number="settings.minMinutes" type="number" min="1" max="1440" step="1" required :disabled="saving"></label>
        <label>最长<input v-model.number="settings.maxMinutes" type="number" :min="settings.minMinutes || 1" max="1440" step="1" required :disabled="saving"></label>
      </fieldset>
      <fieldset>
        <legend>逐单间隔（秒）</legend>
        <label>最短<input v-model.number="settings.minSeconds" type="number" min="1" max="300" step="1" required :disabled="saving"></label>
        <label>最长<input v-model.number="settings.maxSeconds" type="number" :min="settings.minSeconds || 1" max="300" step="1" required :disabled="saving"></label>
      </fieldset>
      <button :disabled="saving">{{saving?'正在保存…':'保存轨迹同步设置'}}</button>
    </form>
    <p v-if="notice" role="status">{{notice}}</p>
    <p v-if="error" role="alert" class="failure">{{error}}</p>
  </details>`
};
