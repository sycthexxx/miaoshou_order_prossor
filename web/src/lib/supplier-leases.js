import {ref,watch,onMounted,onUnmounted} from 'vue/dist/vue.esm-bundler.js';

export function useSupplierLeases(props,onLost,reportError){
 const owners=ref({}),held=ref({}),pending=ref({}),ready=ref(false);
 let generation=0,revision=0,refreshingFor=-1,timer;
 const base=account=>'/accounts/'+account+'/supplier-locks';
 const headers=lease=>({'X-Supplier-Lease':String(lease.leaseId)});
 async function refresh(){
  const account=props.account,version=generation,start=revision;
  if(!account||refreshingFor===version)return;
  refreshingFor=version;
  try{
   const list=await props.api(base(account));
   if(version!==generation||start!==revision)return;
   const nextHeld={},nextOwners={};
   for(const lease of list){
    nextOwners[lease.supplierId]=lease.username;
    if(lease.leaseId>0)nextHeld[lease.supplierId]=lease;
   }
   for(const [id,lease] of Object.entries(held.value)){
    if(lease.leaseId!==nextHeld[id]?.leaseId)onLost(id);
   }
   owners.value=nextOwners;held.value=nextHeld;ready.value=true;
  }catch(e){
   if(version===generation&&!ready.value)reportError(e.message);
  }finally{if(refreshingFor===version)refreshingFor=-1}
 }
 async function acquire(id){
  const account=props.account,version=generation;
  if(!account||pending.value[id])return null;
  const old=held.value[id];
  pending.value[id]=true;
  try{
   const lease=await props.api(base(account)+'/'+id,{method:'POST',headers:old?headers(old):{}});
   if(version!==generation)return null;
   held.value[id]=lease;owners.value[id]=lease.username;revision++;
   return lease;
  }catch(e){
   if(version===generation){reportError(e.message);void refresh()}
   return null;
  }finally{if(version===generation)delete pending.value[id]}
 }
 async function release(id){
  const account=props.account,version=generation,lease=held.value[id];
  if(!lease||pending.value[id])return;
  pending.value[id]=true;
  try{
   await props.api(base(account)+'/'+id,{method:'DELETE',headers:headers(lease)});
   if(version===generation&&held.value[id]?.leaseId===lease.leaseId){
    delete held.value[id];delete owners.value[id];revision++;onLost(id);
   }
  }catch(e){
   if(version===generation){reportError(e.message);void refresh()}
  }finally{if(version===generation)delete pending.value[id]}
 }
 watch(()=>props.account,()=>{
  generation++;
  owners.value={};held.value={};pending.value={};ready.value=false;
  void refresh();
 },{immediate:true,flush:'sync'});
 onMounted(()=>{timer=setInterval(refresh,3000);window.addEventListener('focus',refresh)});
 onUnmounted(()=>{generation++;clearInterval(timer);window.removeEventListener('focus',refresh)});
 return {owners,held,pending,ready,acquire,release};
}
