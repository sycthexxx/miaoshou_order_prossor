// Clipboard APIs may be unavailable on the LAN HTTP workbench.
export async function copyText(text){
 try{if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(text);return}}catch{}
 const active=document.activeElement,selection=window.getSelection();
 const ranges=selection?Array.from({length:selection.rangeCount},(_,i)=>selection.getRangeAt(i).cloneRange()):[];
 const inputSelection=active&&typeof active.selectionStart==='number'?{start:active.selectionStart,end:active.selectionEnd,direction:active.selectionDirection}:null;
 const field=document.createElement('textarea');field.value=text;field.setAttribute('readonly','');field.style.cssText='position:fixed;left:-9999px;top:0;opacity:0';
 try{document.body.appendChild(field);field.focus({preventScroll:true});field.select();if(!document.execCommand('copy'))throw new Error('复制失败，请手动选择标题复制')}
 finally{field.remove();active?.focus?.({preventScroll:true});if(inputSelection)active.setSelectionRange(inputSelection.start,inputSelection.end,inputSelection.direction);if(selection){selection.removeAllRanges();for(const range of ranges)selection.addRange(range)}}
}
