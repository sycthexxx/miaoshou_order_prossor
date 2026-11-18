import {test} from 'node:test';
import assert from 'node:assert/strict';
import '../../extensions/procurement-helper/core.js';
import '../../extensions/procurement-helper/adapter.js';
// Minimal DOM fixture for deterministic matching tests. It does not emulate a
// real 1688 layout or claim that browser selectors have been live validated.
class Element {
 constructor(tag,text='',attrs={},children=[]){this.tagName=tag.toUpperCase();this.text=text;this.attrs=attrs;this.className=attrs.class||'';this.children=children;this.isConnected=true;this.value=attrs.value||'';for(const child of children)child.parentElement=this}
 get textContent(){return this.text+this.children.map(x=>x.textContent).join('')}
 getAttribute(name){return this.attrs[name]??null}
 getClientRects(){return this.attrs.hidden?[]:[{}]}
 matches(selector){return selector.split(',').some(x=>x==='input'?this.tagName==='INPUT':['script','style'].includes(x)?this.tagName===x.toUpperCase():false)}
 querySelectorAll(selector){const all=[];const walk=el=>{for(const child of el.children){all.push(child);walk(child)}};walk(this);if(selector==='*')return all;if(selector.includes('input:not'))return all.filter(x=>x.tagName==='INPUT');return all.filter(x=>x.attrs['aria-selected']==='true'||/(selected|active|checked)/.test(x.className))}
 closest(selector){let el=this;while(el){if(selector.split(',').some(s=>s==='input'?el.tagName==='INPUT':s==='button'?el.tagName==='BUTTON':s==='[role=spinbutton]'?el.attrs.role==='spinbutton':s==='[role=button]'?el.attrs.role==='button':false))return el;el=el.parentElement}return null}
 contains(el){return el===this||this.querySelectorAll('*').includes(el)}
 querySelector(selector){return this.querySelectorAll('*').find(x=>selector.startsWith('#')?x.attrs.id===selector.slice(1):x.tagName===selector.toUpperCase())||null}
}
globalThis.getComputedStyle=()=>({visibility:'visible'});
const makeRow=(size,n,id)=>new Element('div','',{},[new Element('span',size),new Element('span','库存897套'),new Element('input','',{value:String(n),id})]);
function fixture(){const black=new Element('button','黑色',{class:'sku-item selected'}),red=new Element('button','酒红色',{class:'sku-item'}),row=makeRow('M',1,'m'),doc=new Element('div','',{},[black,red,row]);return {black,red,row,doc,input:row.children[2]}}
const {scan}=OPProcurementDOM;
test('selected color plus exact size; changed quantity and color recompute',()=>{
 const f=fixture(),variants=[{spec:'黑色,M'},{spec:'酒红色,M'},{spec:'黑色,S'}];
 assert.deepEqual([...scan(f.doc,variants)],[['黑色,M',1],['酒红色,M',null],['黑色,S',null]]);
 f.input.value='3';assert.equal(scan(f.doc,variants).get('黑色,M'),3);
 f.black.className='sku-item';f.red.className='sku-item selected';
 assert.deepEqual([...scan(f.doc,variants)],[['黑色,M',null],['酒红色,M',3],['黑色,S',null]]);
 f.input.value='';assert.equal(scan(f.doc,variants).get('酒红色,M'),null);
});
test('ambiguous inputs never green; manual binding resolves exact row only',()=>{
 const f=fixture();const other=makeRow('M',9,'duplicate');other.parentElement=f.doc;f.doc.children.push(other);const variants=[{spec:'黑色,M'}];
 assert.equal(scan(f.doc,variants).get('黑色,M'),null);
 assert.equal(scan(f.doc,variants,{'黑色,M':{selector:'#m',alias:'黑色,M'}}).get('黑色,M'),1);
 f.black.className='';assert.equal(scan(f.doc,variants,{'黑色,M':{selector:'#m',alias:'黑色,M'}}).get('黑色,M'),null);
});
test('aliases need exact match and cannot double-count one input',()=>{
 const f=fixture();const variants=[{spec:'Black,M'},{spec:'黑色,M'}];
 const result=scan(f.doc,variants,{'Black,M':{alias:'黑色,M'}});assert.equal(result.get('Black,M'),null);assert.equal(result.get('黑色,M'),null);
 assert.equal(scan(f.doc,[{spec:'Black,M'}],{'Black,M':{alias:'黑色,M'}}).get('Black,M'),1);
 f.input.attrs.hidden=true;assert.equal(scan(f.doc,[{spec:'黑色,M'}]).get('黑色,M'),null);
});

