import {readArchive} from '../internal/adapters/miaoshou/web/worker/archive.mjs';
if(!process.argv[2]){console.error('用法: node scripts/replay.mjs data/raw/<账户>/<记录>.json');process.exit(1)}
try{const {json}=await readArchive(process.argv[2]);process.stdout.write(JSON.stringify(json,null,2)+'\n')}catch(e){console.error(e.message);process.exit(1)}
