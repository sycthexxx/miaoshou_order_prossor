import {computed, nextTick, onMounted, ref} from 'vue/dist/vue.esm-bundler.js';
import './users.css';

export default {
 props:['api','accounts'],
 setup(props){
  const users=ref([]), loading=ref(true), busy=ref(false), error=ref(''), formError=ref('');
  const dialog=ref(null), form=ref(null);
  const accountNames=computed(()=>new Map((props.accounts||[]).map(a=>[a.id,a.name])));
  async function load(){
   try{users.value=await props.api('/users');error.value=''}
   catch(e){error.value=e.message}
   finally{loading.value=false}
  }
  async function edit(user){
   formError.value='';
   form.value={id:user?.id||0,username:user?.username||'',password:'',isAdmin:!!user?.isAdmin,accounts:[...(user?.accounts||[])]};
   await nextTick();
   dialog.value?.showModal();
  }
  function close(){
   if(busy.value)return;
   dialog.value?.close();
   form.value=null;
  }
  async function save(){
   if(busy.value)return;
   busy.value=true;formError.value='';
   const {id,username,password,isAdmin,accounts}=form.value;
   try{
    await props.api(id?'/users/'+id:'/users',{method:id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username,password,isAdmin,accounts:isAdmin?[]:accounts})});
    dialog.value?.close();
    form.value=null;
    await load();
   }catch(e){formError.value=e.message}
   finally{busy.value=false}
  }
  async function remove(user){
   if(busy.value||!window.confirm('删除用户「'+user.username+'」？该用户将退出工作台。'))return;
   busy.value=true;
   try{await props.api('/users/'+user.id,{method:'DELETE'});await load()}
   catch(e){error.value=e.message}finally{busy.value=false}
  }
  onMounted(load);
  return {users,loading,busy,error,formError,form,dialog,accountNames,edit,save,close,remove};
 },
 template:`<section class="panel user-management"><header class="panel-heading"><h2>系统用户</h2><button type="button" @click="edit()">添加用户</button></header>
 <p v-if="error" role="alert" class="failure">{{error}}</p><p v-if="loading">正在读取用户…</p>
 <div v-else class="users-table-wrap"><table><thead><tr><th>用户名</th><th>角色</th><th>可操作的妙手账户</th><th>操作</th></tr></thead>
 <tbody><tr v-for="u in users" :key="u.id"><td><strong>{{u.username}}</strong></td><td>{{u.isAdmin?'管理员':'普通用户'}}</td><td>{{u.isAdmin?'全部账户':u.accounts.map(id=>accountNames.get(id)).filter(Boolean).join('、')||'未分配'}}</td><td><div class="actions"><button type="button" class="secondary" :disabled="busy" @click="edit(u)">编辑</button><button type="button" class="text-button" :disabled="busy" @click="remove(u)">删除</button></div></td></tr></tbody></table></div>
 <dialog v-if="form" ref="dialog" @cancel.prevent="close" aria-labelledby="user-form-title"><form @submit.prevent="save"><h2 id="user-form-title">{{form.id?'编辑用户':'添加用户'}}</h2>
 <label>用户名<input v-model.trim="form.username" required maxlength="60" autocomplete="off" autofocus></label>
 <label>{{form.id?'新密码（留空保留原密码）':'密码'}}<input type="password" v-model="form.password" :required="!form.id" autocomplete="new-password"></label>
 <label class="check"><input v-model="form.isAdmin" type="checkbox">管理员</label>
 <p v-if="form.isAdmin">管理员可管理所有用户和妙手账户。</p>
 <fieldset v-else><legend>可操作的妙手账户</legend><label v-for="a in accounts" :key="a.id" class="check"><input v-model="form.accounts" type="checkbox" :value="a.id">{{a.name}}</label><p v-if="!accounts?.length">请先在账户管理中添加妙手账户。</p></fieldset>
 <p v-if="formError" class="failure" role="alert">{{formError}}</p><footer class="actions"><button type="button" class="secondary" :disabled="busy" @click="close">取消</button><button :disabled="busy">{{busy?'保存中…':'保存'}}</button></footer>
 </form></dialog></section>`
};
