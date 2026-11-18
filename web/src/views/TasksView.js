import PackingJobs from '../components/PackingJobs.js';
export default {
 components:{PackingJobs},props:['account','api'],
 template:`<section class="tasks-view"><PackingJobs v-if="account" :account="account" :api="api" standalone/><div v-else class="empty">请先选择账户，再查看任务。</div></section>`
};
