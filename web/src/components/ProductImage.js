import {computed,ref,watch} from 'vue';
export default {
 props:['item'],
 setup(props){const index=ref(0);const candidates=computed(()=>[...new Set([props.item.picUrlThumb,props.item.picUrl,props.item.originalPicUrl].filter(v=>/^https?:\/\//i.test(v||'')))]);watch(()=>props.item.opOrderItemId,()=>{index.value=0});const src=computed(()=>candidates.value[index.value]||'');const full=computed(()=>/^https?:\/\//i.test(props.item.originalPicUrl||'')?props.item.originalPicUrl:src.value);return{src,full,index}},
 template:`<div class="product-visual"><a v-if="src" :href="full" target="_blank" rel="noopener noreferrer" title="查看商品原图"><img :src="src" :alt="item.title||'商品图片'" loading="lazy" referrerpolicy="no-referrer" @error="index++"></a><span v-else class="image-placeholder">暂无可用图片</span><span v-if="String(item.isTiktokGift)==='1'||String(item.isGift)==='1'" class="gift-label">礼品</span></div>`
};
