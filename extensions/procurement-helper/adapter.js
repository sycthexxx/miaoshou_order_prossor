(()=>{
 const {normalize,tokens,quantity}=OPProcurement;
 const inputSelector='input:not([type=hidden]):not([type=checkbox]):not([type=radio]):not([type=submit]):not([type=button]),[role=spinbutton]';
 function visible(el){return !!el&&el.isConnected&&el.getClientRects().length>0&&getComputedStyle(el).visibility!=='hidden'}
 function value(el){return quantity(el.matches('input')?el.value:el.getAttribute('aria-valuenow')??el.textContent)}
 function labels(el){return new Set([...el.querySelectorAll('*')].filter(x=>!x.children.length&&!x.matches('input,script,style')).map(x=>normalize(x.textContent)).filter(x=>x&&x.length<100))}
 function selected(el){return el.getAttribute('aria-selected')==='true'||el.getAttribute('aria-pressed')==='true'||el.getAttribute('data-state')==='checked'||/(^|[\s_-])(selected|active|checked)([\s_-]|$)/i.test(el.className||'')}
 function activeLabels(doc){const out=new Set();for(const el of doc.querySelectorAll('[aria-selected=true],[aria-pressed=true],[data-state=checked],[class*=selected],[class*=active],[class*=checked]')){if(!visible(el)||!selected(el))continue;for(const x of labels(el))out.add(x);const text=normalize(el.textContent);if(text.length<100)out.add(text)}return out}
 function rowFor(input,wanted){let el=input.parentElement;for(let i=0;el&&i<6;i++,el=el.parentElement){const fields=[...el.querySelectorAll(inputSelector)].filter(visible);if(fields.length>1)return null;const names=labels(el);if(wanted.some(t=>names.has(t)))return {el,names}}return null}
 function path(el){const parts=[];while(el&&el!==document.documentElement){const tag=el.tagName.toLowerCase();const index=[...el.parentElement.children].filter(x=>x.tagName===el.tagName).indexOf(el)+1;parts.unshift(`${tag}:nth-of-type(${index})`);el=el.parentElement}return 'html>'+parts.join('>')}
 function inspect(input,spec,active,identityOnly=false){const wanted=tokens(spec);if(!wanted.length||!visible(input))return null;const row=rowFor(input,wanted);if(!row)return null;
 // Every specification component must be present as an exact row label or
 // an explicitly selected option. A substring/fuzzy match never turns green.
 if(!wanted.every(t=>row.names.has(t)||active.has(t)))return null;
 const n=value(input);return n===null&&!identityOnly?null:{input,quantity:n};
 }
 function scanMatches(doc,variants,bindings={},identityOnly=false){
 const active=activeLabels(doc),inputs=[...doc.querySelectorAll(inputSelector)].filter(visible),results=new Map(),owners=new Map();
 for(const v of variants){const b=bindings[v.spec],spec=b?.alias||v.spec;let candidates=[];
  if(b?.selector){try{const el=doc.querySelector(b.selector);const hit=el&&inspect(el,spec,active,identityOnly);if(hit)candidates=[hit]}catch{}}
  else candidates=inputs.map(el=>inspect(el,spec,active,identityOnly)).filter(Boolean);
  if(candidates.length===1){const hit=candidates[0];results.set(v.spec,hit);const prior=owners.get(hit.input)||[];prior.push(v.spec);owners.set(hit.input,prior)}
 }
 // One 1688 row cannot satisfy two different demand specifications.
 for(const specs of owners.values())if(specs.length>1)for(const spec of specs)results.delete(spec);
 return results;
 }
 function scan(doc,variants,bindings={}){const results=scanMatches(doc,variants,bindings);return new Map(variants.map(v=>[v.spec,results.get(v.spec)?.quantity??null]))}
 function quantityControl(target){
  if(!target)return null;
  const direct=target.closest?.('input,[role=spinbutton]');if(direct)return direct;
  // A color button is not a quantity action. Only +/- controls are considered.
  const button=target.closest?.('button,[role=button]')||target.closest?.('svg')?.parentElement||target;
  const text=String(button.textContent||'').trim(),aria=button.getAttribute?.('aria-label')||'';
  const iconButton=!text&&(button.tagName==='BUTTON'||button.getAttribute?.('role')==='button'||button.querySelector?.('svg'));
  if(!iconButton&&!/^[+＋−–—－-]$/.test(text)&&!/^(增加|减少|加|减|increment|decrement)(数量)?$/i.test(aria))return null;
  let parent=button.parentElement;
  for(let i=0;parent&&i<5;i++,parent=parent.parentElement){const fields=[...parent.querySelectorAll(inputSelector)].filter(visible);if(fields.length>1)return null;if(fields.length===1)return fields[0]}
  return null;
 }
 function operatedSpec(doc,target,variants,bindings={}){
  const input=quantityControl(target);if(!input)return null;
  const matches=scanMatches(doc,variants,bindings,true);
  for(const [spec,hit] of matches)if(hit.input===input)return spec;
  return null;
 }
 function optionText(text){
  const s=String(text||'').trim();
  if(!s||s.length>60||/^[\d\s.+−-]+$/.test(s)||/[¥￥]|库存|已选|合计|立即下单|采购车/.test(s))return '';
  return s;
 }
 // Read visible labels, not page globals or network responses. A heading bounds
 // each group; quantity badges are never part of the color name.
 function pageOptions(doc){
  const all=[...doc.querySelectorAll('*')].filter(visible),headings=new Set(['颜色','颜色分类','尺码','尺寸']);
  const colors=new Map(),sizes=new Map();
  for(const heading of all){
   if(heading.children.length||!['颜色','颜色分类'].includes(String(heading.textContent).trim()))continue;
   let container=heading.parentElement;
   for(let depth=0;container&&depth<4;depth++,container=container.parentElement){
    const nodes=[...container.querySelectorAll('*')];let started=false;const found=[];
    for(const el of nodes){if(el===heading){started=true;continue}if(!started||el.children.length||!visible(el))continue;
     const text=String(el.textContent).trim();if(headings.has(text))break;
     const label=optionText(text);if(label)found.push({label,el});
    }
    if(!found.length)continue;
    for(const {label,el} of found){let image='',parent=el;for(let i=0;parent&&parent!==container&&i<4;i++,parent=parent.parentElement){
      const peers=found.filter(x=>parent===x.el||parent.contains?.(x.el));if(peers.length>1)break;
      const img=parent.querySelector('img');const src=img?.currentSrc||img?.getAttribute('src')||'';if(/^https?:\/\//i.test(src)){image=src;break}
     }
     colors.set(normalize(label),{label,image});
    }
    break;
   }
  }
  const isSize=s=>/^(?:\d{0,2}X{0,5}[SML]|均码|大码|小码|加大码|\d{2,3}(?:码|cm)?)$/i.test(s);
  for(const input of doc.querySelectorAll(inputSelector)){
   if(!visible(input))continue;let parent=input.parentElement;
   for(let depth=0;parent&&depth<6;depth++,parent=parent.parentElement){
    if([...parent.querySelectorAll(inputSelector)].filter(visible).length>1)break;
    const label=[...parent.querySelectorAll('*')].filter(x=>!x.children.length&&!x.matches('input,script,style')&&visible(x)).map(x=>String(x.textContent).trim()).find(isSize);
    if(label){sizes.set(normalize(label),{label});break}
   }
  }
  return {colors:[...colors.values()],sizes:[...sizes.values()]};
 }
 globalThis.OPProcurementDOM={scan,path,value,inspect,activeLabels,visible,pageOptions,quantityControl,operatedSpec};
})();
