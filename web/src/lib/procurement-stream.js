// HTTP responses and WebSocket snapshots share the same per-scope revision.
export function procurementSnapshotGate(account,gifts){
 let revision=-1;
 return (summary,incomingAccount,incomingGifts)=>{
  if(String(incomingAccount)!==String(account)||incomingGifts!==gifts||!Array.isArray(summary?.groups)||!Number.isSafeInteger(summary.revision)||summary.revision<0||summary.revision<=revision)return false;
  revision=summary.revision;return true;
 };
}

export function procurementStream({account,gifts,onSnapshot,onStatus,origin=location.origin,Socket=WebSocket,schedule=setTimeout,cancel=clearTimeout}){
 let disposed=false,socket,timer,attempt=0;
 const url=new URL(`/api/accounts/${encodeURIComponent(account)}/procurement/ws`,origin);
 url.protocol=url.protocol==='https:'?'wss:':'ws:';url.searchParams.set('gifts',gifts?'1':'0');
 function retry(){if(disposed||timer)return;onStatus('实时连接中断，正在重连；重新连接后读取完整汇总。');timer=schedule(()=>{timer=null;connect()},Math.min(15000,1000*2**Math.min(attempt++,4)))}
 function connect(){
  if(disposed)return;
  onStatus('正在连接采购汇总实时更新…');
  let current;try{current=new Socket(url.href);socket=current}catch{retry();return}
  current.onmessage=event=>{
   if(disposed||socket!==current)return;
   let message;try{message=JSON.parse(event.data)}catch{return}
   if(message.type!=='procurement_snapshot'||String(message.account)!==String(account)||message.gifts!==gifts)return;
   attempt=0;onStatus('采购汇总实时更新已连接');onSnapshot(message.summary,message.account,message.gifts);
  };
  current.onerror=()=>{if(!disposed&&socket===current)current.close()};
  current.onclose=()=>{if(!disposed&&socket===current)retry()};
 }
 connect();
 return ()=>{disposed=true;if(timer)cancel(timer);if(socket){socket.onmessage=null;socket.onerror=null;socket.onclose=null;socket.close()}};
}
