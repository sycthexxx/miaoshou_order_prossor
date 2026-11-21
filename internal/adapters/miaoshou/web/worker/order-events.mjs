import path from 'node:path';
import {mkdir, readFile, writeFile, rename} from 'node:fs/promises';
import {randomUUID} from 'node:crypto';
import {nativeClient} from './native-request.mjs';

const origin = 'https://erp.91miaoshou.com';
const listPath = '/api/order/package/render_list/searchOrderPackageList';
const checkPath = '/api/order/purchase/checkRsyncOrderTipMsg';
const wsInfoPath = '/api/auth/shop/getWsInfo';
const identityPath = '/api/auth/account/getAccountInfo';
const text = value => typeof value === 'string' || Number.isSafeInteger(value) ? String(value).trim() : '';
const eventKey = tip => JSON.stringify([tip.messageId, tip.orderNumber, tip.kind]);

// Keep platform order numbers intact even if an upstream response uses a JSON number.
function parse(text) {
  return JSON.parse(text, (_, value, ctx) => Number.isInteger(value) && !Number.isSafeInteger(value) ? ctx?.source ?? null : value);
}
export function orderTips(list) {
  if (!Array.isArray(list)) return [];
  const tips = [];
  for (const row of list) {
    const kind = row?.source === 'new_order' ? 'new_order'
      : row?.source === 'after_sales' && ['in_cancel','cancelled'].includes(row.status) ? row.status : '';
    const orderNumber = text(row?.platformOrderSn);
    const messageId = text(row?.orderTipMsgId);
    if (!kind || !orderNumber || !messageId || orderNumber.length > 150 || messageId.length > 150) continue;
    tips.push({kind, orderNumber, messageId});
  }
  return tips;
}

export function socketTips(payload, identity) {
  if (!identity || payload?.action !== 'ORDER_TIP_MSG') return [];
  const content = payload.content;
  if (!identity.main && (!text(content?.subAccountId) || text(content.subAccountId) !== identity.sub)) return [];
  return orderTips(content?.orderTipMsgList);
}

function matchesOrder(pkg, orderNumber) {
  const matches = v => [v?.platformOrderSn, v?.orderInfo?.platformOrderSn].some(sn => text(sn) === orderNumber);
  if (matches(pkg)) return true;
  for (const key of ['items', 'orderList', 'mergePackageList', 'mergedPackageList', 'subPackageList']) {
    if (Object.values(pkg?.[key] || {}).some(matches)) return true;
  }
  return false;
}

export function eventSearch(request) {
  if (request.method() !== 'POST' || new URL(request.url()).pathname !== listPath) return false;
  const p = new URLSearchParams(request.postData());
  return p.get('source') === 'orderHistory' && p.get('appPackageTab') === 'all' &&
    p.get('platformOrderSnsRp') === 'eq' && !!p.get('platformOrderSns');
}

// Use the same order-history search that the ERP uses for its exact order-number field.
// Take the raw response instead of numbers after the page's own JSON deserialisation.
export async function lookupOrderPackages(page, orderNumber, resolveClient = nativeClient) {
  const client = await resolveClient(page);
  const byId = new Map();
  const pageSize = 50;
  let expected;
  for (let n = 1; n <= 100; n++) {
    const data = {source:'orderHistory', appPackageTab:'all', platformOrderSnsRp:'eq', platformOrderSns:orderNumber, page:n, pageSize};
    const [response] = await Promise.all([
      page.waitForResponse(response => {
        if (response.url() !== origin + listPath || !eventSearch(response.request())) return false;
        const q = new URLSearchParams(response.request().postData());
        return q.get('platformOrderSns') === orderNumber && Number(q.get('page')) === n;
      }, {timeout:35000}),
      page.evaluate(async ({client, data, url}) => {
        const mod = await import(client.url);
        await mod[client.name]({url, method:'POST', data, timeout:30000, __tips:false});
      }, {client, data, url:listPath.slice(4)})
    ]);
    const result = parse(await response.text());
    const total = Number(result.total);
    if (!response.ok() || result.result !== 'success' || !Array.isArray(result.packageList) ||
        !Number.isSafeInteger(total) || total < 1 || Number(result.page) !== n ||
        (expected !== undefined && expected !== total) || !result.packageList.length) {
      throw new Error('妙手未返回完整的订单号查询结果');
    }
    expected = total;
    for (const pkg of result.packageList) {
      const id = text(pkg?.opOrderPackageId);
      if (!id || byId.has(id) || !matchesOrder(pkg, orderNumber)) throw new Error('订单号查询结果不匹配');
      byId.set(id, pkg);
    }
    if (byId.size === total) return [...byId.values()];
    if (byId.size > total) break;
  }
  throw new Error('订单号查询分页不完整');
}

async function atomicJSON(file, obj) {
  const tmp = file + '.' + randomUUID() + '.tmp';
  await writeFile(tmp, JSON.stringify(obj), {mode:0o600});
  await rename(tmp, file);
}

