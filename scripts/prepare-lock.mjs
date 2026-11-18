import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

export function checkLock(manifest,lock,dockerfile){
 const problems=[];
 if(!lock?.packages||lock.lockfileVersion<2)return ['缺少完整的依赖锁'];
 for(const [name,version] of Object.entries({...manifest.dependencies,...manifest.devDependencies})){
  if(!/^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(version))problems.push(`${name} 必须使用固定版本`);
  if(lock.packages['node_modules/'+name]?.version!==version)problems.push(`${name} 与 package.json 版本不一致`);
 }
 const playwright=manifest.dependencies?.playwright;
 if(lock.packages['node_modules/playwright-core']?.version!==playwright)problems.push('Playwright core 版本不匹配');
 if(!dockerfile.includes(`FROM mcr.microsoft.com/playwright:v${playwright}-`))problems.push('Dockerfile 的 Playwright 镜像版本不匹配');
 for(const [name,pkg] of Object.entries(lock.packages)){
  if(!name)continue;
  if(!pkg.resolved||!pkg.integrity)problems.push(`${name} 缺少下载校验信息`);
  for(const dependency of Object.keys(pkg.optionalDependencies||{})){
   if(!lock.packages[name+'/node_modules/'+dependency]&&!lock.packages['node_modules/'+dependency])problems.push(`缺少跨平台可选依赖 ${dependency}`);
  }
 }
 return problems;
}

async function main(){
 const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
 const manifest=JSON.parse(await fs.readFile(path.join(root,'package.json'),'utf8'));
 const dockerfile=await fs.readFile(path.join(root,'deploy/browser/Dockerfile'),'utf8');
 let lock;try{lock=JSON.parse(await fs.readFile(path.join(root,'package-lock.json'),'utf8'))}catch{}
 const problems=checkLock(manifest,lock,dockerfile);
 if(!problems.length){console.log('依赖锁和 Playwright 镜像版本已校验');return}
 // Generate in a clean directory: npm otherwise inherits only this Mac's
 // installed optional native packages, omitting Linux entries from the lock.
 console.log('正在恢复完整依赖锁（固定直接依赖版本，包含跨平台可选依赖）…');
 const temp=await fs.mkdtemp(path.join(os.tmpdir(),'order-processor-lock-'));
 try{
  const locked={...manifest};
  // Preserve recovered transitive versions too. Missing optional packages are
  // resolved from those pinned parents' manifests rather than guessed here.
  const pins={};for(const [name,pkg] of Object.entries(lock?.packages||{}))if(name.startsWith('node_modules/')&&!name.slice(13).includes('/node_modules/'))pins[name.slice(13)]=pkg.version;
  locked.overrides={...pins,...manifest.overrides};
  await fs.writeFile(path.join(temp,'package.json'),JSON.stringify(locked,null,2)+'\n');
  const result=spawnSync(process.platform==='win32'?'npm.cmd':'npm',['install','--package-lock-only','--ignore-scripts','--include=optional','--no-audit','--no-fund','--fetch-retries=0','--fetch-timeout=20000'],{cwd:temp,stdio:'inherit'});
  if(result.error||result.status!==0)throw Error('无法生成完整依赖锁，请检查当前终端到 npm registry 的网络后重新运行 npm run start:remote；尚未开始构建 Docker 镜像。');
  const text=await fs.readFile(path.join(temp,'package-lock.json'),'utf8');
  const next=JSON.parse(text),remaining=checkLock(manifest,next,dockerfile);
  if(remaining.length)throw Error('依赖锁校验失败：'+remaining.slice(0,3).join('；'));
  await fs.writeFile(path.join(root,'package-lock.json'),text);
  console.log('完整依赖锁已保存');
 }finally{await fs.rm(temp,{recursive:true,force:true})}
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url))main().catch(e=>{console.error(e.message);process.exitCode=1});
