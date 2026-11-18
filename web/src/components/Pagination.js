import {computed,ref,watch} from 'vue/dist/vue.esm-bundler.js';
export default {
 props:{page:{type:Number,default:1},total:{type:Number,default:0},pageSize:{type:Number,default:20},loading:Boolean,unit:{type:String,default:'条'}},emits:['change'],
 setup(props,{emit}){
  const pages=computed(()=>Math.max(1,Math.ceil(props.total/props.pageSize))),target=ref(props.page);
  const numbers=computed(()=>{const set=new Set([1,pages.value]);for(let i=Math.max(1,props.page-2);i<=Math.min(pages.value,props.page+2);i++)set.add(i);const out=[];for(const n of [...set].sort((a,b)=>a-b)){if(out.length&&n-out[out.length-1]>1)out.push('gap-'+n);out.push(n)}return out});
  function go(value){const n=Number(value);if(props.loading||!Number.isInteger(n)||n<1||n>pages.value){target.value=props.page;return}if(n!==props.page)emit('change',n);target.value=n}
  watch(()=>props.page,n=>target.value=n);
  return{pages,numbers,target,go};
 },
 template:`<nav class="ui-pagination" aria-label="列表分页"><div class="pagination-summary">共 <strong>{{total}}</strong> {{unit}}<span class="pagination-divider"></span>每页 {{pageSize}} {{unit}}</div><div class="pagination-controls"><button type="button" class="page-arrow" aria-label="上一页" :disabled="loading||page<=1" @click="go(page-1)">‹</button><template v-for="n in numbers" :key="n"><span v-if="typeof n==='string'" class="page-gap" aria-hidden="true">…</span><button v-else type="button" class="page-number" :class="{selected:page===n}" :aria-label="'第 '+n+' 页'" :aria-current="page===n?'page':undefined" :disabled="loading" @click="go(n)">{{n}}</button></template><button type="button" class="page-arrow" aria-label="下一页" :disabled="loading||page>=pages" @click="go(page+1)">›</button><form class="page-jump" @submit.prevent="go(target)"><label>前往 <input v-model="target" type="number" min="1" :max="pages" :disabled="loading" aria-label="跳转页码"></label><span>页</span><button type="submit" class="secondary" :disabled="loading">跳转</button></form></div></nav>`
};
