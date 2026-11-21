export function offerId(url){try{const u=new URL(url);return /^https?:$/.test(u.protocol)&&u.hostname==='detail.1688.com'?u.pathname.match(/^\/offer\/(\d+)\.html$/)?.[1]||'':''}catch{return ''}}
// Link identity only: seller assignments never affect the extension's demand.
export function procurementHandoff(groups,group,account,createdAt){
 const id=offerId(group.url);if(!id)return null;
 const variants=new Map(),images=new Map();
 for(const g of groups){if(g.reason||offerId(g.url)!==id)continue;for(const v of g.variants){const n=Number(v.quantity);if(!Number.isSafeInteger(n)||n<=0)continue;variants.set(v.spec,(variants.get(v.spec)||0)+n);images.set(v.spec,[...new Set([...(images.get(v.spec)||[]),...(v.images||[])])].filter(u=>/^https?:\/\//i.test(u)).slice(0,6))}}
 if(!variants.size)return null;
 return {version:1,offerId:id,url:`https://detail.1688.com/offer/${id}.html`,account:String(account),title:group.title||'待采购商品',createdAt,sourceStatus:{state:['valid','invalid'].includes(group.sourceStatus?.state)?group.sourceStatus.state:'unknown',reason:typeof group.sourceStatus?.reason==='string'?group.sourceStatus.reason.slice(0,500):'',checkedAt:typeof group.sourceStatus?.checkedAt==='string'?group.sourceStatus.checkedAt.slice(0,60):'',source:['manual','plugin'].includes(group.sourceStatus?.source)?group.sourceStatus.source:''},variants:[...variants].map(([spec,quantity])=>({spec,quantity,...(images.get(spec)?.length?{images:images.get(spec)}:{})}))};
}