export async function attachOrderEvents(root, id, session) {
  const dir = path.join(root, 'runtime', id);
  const inbox = path.join(dir, 'order-events');
  const stateFile = path.join(dir, 'order-event-queue.json');
  await mkdir(inbox, {recursive:true, mode:0o700});
  let prior;
  try { prior = JSON.parse(await readFile(stateFile,'utf8')); }
  catch (e) { if (e.code !== 'ENOENT') throw e; }
  const pending = new Map((prior?.pending || []).map(tip => [eventKey(tip),tip]));
  const done = new Set(prior?.done || []);
  const context = session.context;
  const activeQueries = new Map();
  const metadata = new WeakMap();
  const clients = new WeakMap();
  const batches = Array.isArray(prior?.batches) ? prior.batches.slice(0, 30) : [];
  let lastSocketAt = batches.find(batch => batch.source === 'websocket')?.receivedAt ?? null;
  let lastCheckAt = batches.find(batch => batch.source === 'http')?.receivedAt ?? null;
  let lastQueuedAt = null;
  let closed = false, running, timer, saving = Promise.resolve();
  function save() {
    const state = {pending:[...pending.values()], done:[...done], batches:[...batches]};
    saving = saving.catch(() => {}).then(() => atomicJSON(stateFile,state));
    return saving;
  }
  function receive(source, tips) {
    if (closed) return;
    const receivedAt = new Date().toISOString();
    const byKind = {new_order:new Set(), in_cancel:new Set(), cancelled:new Set()};
    let accepted = 0;
    for (const tip of tips) {
      byKind[tip.kind].add(tip.orderNumber);
      const key = eventKey(tip);
      if (pending.has(key) || done.has(key)) continue;
      pending.set(key, tip);
      accepted++;
    }
    if (source === 'websocket') lastSocketAt = receivedAt;
    else lastCheckAt = receivedAt;
    batches.unshift({
      source, receivedAt, newOrders:byKind.new_order.size,
      cancelling:byKind.in_cancel.size, cancelled:byKind.cancelled.size,
      accepted
    });
    if (batches.length > 30) batches.pop();
    save().catch(() => console.error('[订单消息] 待处理队列写入失败')).finally(kick);
  }
  function kick() {
    if (closed || running) return;
    clearTimeout(timer);
    running = drain().catch(() => console.error('[订单消息] 队列处理失败')).finally(() => {
      running = null;
      if (!closed && pending.size) timer = setTimeout(kick,5000);
    });
  }
  async function drain() {
    for (const [k,tip] of pending) {
      if (closed || session.state !== 'verified' || session.sync?.state === 'running' || session.packing) return;
      if (tip.nextTry > Date.now()) continue;
      const page = context.pages().find(p => p.url().startsWith(origin+'/order/'));
      if (!page) return;
      try {
        activeQueries.set(page,tip.orderNumber);
        const capturedAt = new Date().toISOString();
        let client = clients.get(page);
        if (!client) {
          client = await nativeClient(page);
          clients.set(page,client);
        }
        const packages = await lookupOrderPackages(page,tip.orderNumber,() => client);
        const record = {version:1, accountId:id, messageId:tip.messageId, orderNumber:tip.orderNumber, kind:tip.kind, capturedAt, packages};
        await atomicJSON(path.join(inbox, capturedAt.replaceAll(':','-')+'-'+randomUUID()+'.json'), record);
        lastQueuedAt = new Date().toISOString();
        pending.delete(k);
        done.add(k);
        if (done.size > 5000) done.delete(done.values().next().value);
      } catch (error) {
        clients.delete(page);
        tip.attempts = (tip.attempts || 0) + 1;
        tip.nextTry = Date.now() + Math.min(15 * 60, 5 * 2 ** Math.min(tip.attempts,8))*1000;
        console.warn('[订单消息] 单号查询待重试', tip.messageId, tip.attempts);
      } finally {
        activeQueries.delete(page);
      }
      await save();
    }
  }
  function track(page) {
    if (metadata.has(page)) return;
    const meta = {identity:null, socketURL:null, deferred:[]};
    metadata.set(page,meta);
    page.on('framenavigated', frame => {if (frame === page.mainFrame()) clients.delete(page);});
    function acceptFrame(socketURL, frame) {
      if (socketURL !== meta.socketURL) return;
      const sub = text(frame.content?.subAccountId);
      if (!meta.identity.main && (!sub || sub !== meta.identity.sub)) return;
      receive('websocket', socketTips(frame,meta.identity));
    }
    page.on('websocket', ws => {
      ws.on('framereceived', ({payload}) => {
        if (closed || !page.url().startsWith(origin+'/')) return;
        try {
          const frame = parse(payload.toString());
          if (frame?.action !== 'ORDER_TIP_MSG') return;
          if (meta.identity && meta.socketURL) acceptFrame(ws.url(), frame);
          else if (meta.deferred.length < 50) meta.deferred.push([ws.url(), frame]);
        } catch {}
      });
    });
    page.on('response', async response => {
      if (closed || !response.ok()) return;
      const url = new URL(response.url());
      if (url.origin !== origin || ![wsInfoPath,identityPath,checkPath].includes(url.pathname)) return;
      try {
        const data = parse(await response.text());
        if (closed || data?.result !== 'success') return;
        if (url.pathname === wsInfoPath) meta.socketURL = new URL(data.wsSocketAddress).href;
        else if (url.pathname === identityPath) meta.identity = {main:!('subAccountId' in data) || !!data.isSubAdmin, sub:text(data.subAccountId)};
        else {
          receive('http', orderTips(data.orderTipMsgList));
        }
        if (meta.socketURL && meta.identity) {
          for (const [socketURL, frame] of meta.deferred.splice(0)) acceptFrame(socketURL,frame);
        }
      } catch {}
    });
  }
  context.on('page',track);
  for (const page of context.pages()) track(page);
  kick();
  return {
    status() {return {pending:pending.size, lastSocketAt, lastCheckAt, lastQueuedAt, batches:[...batches]};},
    handles(response) {
      const req = response.request();
      if (!eventSearch(req)) return false;
      try {return activeQueries.get(req.frame().page()) === new URLSearchParams(req.postData()).get('platformOrderSns');}
      catch {return false;}
    },
    async stop() {
      closed = true; clearTimeout(timer);
      context.off('page',track);
      await running;
      await saving;
    }
  };
}
