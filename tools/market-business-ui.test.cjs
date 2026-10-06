const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../internal/entry/webadmin/assets/app.js'), 'utf8');
const lines = source.split(/\r?\n/);

function setup() {
  const calls = [], dialogs = [], notices = [];
  const elements = { marketBusinessJSON: { value: '{"version":2,"items":[]}' }, marketCollectPreviewResult: {} };
  const status = { config: { version: 2, items: [], limits: { daily_gold: 1000 } }, items: [{ item_id: 1, name: '</textarea><script>bad()</script>', sell_price: 200, buy_max_price: 100, stock: 7, target: 10, used_quantity: 2, daily_quantity: 5 }], pending_trades: 1 };
  const context = {
    busy: false, console: { log() {} }, document: { getElementById: id => elements[id] },
    setBusy(value) { context.busy = value; },
    i18nText: text => text, i18nFormat: key => key,
    toast: text => notices.push(text), closeModal() {},
    showModal(...args) { dialogs.push(args); },
    async api(command, payload) {
      calls.push({ command, payload });
      return { result: { result: command === 'marketCollectPreview' ? { actions: [], skipped: [{ reason: '超价拒收' }] } : status } };
    }
  };
  vm.createContext(context);
  vm.runInContext(lines.find(line => line.startsWith('function byId')) + '\n' + lines.find(line => line.startsWith('function resultOf')) + '\n' + lines.find(line => line.startsWith('async function guarded')) + '\n' + source.slice(source.indexOf('async function openMarketBusinessDialog')), context);
  return { context, calls, dialogs, elements, notices };
}

test('经营页展示库存和买卖分价，转义物品名称且不显示旧重建按钮', async () => {
  const { context, dialogs } = setup();
  await context.openMarketBusinessDialog();
  assert.equal(dialogs.length, 1);
  assert.match(dialogs[0][1], /最高收购价/);
  assert.match(dialogs[0][1], /7 \/ 10/);
  assert.doesNotMatch(dialogs[0][1], /<script>bad/);
  assert.equal(dialogs[0][3], 'market business');
  assert.equal(dialogs[0][4], false);
});

test('保存后等待 busy 释放再刷新经营清单', async () => {
  const { context, calls, dialogs } = setup();
  await context.saveMarketBusiness();
  assert.deepEqual(calls.map(c => c.command), ['marketBusinessUpdate', 'marketBusinessStatus']);
  assert.equal(dialogs.length, 1);
  assert.equal(context.busy, false);
});

test('无效 JSON 不保存，预览只调用规划接口并显示拒收原因', async () => {
  const { context, calls, elements, notices } = setup();
  elements.marketBusinessJSON.value = '{broken';
  await context.saveMarketBusiness();
  assert.equal(calls.length, 0);
  assert.match(notices[0], /JSON 格式错误/);
  await context.previewMarketBusinessCollect();
  assert.equal(calls[0].command, 'marketCollectPreview');
  assert.match(elements.marketCollectPreviewResult.textContent, /超价拒收/);
});
