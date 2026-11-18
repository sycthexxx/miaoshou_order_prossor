(async()=>{
 if(globalThis.__opProcurementPanel)return;globalThis.__opProcurementPanel=true;
 const {offerId,comparison,reconcile,quantityTracker}=OPProcurement,{scan,path,value,inspect,activeLabels,pageOptions,operatedSpec}=OPProcurementDOM;
 const currentOffer=offerId(location.href);if(!currentOffer)return;
 const answer=await chrome.runtime.sendMessage({type:'GET_PROCUREMENT'}).catch(()=>({ok:false,error:'插件已更新或连接失效，请刷新工作台并重新打开货源'})),task=answer?.task;
 const host=document.createElement('div');host.id='op-procurement-helper';
 host.style.cssText='position:fixed;right:24px;top:180px;z-index:2147483647';document.documentElement.append(host);
 const shadow=host.attachShadow({mode:'closed'});
 const style=document.createElement('style');style.textContent=`
 :host{all:initial;font:14px/1.5 system-ui,-apple-system,"Microsoft YaHei",sans-serif;color:#263d48}*{box-sizing:border-box}button,input{font:inherit}button{cursor:pointer}button:focus-visible,input:focus-visible{outline:2px solid #00a7a3;outline-offset:2px}.bubble{border:0;color:#fff;background:#009f9b;box-shadow:0 5px 20px #003f3f35;border-radius:28px;padding:13px 18px;font-weight:650;touch-action:none;cursor:grab}.panel{width:min(380px,calc(100vw - 16px));background:#fff;border:1px solid #dce5e9;border-radius:16px;box-shadow:0 12px 50px #122c4930;overflow:hidden;margin-top:8px}.head{display:flex;align-items:center;justify-content:space-between;padding:14px 16px;background:#f4faf9;cursor:grab;touch-action:none;font-weight:700}.plain{background:none;border:0;color:#62737e;padding:3px 6px}.body{max-height:min(560px,65vh);overflow:auto;padding:14px 16px}.title{font-weight:650;margin:0 0 8px;overflow-wrap:anywhere}.meta,.note{font-size:12px;color:#788791;margin:8px 0;line-height:1.6}.notice{color:#9b5c16;background:#fff7e8;padding:9px 10px;border-radius:8px;font-size:12px}.row{border:1px solid #e3e9ed;border-radius:10px;padding:11px 12px;margin:9px 0;background:white}@keyframes sku-pulse{0%,100%{box-shadow:0 0 0 0 #00aaa000}35%{box-shadow:0 0 0 4px #00aaa078}}.row.flashing{animation:sku-pulse .65s ease-in-out 3}.row.match{background:#edfaf1;border-color:#a8dbb8}.row.over{background:#fff4ec;border-color:#f2c29d}.spec{font-weight:650;word-break:break-word}.sku-titles{flex:1;min-width:0;overflow-wrap:anywhere}.original-spec{font-size:12px;color:#526d8a;line-height:1.5;margin-bottom:4px;overflow-wrap:anywhere}.sku-titles .spec{color:#263d48;overflow-wrap:anywhere}.sku-heading{display:flex;align-items:center;gap:12px}.sku-image{width:72px;height:88px;flex-shrink:0;object-fit:contain;background:#f5f7f8;border-radius:8px;cursor:zoom-in}.sku-empty{width:72px;flex-shrink:0;color:#8b969d;font-size:12px}.image-view{position:fixed;inset:0;background:#122536bb;display:grid;place-items:center;z-index:2;cursor:zoom-out}.image-view img{max-width:85vw;max-height:85vh;object-fit:contain;background:white;border-radius:10px}.numbers{display:flex;justify-content:space-between;margin:7px 0}.status{font-size:12px;color:#75838d}.match .status{color:#218648}.over .status{color:#b25d18}.tools{display:flex;gap:8px;align-items:center;margin-top:8px}.tools button,.footer button{border:1px solid #dbe4e9;background:white;color:#3f636c;border-radius:6px;padding:4px 8px;font-size:12px}.mapping{margin-top:8px;padding:8px;background:#f6f9fa;border-radius:8px}.mapping select{width:100%;margin:5px 0;padding:7px;border:1px solid #dbe4e9;border-radius:6px;background:white;font:inherit}.mapping button{border:1px solid #dbe4e9;background:white;border-radius:6px;padding:5px 8px;margin:4px 5px 4px 0;cursor:pointer}.color-preview{width:42px;height:50px;object-fit:contain;vertical-align:middle}.alias{width:100%;border:1px solid #dbe4e9;border-radius:6px;padding:6px;margin-top:6px}.footer{display:flex;flex-wrap:wrap;align-items:center;gap:10px;padding:12px 16px;border-top:1px solid #edf1f3;font-size:12px}.footer span{flex:1 1 180px;color:#647781}[hidden]{display:none!important}
 `;shadow.append(style);
 const make=(tag,cls,text)=>{const el=document.createElement(tag);if(cls)el.className=cls;if(text!==undefined)el.textContent=text;return el};
 const bubble=make('button','bubble','采购清单'),panel=make('section','panel');bubble.title='拖动移动，点击展开采购清单';bubble.setAttribute('aria-expanded','false');panel.hidden=true;
 const head=make('div','head','待采购商品 · 0.1.11'),close=make('button','plain','收起');head.append(close);
 const body=make('div','body'),footer=make('div','footer'),summary=make('span','','等待核对'),rescan=make('button','','重新核对'),resetAll=make('button','','清除核对');footer.append(summary,rescan,resetAll);panel.append(head,body,footer);shadow.append(bubble,panel);
 let dragged=false,expanded=false;
 function clamp(){const r=host.getBoundingClientRect();host.style.left=Math.max(8,Math.min(r.left,innerWidth-r.width-8))+'px';host.style.top=Math.max(8,Math.min(r.top,innerHeight-r.height-8))+'px';host.style.right='auto'}
 function toggle(open){expanded=open;panel.hidden=!open;bubble.setAttribute('aria-expanded',String(open));clamp()}
 bubble.addEventListener('click',()=>{if(!dragged)toggle(!expanded)});close.onclick=()=>toggle(false);
 function drag(handle){handle.addEventListener('pointerdown',e=>{if(e.button!==0||e.target===close)return;const r=host.getBoundingClientRect(),x=e.clientX,y=e.clientY;dragged=false;handle.setPointerCapture(e.pointerId);
  const move=ev=>{if(Math.hypot(ev.clientX-x,ev.clientY-y)>4)dragged=true;if(dragged){host.style.left=r.left+ev.clientX-x+'px';host.style.top=r.top+ev.clientY-y+'px';host.style.right='auto';clamp()}};
  const up=()=>{handle.removeEventListener('pointermove',move);handle.removeEventListener('pointerup',up);handle.removeEventListener('pointercancel',up);if(dragged)void chrome.storage.local.set({position:{left:host.offsetLeft,top:host.offsetTop}})};
  handle.addEventListener('pointermove',move);handle.addEventListener('pointerup',up);handle.addEventListener('pointercancel',up);
 })}drag(bubble);drag(head);window.addEventListener('resize',clamp);
 const bindingKey='bindings:'+currentOffer;
 const saved=await chrome.storage.local.get(['position',bindingKey]);
 const legacyBindings=saved[bindingKey],selectors={};let mappingState=OPMappings.empty(),bindings={};
 if(saved.position){host.style.left=saved.position.left+'px';host.style.top=saved.position.top+'px';host.style.right='auto';clamp()}
 const connectionTools=make('div','tools'),retryLookup=make('button','','连接工作台并查询'),changeContext=make('a','','切换查询账户');
 if(answer?.connectURL){changeContext.href=answer.connectURL+'/#/procurement';changeContext.target='_blank';changeContext.rel='noopener noreferrer';connectionTools.append(changeContext)}
 retryLookup.onclick=async()=>{retryLookup.disabled=true;retryLookup.textContent='正在查询…';try{const r=await chrome.runtime.sendMessage({type:'GET_PROCUREMENT',reload:true,connect:true});if(!r?.ok)throw Error(r?.error||'查询失败');location.reload()}catch(e){retryLookup.textContent=e.message;retryLookup.disabled=false}};
 if(!task||task.direct)connectionTools.append(retryLookup);
 body.append(connectionTools);
 if(!task){body.append(make('p','title','直接打开货源查询'),make('p','notice',answer?.error||answer?.reason||'请从采购汇总重新打开货源'),make('p','note','在这台电脑打开公司工作台，再点击采购助手图标，保存并授权工作台实际地址（不是员工电脑的 localhost）。回到工作台确认“采购助手已连接”。'));summary.textContent='按所选账户查询采购缓存';toggle(true);return}
 const requiredQuantity=task.variants.reduce((sum,v)=>sum+v.quantity,0);
 if(task.direct)body.append(make('p','meta',`直接查询 · ${answer.connectURL} · ${task.gifts?'包含':'不含'}赠品`));
 body.append(make('p','title',task.title),make('p','meta',`货源 ${task.offerId} · 账户 #${task.account} · ${new Date(task.createdAt).toLocaleString()}`));
 body.append(make('p','note','绿色 = 数量核对一致，不代表已下单。切换颜色保留已核对结果，切回自动更新。若在其他位置清空选择，请点“清除核对”。SKU图来自本地订单，点击可放大。'));
 if(Date.now()-(typeof task.createdAt==='number'?task.createdAt:Date.parse(task.createdAt))>86400000)body.append(make('p','notice','这是超过一天的采购快照，请刷新采购汇总并重新打开货源。'));
 const notice=make('p','notice','无法识别时，先展开商品规格，并选中颜色；可以填写对应的 1688 规格，再指定数量框。');body.append(notice);
 const supplierBox=make('div','mapping'),supplierTitle=make('strong','','厂家 / 店铺'),supplierInput=make('input','alias'),supplierStatus=make('p','note','正在读取页面厂家名称…'),readSupplier=make('button','','重新读取'),saveSupplier=make('button','','回传厂家');
 supplierInput.placeholder='未识别时可手动填写厂家名称';supplierInput.setAttribute('aria-label','厂家或店铺名称');supplierInput.maxLength=120;
 supplierBox.append(supplierTitle,supplierInput,readSupplier,saveSupplier,supplierStatus);body.append(supplierBox);
 let supplierDone=false,supplierBusy=false,supplierEdited=false,supplierCandidate='',supplierStable=0,supplierAttempts=0;
 async function reportSupplier(name){
  if(supplierBusy||offerId(location.href)!==task.offerId)return;
  const cleaned=OPSupplier.cleanName(name);if(!cleaned){supplierStatus.textContent='请输入有效的厂家名称';return}
  supplierBusy=true;saveSupplier.disabled=true;supplierStatus.textContent=`正在回传「${cleaned}」…`;
  try{
   const reply=await chrome.runtime.sendMessage({type:'REPORT_SUPPLIER',name:cleaned});
   if(!reply?.ok)throw Error(reply?.error||'厂家回传失败');
   const result=reply.result;
   supplierStatus.textContent=OPSupplier.supplierResultText(result);
   supplierDone=true;
  }catch(e){supplierStatus.textContent=e.message+'；可确认后点击“回传厂家”重试。';supplierEdited=true}
  finally{supplierBusy=false;saveSupplier.disabled=false}
 }
 function readSupplierPage(auto=false){
  if(offerId(location.href)!==task.offerId||supplierBusy)return;
  const reading=OPSupplier.supplierReading(document,task.offerId),names=reading.automaticName?[reading.automaticName]:reading.names;
  if(names.length!==1){supplierStable=0;supplierCandidate='';if(!supplierEdited)supplierStatus.textContent=names.length?'页面有多个厂家候选，请核对后手动填写回传。':'暂未识别到明确厂家名称，可手动填写，或等待页面加载后重新读取。';return}
  const name=names[0];if(!supplierEdited)supplierInput.value=name;
  if(name===supplierCandidate)supplierStable++;else{supplierCandidate=name;supplierStable=1}
  if(auto&&!supplierEdited&&supplierStable>=2&&reading.automaticName===name)void reportSupplier(name);
  else if(auto&&!supplierEdited&&!reading.automaticName)supplierStatus.textContent='读到页面候选名称，但无法确认属于当前商品，请核对后点击“回传厂家”。';
  else if(!auto){supplierInput.value=name;supplierStatus.textContent='已读取页面名称，点击“回传厂家”保存。'}
 }
 supplierInput.addEventListener('input',()=>{supplierEdited=true});
 readSupplier.onclick=()=>{supplierEdited=true;readSupplierPage(false)};
 saveSupplier.onclick=()=>{supplierEdited=true;void reportSupplier(supplierInput.value)};
 readSupplierPage(true);
 const supplierTimer=setInterval(()=>{
  if(supplierDone||supplierEdited||supplierAttempts>=20){clearInterval(supplierTimer);return}
  if(!document.hidden){supplierAttempts++;readSupplierPage(true)}
 },2000);
 window.addEventListener('pagehide',()=>clearInterval(supplierTimer),{once:true});
 const rows=new Map(),ledger=new Map(),mappingDrafts=new Set();let picking=null;
 const mappingBox=make('div','mapping'),mappingStatus=make('p','note','正在从工作台读取采购专用映射…'),mappingReload=make('button','','读取服务器映射'),mappingRetry=make('button','','重试保存'),importLegacy=make('button','','导入本机旧映射');
 mappingBox.append(make('strong','','采购专用映射'),mappingStatus,mappingReload,mappingRetry,importLegacy);body.append(mappingBox);
 let mappingRevision=null,mappingReady=false,mappingLoading=false,mappingSaving=false,mappingDirty=false,mappingConflict=false;
 bindings=OPMappings.effective(mappingState,task.variants,selectors);
 function mappingControls(){
  const disabled=!mappingReady||mappingLoading||mappingSaving||mappingConflict;
  for(const item of rows.values())for(const control of item.mappingControls)control.disabled=disabled;
  mappingReload.disabled=mappingLoading||mappingSaving;
  mappingReload.textContent=mappingDirty||mappingDrafts.size?'读取服务器（放弃未保存修改）':'读取服务器映射';
  mappingRetry.hidden=!mappingDirty;mappingRetry.disabled=disabled;
  importLegacy.hidden=!mappingReady||mappingRevision!==0||!legacyBindings||!Object.keys(legacyBindings).length;importLegacy.disabled=disabled;
 }
 function refreshBindings(change,discardDrafts=false){
  if(discardDrafts)mappingDrafts.clear();
  const before=bindings,next=OPMappings.effective(mappingState,task.variants,selectors);
  for(const v of task.variants){
   const scoped=change&&(change.scope==='sku'?change.spec===v.spec:change.color===OPMappings.colorKey(v.spec)&&(!mappingState.skus[v.spec]?.exception||change.reset));
   if(scoped||!OPMappings.equivalent(before[v.spec]?.alias||v.spec,next[v.spec].alias)){delete selectors[v.spec];ledger.delete(v.spec);mappingDrafts.delete(v.spec)}
  }
  bindings=OPMappings.effective(mappingState,task.variants,selectors);
  for(const item of rows.values())item.refreshMapping();
  last='';update();mappingControls();
 }
 function readMappingResult(result){
  if(!result||!Number.isSafeInteger(result.revision)||result.revision<0||result.mapping?.version!==2)throw Error('采购映射响应异常，请更新工作台后端');
  return result;
 }
 async function loadMappings(){
  if(mappingLoading||mappingSaving)return;
  mappingLoading=true;mappingStatus.textContent='正在从工作台读取采购专用映射…';mappingControls();
  try{
   const response=await chrome.runtime.sendMessage({type:'GET_PROCUREMENT_MAPPING'});if(!response?.ok)throw Error(response?.error||'读取采购映射失败');
   const result=readMappingResult(response.result);mappingState=OPMappings.migrate(result.mapping);mappingRevision=result.revision;mappingReady=true;mappingDirty=false;mappingConflict=false;
   refreshBindings(null,true);mappingStatus.textContent=mappingRevision?`已读取服务器采购映射 · 版本 ${mappingRevision}`:'服务器暂无采购映射；改一次颜色会应用同色所有尺码并保存。';
  }catch(e){mappingReady=false;mappingStatus.textContent=e.message+'。未读取成功前不能保存，避免覆盖服务器配置。'}
  finally{mappingLoading=false;mappingControls()}
 }
 async function saveMappings(){
  if(!mappingReady||mappingLoading||mappingSaving||mappingConflict||!mappingDirty)return;
  mappingSaving=true;mappingStatus.textContent='正在保存采购专用映射…';mappingControls();
  try{
   const response=await chrome.runtime.sendMessage({type:'PUT_PROCUREMENT_MAPPING',revision:mappingRevision,mapping:OPMappings.migrate(mappingState)});
   if(!response?.ok){if(response?.status===409){mappingConflict=true;throw Error('其他电脑已修改此货源映射，请读取服务器最新版本后重新修改')};throw Error(response?.error||'保存采购映射失败')}
   const result=readMappingResult(response.result);mappingState=OPMappings.migrate(result.mapping);mappingRevision=result.revision;mappingDirty=false;
   refreshBindings();mappingStatus.textContent=`采购映射已保存到服务器 · 版本 ${mappingRevision}，不会被商品同步覆盖。`;
  }catch(e){mappingStatus.textContent=e.message+'。未保存的修改仅在本页生效。'}
  finally{mappingSaving=false;mappingControls()}
 }
 function changeMappings(action){
  if(!mappingReady||mappingLoading||mappingSaving||mappingConflict)return;
  try{const change=action();if(change?.spec)mappingDrafts.delete(change.spec);mappingDirty=true;refreshBindings(change);void saveMappings()}catch(e){mappingStatus.textContent=e.message}
 }
 mappingReload.onclick=()=>{void loadMappings()};mappingRetry.onclick=()=>{void saveMappings()};
 importLegacy.onclick=()=>{if(mappingRevision!==0)return;changeMappings(()=>{mappingState=OPMappings.migrate(legacyBindings);return null})};
 window.addEventListener('focus',()=>{if(mappingReady&&!mappingDirty&&!mappingDrafts.size&&!mappingLoading&&!mappingSaving)void loadMappings()});
 for(const v of task.variants){
  const row=make('div','row'),spec=make('div','spec',OPMappings.aliasFor(mappingState,v.spec)),numbers=make('div','numbers'),required=make('span','',`需 ${v.quantity} 件`),selected=make('b','','已选 —'),status=make('div','status','待核对');numbers.append(required,selected);
  const tools=make('div','tools'),pick=make('button','','指定数量框'),edit=make('button','','规格映射'),clear=make('button','','重置此规格');tools.append(pick,edit,clear);
  const mapping=make('div','mapping');mapping.hidden=true;
  const colorSelect=make('select'),sizeSelect=make('select'),refreshOptions=make('button','','读取页面规格'),applyOptions=make('button','','应用选择'),resetColor=make('button','','重置同色映射'),onlyThis=make('input'),scopeLabel=make('label','note'),hint=make('div','note'),preview=make('img','color-preview');
  onlyThis.type='checkbox';onlyThis.checked=!!mappingState.skus[v.spec]?.exception;scopeLabel.append(onlyThis,document.createTextNode(' 仅此规格例外（不修改同色其他尺码）'));resetColor.hidden=!OPMappings.colorKey(v.spec);preview.hidden=true;preview.referrerPolicy='no-referrer';preview.onerror=()=>{preview.hidden=true};
  colorSelect.setAttribute('aria-label','1688颜色');sizeSelect.setAttribute('aria-label','1688尺码');
  const alias=make('input','alias');alias.value=bindings[v.spec]?.alias||v.spec;alias.placeholder='手动填写，例如：酒红色,M';alias.setAttribute('aria-label',v.spec+' 对应的1688规格');
  let choices={colors:[],sizes:[]};
  function fill(select,list,prompt,current){select.replaceChildren();const empty=make('option','',prompt);empty.value='';select.append(empty);for(const item of list){const option=make('option','',item.label);option.value=item.label;select.append(option)}select.value=list.find(x=>OPProcurement.normalize(x.label)===OPProcurement.normalize(current))?.label||''}
  function showPreview(){const image=choices.colors.find(x=>x.label===colorSelect.value)?.image;preview.hidden=!image;if(image)preview.src=image}
  function readOptions(){choices=pageOptions(document);const parts=alias.value.split(/[,，;；|/]+/).map(s=>s.trim());fill(colorSelect,choices.colors,'选择1688颜色',parts[0]);fill(sizeSelect,choices.sizes,'选择1688尺码',parts[1]);showPreview();hint.textContent=`读取到 ${choices.colors.length} 种颜色、${choices.sizes.length} 个当前可见尺码。选择后点击应用；读取不到可手动填写。`}
  function commit(){changeMappings(()=>OPMappings.apply(mappingState,v.spec,alias.value.trim()||v.spec,onlyThis.checked))}
  function refreshMapping(){spec.textContent=OPMappings.aliasFor(mappingState,v.spec);if(mappingDrafts.has(v.spec))return;alias.value=bindings[v.spec]?.alias||v.spec;onlyThis.checked=!!mappingState.skus[v.spec]?.exception;const p=OPMappings.parts(alias.value);fill(colorSelect,choices.colors,'选择1688颜色',p[0]);fill(sizeSelect,choices.sizes,'选择1688尺码',p[1]);showPreview()}
  function markDraft(){mappingDrafts.add(v.spec);mappingControls();hint.textContent='当前编辑尚未应用；选择后点击应用，手动输入离开输入框后保存。'}
  alias.addEventListener('input',markDraft);alias.onchange=commit;colorSelect.onchange=()=>{markDraft();showPreview()};sizeSelect.onchange=markDraft;onlyThis.onchange=markDraft;refreshOptions.onclick=readOptions;
  applyOptions.onclick=()=>{if(!colorSelect.value||!sizeSelect.value){hint.textContent='请选择颜色和尺码；非颜色/尺码规格可直接手动填写。';return}alias.value=colorSelect.value+','+sizeSelect.value;const scoped=onlyThis.checked;commit();hint.textContent=scoped?'已应用本页，仅当前规格例外；服务器保存状态见上方。':'已应用本页：颜色同步同色所有尺码，尺码仅当前规格（已有明确例外除外）；服务器保存状态见上方。'};
  mapping.append(refreshOptions,colorSelect,preview,sizeSelect,scopeLabel,applyOptions,resetColor,hint,make('div','note','默认共享同色映射；也可手动填写整规格：'),alias);
  edit.onclick=()=>{mapping.hidden=!mapping.hidden;if(!mapping.hidden)readOptions()};
  clear.title='取消此规格例外、恢复原尺码，继续使用同色共享颜色';clear.onclick=()=>changeMappings(()=>OPMappings.reset(mappingState,v.spec));
  resetColor.title='清除该原颜色全部映射、尺码修改及例外';resetColor.onclick=()=>changeMappings(()=>({...OPMappings.reset(mappingState,v.spec,true),reset:true}));
  pick.onclick=()=>{picking=v;notice.textContent=`正在指定「${v.spec}」：请点击 1688 对应数量输入框或数字。不会修改数量；按 Esc 取消。`;};
  const heading=make('div','sku-heading'),titles=make('div','sku-titles'),original=make('div','original-spec','原规格：'+v.spec);titles.append(original,spec);heading.append(titles);
  if(v.images?.length){
   const img=make('img','sku-image');img.alt=v.spec+' SKU图';img.referrerPolicy='no-referrer';img.loading='lazy';let imageIndex=0;
   img.onerror=()=>{if(++imageIndex<v.images.length)img.src=v.images[imageIndex];else img.replaceWith(make('span','sku-empty','SKU图加载失败'))};img.src=v.images[0];
   img.onclick=()=>{const overlay=make('div','image-view'),large=make('img');large.referrerPolicy='no-referrer';large.src=img.src;large.alt=img.alt;overlay.append(large);overlay.onclick=()=>overlay.remove();shadow.append(overlay)};heading.prepend(img);
  }else heading.prepend(make('span','sku-empty','暂无SKU图'));
  row.append(heading,numbers,status,tools,mapping);body.append(row);rows.set(v.spec,{row,selected,status,refreshMapping,mappingControls:[alias,colorSelect,sizeSelect,onlyThis,applyOptions,resetColor,clear,pick]});
 }
 document.addEventListener('keydown',e=>{if(e.key==='Escape'&&picking){picking=null;notice.textContent='已取消指定数量框。'}},true);
 document.addEventListener('click',e=>{
  if(!picking||e.composedPath().includes(host))return;
  e.preventDefault();e.stopImmediatePropagation();
  const el=e.target.closest?.('input,[role=spinbutton]')||e.target,v=picking,alias=bindings[v.spec]?.alias||v.spec;
  if(value(el)===null){notice.textContent='这里不是可读取的数量，请点击实际数量框。按 Esc 可取消。';return}
  if(!inspect(el,alias,activeLabels(document))){notice.textContent='无法同时确认颜色/尺码。请先选中对应颜色，检查“规格映射”与页面文字完全一致，再指定数量框。按 Esc 可取消。';return}
  selectors[v.spec]={alias,selector:path(el)};bindings=OPMappings.effective(mappingState,task.variants,selectors);picking=null;notice.textContent='已指定。插件只读取数量，不修改页面、不下单。';update();
 },true);
 const trackQuantity=quantityTracker();
 let last='',timer;
 function update(){
  const valid=offerId(location.href)===currentOffer;
  const result=valid?scan(document,task.variants,bindings):new Map();
  const changes=trackQuantity(result,valid?task.variants.map(v=>OPProcurement.tokens(bindings[v.spec]?.alias||v.spec)[0]).filter(color=>activeLabels(document).has(color)).sort().join('|'):'other-offer');
  if(!document.hidden&&!picking&&changes.length)focusQuantity(changes[changes.length-1]);
  const checked=reconcile(task.variants,result,ledger,activeLabels(document),bindings,valid);
  const signature=JSON.stringify([...checked]);if(signature===last)return;last=signature;
  let matches=0;
  for(const v of task.variants){const {quantity:n,cached}=checked.get(v.spec),r=rows.get(v.spec);const state=comparison(v.quantity,n);r.row.className='row '+state.kind+(r.row.classList.contains('flashing')?' flashing':'');r.selected.textContent=n===null?'已选 —':`已选 ${n}`;r.status.textContent=state.text+(cached?' · 已核对（其他颜色）':'');if(state.kind==='match')matches++}
  summary.textContent=valid?`${matches}/${task.variants.length} 个规格数量一致 · 共需 ${requiredQuantity} 件`:'已切换货源，请重新从采购汇总打开';bubble.textContent=`采购清单 ${matches}/${task.variants.length}`;
 }
 function schedule(){clearTimeout(timer);timer=setTimeout(update,150)}
 rescan.onclick=()=>{last='';update()};
 resetAll.onclick=()=>{ledger.clear();last='';update()};
 document.addEventListener('visibilitychange',()=>{if(!document.hidden)update()});
 window.addEventListener('focus',()=>update());
 let flashTimer,flashedRow;
 function focusQuantity(spec){
  // Trigger on interaction, independently of quantity value or comparison.
   if(offerId(location.href)!==currentOffer)return;
   toggle(true);
   const row=rows.get(spec)?.row;if(!row)return;
   // Scroll only this panel, including when the merchant used an icon button
   // or replaced its quantity input during a React render.
   const r=row.getBoundingClientRect(),b=body.getBoundingClientRect();
   body.scrollBy({top:r.top-b.top-(b.height-Math.min(r.height,b.height))/2,behavior:'smooth'});
   clearTimeout(flashTimer);flashedRow?.classList.remove('flashing');
   row.classList.remove('flashing');void row.offsetWidth;row.classList.add('flashing');flashedRow=row;
   flashTimer=setTimeout(()=>row.classList.remove('flashing'),2000);
 }
 function followQuantity(event){
  if(event.composedPath().includes(host)||picking||!event.isTrusted)return;
  const spec=operatedSpec(document,event.target,task.variants,bindings);
  if(spec)focusQuantity(spec);
  // A DOM value change is the fallback for unlabelled/custom +/- controls.
  schedule();
 }
 document.addEventListener('pointerdown',followQuantity,true);
 document.addEventListener('input',followQuantity,true);
 document.addEventListener('change',followQuantity,true);
 document.addEventListener('click',followQuantity,true);
 document.addEventListener('input',schedule,true);document.addEventListener('change',schedule,true);document.addEventListener('click',schedule,true);
 const observer=new MutationObserver(schedule);observer.observe(document.body,{subtree:true,childList:true,attributes:true,characterData:true});
 // Some frameworks update input.value without a DOM mutation or input event.
 const poll=setInterval(()=>{if(!document.hidden)update()},800);
 window.addEventListener('pagehide',()=>{clearInterval(poll);clearTimeout(timer);clearTimeout(flashTimer);observer.disconnect()},{once:true});
 update();toggle(true);mappingControls();void loadMappings();
})();
