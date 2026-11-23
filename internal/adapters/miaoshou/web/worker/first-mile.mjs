import {observeOutbound} from './outbound.mjs';
import {purchaseLookupEndpoint} from './purchase-lookup.mjs';
import {access, mkdir, readFile, readdir, rename, writeFile} from 'node:fs/promises';
import path from 'node:path';

const origin = 'https://erp.91miaoshou.com';
const listEndpoint = '/api/order/package/render_list/searchOrderPackageList';
const updateEndpoint = '/api/order/purchase_order/update_purchase_order';
const purchasedRadio = 'input[type=radio][value=hasPurchase]';
const waitingForReceipt = new Set(['wait_pay', 'wait_send', 'part_send', 'has_send']);
const digits = value => /^\d+$/.test(String(value ?? '')) && (typeof value === 'string' || Number.isSafeInteger(value));
const purchaseBox = '.J_purchaseOrderInfoSingleBox[data-purchase-order-id]';
const refreshButton = 'a.J_manualCheckPurchaseOrderStatus';

export function firstMilePurchases(outbound, skippedSigned) {
  const purchases = new Map();
  const signed = new Set();
  for (const pkg of Object.values(outbound)) {
    for (const purchase of Object.values(pkg?.purchaseOrders || {})) {
      if (!digits(purchase.purchaseOrderId) || !digits(purchase.purchaseOrderSn) ||
          ![purchase.purchasePlatform, purchase.purchasePlatformName].some(p => p === 'ali1688' || p === '1688') ||
          [true, 1, '1', 'true'].includes(purchase.isUnrelated)) continue;
      const id = String(purchase.purchaseOrderId);
      const number = String(purchase.purchaseOrderSn);
      if (purchase.purchaseOrderStatus === 'has_sign') signed.add(number);
      if (waitingForReceipt.has(purchase.purchaseOrderStatus)) {
        purchases.set(id, {id, number, beforeStatus:purchase.purchaseOrderStatus});
      }
    }
  }
  for (const [id, purchase] of purchases) if (signed.has(purchase.number)) purchases.delete(id);
  if (skippedSigned) for (const number of signed) skippedSigned.add(number);
  return purchases;
}

async function capture(page, number, action) {
  const outbound = observeOutbound(page);
  try {
    const [response] = await Promise.all([
      page.waitForResponse(r => {
        const url = new URL(r.url());
        if (url.origin !== origin || url.pathname !== listEndpoint) return false;
        const params = new URLSearchParams(r.request().postData());
        return params.get('appPackageTab') === 'waitProcess' && params.get('purchaseTab') === 'hasPurchase' &&
          Number(params.get('page')) === number;
      }, {timeout:45000}),
      action()
    ]);
    const list = await response.json();
    if (!response.ok() || list.result !== 'success' || !Array.isArray(list.packageList) ||
        !Number.isSafeInteger(Number(list.total)) || Number(list.total) < 0 ||
        !Number.isSafeInteger(Number(list.pageSize)) || Number(list.pageSize) < 1 ||
        list.packageList.some(pkg => !digits(pkg.opOrderPackageId))) {
      throw new Error('妙手未返回有效的订单列表');
    }
    return {pages:Math.ceil(Number(list.total) / Number(list.pageSize)), pageSize:Number(list.pageSize),
      outbound:await outbound.wait(list.packageList.map(pkg => pkg.opOrderPackageId))};
  } finally {
    outbound.stop();
  }
}

async function widenPages(page, current) {
  const size = page.locator('#appScrollContainer').getByText(`${current.pageSize}条/页`, {exact:true});
  if (!await size.count()) return current;
  await size.click({timeout:15000});
  const listbox = page.getByRole('listbox').filter({visible:true});
  const options = listbox.getByRole('option');
  await options.first().waitFor({timeout:10000});
  const supported = (await options.allTextContents()).map(text => /^\s*(\d+)\s*条\/页\s*$/.exec(text)?.[1]).filter(Boolean).map(Number);
  const largest = Math.max(current.pageSize,...supported);
  if (largest === current.pageSize) {
    await page.keyboard.press('Escape');
    return current;
  }
  const widened = await capture(page,1,() => listbox.getByRole('option',{name:`${largest}条/页`,exact:true})
    .click({timeout:15000}));
  if (widened.pageSize !== largest) throw new Error('妙手分页大小未生效');
  return widened;
}

