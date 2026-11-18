import {ref,useId,watch} from 'vue/dist/vue.esm-bundler.js';

export default {
 props:['api','account','number','company'],
 emits:['update:number','update:company'],
 setup(props,{emit}){
  const hint=ref('');
  const listId=useId();
  let revision=0;
  function changeNumber(event){
   const number=event.target.value.trim();
   if(number===props.number)return;
   revision++;
   emit('update:number',number);
   emit('update:company','');
  }
  function changeCompany(event){
   revision++;
   hint.value='';
   emit('update:company',event.target.value.trim());
  }
  watch(()=>[props.account,props.number],([account,number],previous,onCleanup)=>{
   const current=++revision;
   hint.value='';
   if(!account||!number||props.company)return;
   const controller=new AbortController();
   const timer=setTimeout(async()=>{
    if(current!==revision)return;
    hint.value='正在识别物流公司…';
    try{
     const result=await props.api('/accounts/'+account+'/logistics-company?logisticsNo='+encodeURIComponent(number),{signal:controller.signal});
     if(current!==revision)return;
     if(result.company)emit('update:company',result.company);
     hint.value=result.company?'':'未识别到物流公司，请手动填写。';
    }catch{
     if(current===revision)hint.value='暂时无法识别，请手动填写物流公司。';
    }
   },450);
   onCleanup(()=>{revision++;clearTimeout(timer);controller.abort()});
  });
  return {changeNumber,changeCompany,hint,listId};
 },
 template:`<label>国内快递单号<input :value="number" @input="changeNumber" required placeholder="请输入国内快递单号"></label>
 <label>物流公司<input :value="company" @input="changeCompany" required placeholder="请选择或输入物流公司" :list="listId"><small v-if="hint" role="status">{{hint}}</small></label>
 <datalist :id="listId"><option v-for="name in ['中通快递','圆通速递','申通快递','韵达快递','顺丰速运','极兔速递','邮政快递包裹','京东物流']" :key="name" :value="name"></option></datalist>`
};
