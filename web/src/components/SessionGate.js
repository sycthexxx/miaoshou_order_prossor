import {ref,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';

async function request(url,options){
 const response=await fetch('/api'+url,options);
 if(!(response.headers.get('content-type')||'').includes('application/json'))throw new Error('服务暂时不可用，请稍后重试');
 const data=await response.json();
 if(!response.ok)throw Object.assign(new Error(data.error||'请求失败'),{status:response.status,browserMode:data.browserMode});
 return data;
}

export default {
 setup(){
  const user=ref(null),ready=ref(false),busy=ref(false),message=ref(''),username=ref(''),password=ref('');
  let revision=0,timer,checking=false;
  const channel='order-workbench-session-changed';
  function setUser(next){
   if(next?.id===user.value?.id&&next?.isAdmin===user.value?.isAdmin&&next?.username===user.value?.username)return;
   revision++;
   user.value=next;
   password.value='';
  }
  async function checkSession(){
   if(checking||busy.value)return;
   checking=true;
   const version=revision;
   try{
    const next=await request('/session');
    if(version===revision){setUser(next);message.value=''}
   }catch(e){
    if(version===revision){
     if(e.status===401)setUser(null);
     else if(!user.value)message.value=e.message;
    }
   }finally{ready.value=true;checking=false}
  }
  function notifyTabs(){try{localStorage.setItem(channel,String(Date.now()))}catch{}}
  async function login(){
   if(busy.value)return;
   ++revision;busy.value=true;message.value='';
   try{
    const next=await request('/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:username.value.trim(),password:password.value})});
    setUser(next);notifyTabs();
   }catch(e){message.value=e.message}finally{busy.value=false}
  }
  async function logout(){
   if(busy.value)return;
   ++revision;busy.value=true;message.value='';
   try{await request('/logout',{method:'POST'});setUser(null);notifyTabs()}
   catch(e){if(e.status===401)setUser(null);else message.value=e.message}
   finally{busy.value=false}
  }
  async function api(url,options){
   const version=revision;
   try{return await request(url,options)}
   catch(e){
    if(e.status===401&&version===revision){setUser(null);message.value='请重新登录工作台'}
    throw e
   }
  }
  function sessionChanged(event){if(event.key===channel)checkSession()}
  onMounted(()=>{
   checkSession();timer=setInterval(checkSession,5000);
   window.addEventListener('focus',checkSession);window.addEventListener('storage',sessionChanged);
  });
  onUnmounted(()=>{clearInterval(timer);window.removeEventListener('focus',checkSession);window.removeEventListener('storage',sessionChanged)});
  return {user,ready,busy,message,username,password,login,logout,api};
 },
 template:`<section v-if="!ready" class="login-screen" role="status">正在打开工作台…</section>
 <section v-else-if="!user" class="login-screen"><form class="login-card" @submit.prevent="login">
  <div class="login-brand"><span class="brand-mark">OP</span><h1>登录订单工作台</h1></div>
  <label>用户名<input v-model="username" autocomplete="username" autocapitalize="none" required autofocus></label>
  <label>密码<input v-model="password" type="password" autocomplete="current-password" required></label>
  <p v-if="message" role="alert" class="error">{{message}}</p>
  <button type="submit" :disabled="busy||!username.trim()||!password">{{busy?'登录中…':'登录'}}</button>
 </form></section>
 <template v-else><p v-if="message" class="session-error error" role="alert">{{message}}</p><slot :user="user" :api="api" :logout="logout" :busy="busy"/></template>`
};