async function refreshPurchase(page, box, purchase) {
  const context = page.context();
  const prior = context.waitForEvent('response', {timeout:20000,predicate:r => {
    const req = r.request();
    const url = new URL(r.url());
    return url.origin === origin && url.pathname === purchaseLookupEndpoint &&
      req.method() === 'GET' && url.searchParams.get('purchaseOrderSn') === purchase.number;
  }});
  const hasPurchaseNumber = new RegExp(`(?:^|\\D)${purchase.number}(?:\\D|$)`);
  const saved = context.waitForEvent('response', {timeout:30000,predicate:r => {
    const url = new URL(r.url());
    return url.origin === origin && url.pathname === updateEndpoint && r.request().method() === 'POST' &&
      hasPurchaseNumber.test(r.request().postData() || '');
  }});
  const [lookup, update] = await Promise.all([prior, saved, box.locator(refreshButton).first().click({timeout:5000})]);
  const result = await update.json();
  if (!lookup.ok() || !update.ok() || result.result !== 'success') throw new Error('妙手采购状态刷新失败');
}

export async function syncFirstMile(session, report = {
  attempted:0, updated:0, failed:0, skippedSigned:0, items:[]
}, shouldStop = async () => false) {
  const allowed = () => session.context && session.state === 'verified' && !session.packing &&
    session.sync?.state !== 'running';
  const keepGoing = async () => allowed() && !await shouldStop();
  if (!await keepGoing()) return report;
  const page = await session.context.newPage();
  const attempted = new Set();
  const unavailable = new Map();
  const signed = new Set();
  try {
    await page.setViewportSize({width:1728,height:1000});
    let number = 1;
    let current = await capture(page, number, async () => {
      await page.goto(origin + '/order/package/index?appPackageTab=waitProcess&purchaseTab=hasPurchase', {waitUntil:'domcontentloaded'});
      const radio = page.locator(purchasedRadio);
      if (!await radio.waitFor({state:'attached',timeout:10000}).then(() => true, () => false) &&
          new URL(page.url()).pathname === '/order/package/index') {
        await page.reload({waitUntil:'domcontentloaded'});
      }
      if (await radio.isChecked({timeout:15000})) return;
      await page.locator('label').filter({has:radio}).click({timeout:15000});
    });
    if (current.pages > 1 && await keepGoing()) current = await widenPages(page,current);
    for (;;) {
      if (!await keepGoing()) break;
      const candidates = firstMilePurchases(current.outbound, signed);
      report.skippedSigned = signed.size;
      for (const [id, purchase] of candidates) {
        if (attempted.has(purchase.number) || signed.has(purchase.number)) candidates.delete(id);
      }
      const before = report.attempted;
      const updated = new Map();
      const scroll = page.locator('#appScrollContainer');
      for (let top = 0; candidates.size && await keepGoing();) {
        const bottom = await scroll.evaluate((el, top) => {
          el.scrollTop = top;
          return el.scrollTop + el.clientHeight >= el.scrollHeight - 1;
        }, top);
        await page.waitForTimeout(120);
        const visibleIds = await page.locator(purchaseBox).evaluateAll(boxes =>
          boxes.map(box => box.dataset.purchaseOrderId));
        for (const id of new Set(visibleIds)) {
          const purchase = candidates.get(id);
          if (!purchase || attempted.has(purchase.number) || !allowed()) continue;
          const box = page.locator(`${purchaseBox}[data-purchase-order-id="${id}"]`)
            .filter({hasText:purchase.number, has:page.locator(refreshButton)}).first();
          if (!await box.count()) continue;
          if (!await keepGoing()) break;
          for (const [aliasID, alias] of candidates) {
            if (alias.number === purchase.number) candidates.delete(aliasID);
          }
          attempted.add(purchase.number);
          unavailable.delete(purchase.number);
          report.attempted++;
          const item = {...purchase, state:'failed'};
          report.items.push(item);
          session.firstMileWriting = true;
          try {
            await refreshPurchase(page, box, purchase);
            report.updated++;
            item.state = 'updated';
            updated.set(id, item);
          } catch (error) {
            report.failed++;
            item.error = /Timeout/i.test(error.message) ? '等待妙手刷新响应超时' : '妙手查询或回写失败';
          } finally {
            session.firstMileWriting = false;
          }
        }
        if (bottom) break;
        top += 450;
      }
      if (await keepGoing()) {
        for (const purchase of candidates.values()) {
          unavailable.set(purchase.number,purchase);
        }
      }
      if (report.attempted > before && allowed()) {
        const refreshed = await capture(page, number, () => page.locator('#J_refreshOrderList').evaluate(link => link.click()));
        await Promise.all([...session.pending || []]);
        for (const pkg of Object.values(refreshed.outbound)) for (const p of Object.values(pkg?.purchaseOrders || {})) {
          const item = updated.get(String(p.purchaseOrderId));
          if (item && typeof p.purchaseOrderStatus === 'string') item.afterStatus = p.purchaseOrderStatus;
        }
      }
      if (!await keepGoing()) break;
      if (number >= current.pages) break;
      number++;
      current = await capture(page, number, () => page.getByRole('listitem', {name:`第 ${number} 页`,exact:true})
        .press('Enter', {timeout:15000}));
    }
    return report;
  } finally {
    for (const purchase of unavailable.values()) {
      if (signed.has(purchase.number)) continue;
      report.failed++;
      report.items.push({...purchase, state:'skipped', error:'未找到该采购单的刷新按钮'});
    }
    await page.close().catch(() => {});
  }
}