test('reads page color labels without count badges and sizes without stock/prices',()=>{
 const color=(name,count,src,hidden=false)=>new Element('button','',{hidden},[new Element('span',count),new Element('img','',{src}),new Element('span',name)]);
 const colors=new Element('section','',{},[new Element('div','颜色'),new Element('div','',{},[
 color('酒红色','2','https://images.example/red.jpg'),color('藏青色','3','https://images.example/navy.jpg'),color('隐藏色','9','https://images.example/hidden.jpg',true)
 ])]);
 // Hidden descendants in a real browser have no client rects.
 colors.children[1].children[2].children.forEach(x=>{x.attrs.hidden=true});
 const sizes=new Element('section','',{},[new Element('div','尺码'),makeRow('1XL',2,'xl'),makeRow('5XL',0,'5xl')]);
 const doc=new Element('main','',{},[colors,sizes,new Element('p','立即下单')]);
 const options=OPProcurementDOM.pageOptions(doc);
 assert.deepEqual(options.colors,[{label:'酒红色',image:'https://images.example/red.jpg'},{label:'藏青色',image:'https://images.example/navy.jpg'}]);
 assert.deepEqual(options.sizes,[{label:'1XL'},{label:'5XL'}]);
});
test('missing page options does not fabricate a mapping',()=>{
 const doc=new Element('main','',{},[new Element('span','海军蓝'),new Element('span','¥52'),new Element('span','库存999')]);
 assert.deepEqual(OPProcurementDOM.pageOptions(doc),{colors:[],sizes:[]});
});

test('quantity input and plus/minus follow only the uniquely matched SKU',()=>{
 const f=fixture(),plus=new Element('button','+'),minus=new Element('button','−');
 plus.parentElement=f.row;minus.parentElement=f.row;f.row.children.push(plus,minus);
 const variants=[{spec:'黑色,M'},{spec:'酒红色,M'}];
 const {operatedSpec}=OPProcurementDOM;
 for(const control of [f.input,plus,minus])assert.equal(operatedSpec(f.doc,control,variants),'黑色,M');
 assert.equal(operatedSpec(f.doc,f.red,variants),null);
 f.black.className='';f.red.className='selected';assert.equal(operatedSpec(f.doc,plus,variants),'酒红色,M');
 assert.equal(operatedSpec(f.doc,plus,[{spec:'Red,M'}],{'Red,M':{alias:'酒红色,M'}}),'Red,M');
 const duplicate=makeRow('M',1,'duplicate');duplicate.parentElement=f.doc;f.doc.children.push(duplicate);
 assert.equal(operatedSpec(f.doc,plus,variants),null);
});

test('icon-only quantity button follows its input without plus/minus text',()=>{
 const f=fixture(),icon=new Element('svg'),button=new Element('button','',{},[icon]);
 button.parentElement=f.row;f.row.children.push(button);
 assert.equal(OPProcurementDOM.operatedSpec(f.doc,icon,[{spec:'黑色,M'}]),'黑色,M');
});

test('operation identity does not depend on quantity validity, change or demand equality',()=>{
 const f=fixture(),plus=new Element('button','+');plus.parentElement=f.row;f.row.children.push(plus);
 const variants=[{spec:'黑色,M',quantity:5}];
 for(const n of ['0','1','5','9','','-','1.']){
  f.input.value=n;
  assert.equal(OPProcurementDOM.operatedSpec(f.doc,plus,variants),'黑色,M');
  assert.equal(OPProcurementDOM.operatedSpec(f.doc,f.input,variants),'黑色,M');
 }
 assert.equal(OPProcurementDOM.scan(f.doc,variants).get('黑色,M'),null,'incomplete numeric input still cannot turn green');
});

test('quantity checks and operation follow use the same shared-color effective aliases',async()=>{
 await import('../../extensions/procurement-helper/mappings.js');
 const f=fixture(),state=OPMappings.empty(),variants=[{spec:'深黑,M',quantity:1}];
 OPMappings.apply(state,'深黑,L','黑色,L');
 const bindings=OPMappings.effective(state,variants);
 assert.equal(OPProcurementDOM.scan(f.doc,variants,bindings).get('深黑,M'),1);
 assert.equal(OPProcurementDOM.operatedSpec(f.doc,f.input,variants,bindings),'深黑,M');
 const ledger=new Map();const checked=OPProcurement.reconcile(variants,new Map([['深黑,M',1]]),ledger,new Set(['黑色']),bindings);
 assert.equal(checked.get('深黑,M').quantity,1);
 OPMappings.apply(state,'深黑,L','酒红色,L');
 const changed=OPMappings.effective(state,variants),readings=OPProcurementDOM.scan(f.doc,variants,changed);
 assert.equal(readings.get('深黑,M'),null);assert.equal(OPProcurementDOM.operatedSpec(f.doc,f.input,variants,changed),null);
 assert.equal(OPProcurement.reconcile(variants,readings,ledger,new Set(['黑色']),changed).get('深黑,M').quantity,null);
});
