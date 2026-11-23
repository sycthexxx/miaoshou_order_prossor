export default {
  props:['api','account'],
  data(){
    return {rules:null,loading:false,saving:false,error:'',notice:'',nextKey:0};
  },
  computed:{
    endpoint(){return '/accounts/'+encodeURIComponent(this.account.id)+'/first-mile/tracking-rules';}
  },
  methods:{
    editRules(rules){
      this.rules=rules.map(rule=>({key:this.nextKey++,status:rule.status,
        keywords:rule.keywords.join('\n'),exclude:(rule.exclude||[]).join('\n')}));
    },
    async open(event){
      if(!event.currentTarget.open||this.rules||this.loading)return;
      this.loading=true;this.error='';
      try{this.editRules(await this.api(this.endpoint));}
      catch(e){this.error=e.message;}
      finally{this.loading=false;}
    },
    add(){
      this.rules.push({key:this.nextKey++,status:'',keywords:'',exclude:''});
      this.notice='';
    },
    async save(){
      const lines=text=>text.split(/\r?\n/).map(line=>line.trim()).filter(Boolean);
      const rules=this.rules.map(rule=>({status:rule.status,keywords:lines(rule.keywords),exclude:lines(rule.exclude)}));
      this.saving=true;this.error='';this.notice='';
      try{
        const saved=await this.api(this.endpoint,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(rules)});
        this.editRules(saved);
        this.notice='规则已保存，下次查询时生效';
      }catch(e){this.error=e.message;}
      finally{this.saving=false;}
    }
  },
  template:`<details class="domestic-tracking-rules" @toggle="open">
    <summary>头程物流状态判定规则</summary>
    <p class="scope-note">按顺序匹配最新一条轨迹，每行一个关键词。用“+”连接需要按顺序同时出现的词，例如“已由+收取”。</p>
    <p v-if="loading" role="status">正在读取规则…</p>
    <form v-if="rules" @submit.prevent="save">
      <fieldset v-for="(rule,index) in rules" :key="rule.key">
        <legend>第 {{index+1}} 条规则</legend>
        <label>显示状态<input v-model.trim="rule.status" maxlength="24" required :disabled="saving"></label>
        <label>关键词<textarea v-model="rule.keywords" rows="4" required :disabled="saving"></textarea></label>
        <label>排除词（可选）<textarea v-model="rule.exclude" rows="4" :disabled="saving"></textarea></label>
        <button class="secondary" type="button" :disabled="saving" @click="rules.splice(index,1);notice=''">删除规则</button>
      </fieldset>
      <div class="tracking-rule-actions">
        <button class="secondary" type="button" :disabled="saving||rules.length>=32" @click="add">添加规则</button>
        <button :disabled="saving">保存判定规则</button>
      </div>
    </form>
    <p v-if="notice" role="status">{{notice}}</p>
    <p v-if="error" role="alert" class="failure">{{error}}</p>
  </details>`
};