export async function readFirstMileSettings(dir) {
  try {
    const settings = JSON.parse(await readFile(path.join(dir,'first-mile-settings.json'), 'utf8'));
    if (typeof settings.enabled !== 'boolean' || !Number.isInteger(settings.intervalMinutes) ||
        settings.intervalMinutes < 1 || settings.intervalMinutes > 1440) throw new Error('头程同步设置无效');
    return settings;
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
    return {enabled:true, intervalMinutes:60};
  }
}

export async function firstMileStopRequested(dir, id) {
  try {
    await access(path.join(dir,id+'.stop'));
    return true;
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
    return false;
  }
}

export function startFirstMileSync(session, dir, synchronize = syncFirstMile) {
  const runs = path.join(dir,'first-mile-runs');
  let stopped = false;
  let timer;
  let busy = false;
  let lastRun;
  let unsavedRun;

  async function save(run) {
    const file = path.join(runs,run.id+'.json');
    await writeFile(file+'.tmp', JSON.stringify(run), {mode:0o600});
    await rename(file+'.tmp',file);
  }

  async function finish(run) {
    let stopRequested = false;
    const shouldStop = async () => {
      if (!stopRequested) stopRequested = await firstMileStopRequested(runs,run.id);
      return stopRequested;
    };
    try {
      await synchronize(session,run,shouldStop);
      run.state = await shouldStop() ? 'stopped'
        : stopped || session.state !== 'verified' || session.packing || session.sync?.state === 'running'
          ? 'interrupted' : run.failed ? 'partial' : 'completed';
    } catch (error) {
      run.state = stopped || !session.context ? 'interrupted' : 'failed';
      run.error = run.state === 'interrupted' ? '浏览器已关闭' : /Timeout/i.test(error.message)
        ? '等待妙手订单列表或出货信息超时' : '读取妙手订单或保存出货信息失败';
      console.warn('[头程物流]', String(error.message).split('\n')[0]);
    }
    run.finishedAt = new Date().toISOString();
    lastRun = run;
    unsavedRun = run;
    await save(run);
    unsavedRun = null;
  }

  async function begin(manual) {
    if (busy) return {error:'已有一轮头程物流同步正在运行'};
    if (stopped || !session.context || session.state !== 'verified') return {error:'请先登录妙手并打开订单处理页'};
    if (session.packing || session.sync?.state === 'running') return {error:'账户正在执行订单同步或打包，请稍后重试'};
    busy = true;
    let started = false;
    try {
      if (unsavedRun) {
        await save(unsavedRun);
        unsavedRun = null;
      }
      if (lastRun === undefined) {
        await mkdir(runs, {recursive:true,mode:0o700});
        const latest = (await readdir(runs)).filter(name => /^\d{13}\.json$/.test(name)).sort().at(-1);
        lastRun = latest ? JSON.parse(await readFile(path.join(runs,latest),'utf8')) : null;
        if (lastRun?.state === 'running') {
          lastRun.state = 'interrupted';
          lastRun.finishedAt = new Date().toISOString();
          lastRun.error = '浏览器进程已重启';
          unsavedRun = lastRun;
          await save(unsavedRun);
          unsavedRun = null;
        }
      }
      const settings = await readFirstMileSettings(dir);
      if (stopped || !session.context || session.state !== 'verified' || session.packing || session.sync?.state === 'running') {
        return {error:'账户浏览器状态已变化，请稍后重试'};
      }
      if (!manual && (!settings.enabled || lastRun && Date.now() - Date.parse(lastRun.finishedAt) < settings.intervalMinutes * 60000)) return {};
      const now = Date.now();
      const run = {id:String(now), startedAt:new Date(now).toISOString(), intervalMinutes:settings.intervalMinutes,
        trigger:manual?'manual':'automatic',state:'running',canStop:true,attempted:0,updated:0,failed:0,skippedSigned:0,items:[]};
      await save(run);
      started = true;
      void finish(run).catch(logError).finally(() => {busy = false;});
      return {id:run.id,state:'running'};
    } finally {
      if (!started) busy = false;
    }
  }
  function logError(error) {
    console.warn('[头程物流] 设置或记录读写失败', error.code || error.message);
  }
  async function tick() {
    try {await begin(false);}
    catch (error) {logError(error);}
    finally {if (!stopped) timer = setTimeout(tick, 10000);}
  }
  timer = setTimeout(tick, 10000);
  return {
    runNow:() => begin(true),
    stop() {
      stopped = true;
      clearTimeout(timer);
    }
  };
}
