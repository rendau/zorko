// node --test tests/ — сегменты записей сессий (app/utils/replay.ts)
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { deflateSync } from 'node:zlib'

import { decodeSegment, formatOffset, mapLimit, playable, replayItems, viewport } from '../app/utils/replay.ts'

// события так, как пишет @sentry/replay 11.x: rrweb + служебные события SDK (type 5);
// время событий — мс, время внутри payload — секунды
const T = 1_793_969_300_000
const sec = ms => (T + ms) / 1000
const events = [
  { type: 4, timestamp: T + 1_000, data: { href: 'https://seller.mechta.kz/orders', width: 1440, height: 900 } },
  { type: 2, timestamp: T + 1_001, data: { node: {}, initialOffset: { top: 0, left: 0 } } },
  { type: 5, timestamp: T + 2_000, data: { tag: 'breadcrumb', payload: { timestamp: sec(2_000), type: 'default', category: 'ui.click', message: 'button.save', data: { node: { tagName: 'button', textContent: 'Сохранить' } } } } },
  { type: 5, timestamp: T + 3_000, data: { tag: 'breadcrumb', payload: { timestamp: sec(3_000), type: 'default', category: 'console', level: 'error', message: 'Order failed', data: { arguments: ['Order failed', { id: 42 }], logger: 'console' } } } },
  { type: 5, timestamp: T + 2_500, data: { tag: 'performanceSpan', payload: { op: 'resource.fetch', description: 'https://seller.mechta.kz/api/order', startTimestamp: sec(2_500), endTimestamp: sec(3_250), data: { method: 'POST', statusCode: 500 } } } },
  // адрес из networkDetailAllowUrls: SDK присылает заголовки (имена — в нижнем регистре) и размеры тел
  { type: 5, timestamp: T + 3_500, data: { tag: 'performanceSpan', payload: { op: 'resource.xhr', description: 'https://seller.mechta.kz/api/order/42', startTimestamp: sec(3_500), endTimestamp: sec(3_620), data: { method: 'PUT', statusCode: 200, request: { size: 57, headers: { 'x-request-id': 'req-1', 'content-type': 'application/json', 'accept': '' } }, response: { size: 1280, headers: { 'content-type': 'application/json', 'content-length': '1280' }, body: { ok: true } } } } } },
  // адрес не из списка: только размер тела и отметка URL_SKIPPED
  { type: 5, timestamp: T + 3_700, data: { tag: 'performanceSpan', payload: { op: 'resource.fetch', description: 'https://mc.yandex.ru/watch/1', startTimestamp: sec(3_700), endTimestamp: sec(3_710), data: { method: 'GET', statusCode: 200, request: { headers: {}, _meta: { warnings: ['URL_SKIPPED'] } }, response: { headers: {}, size: 43, _meta: { warnings: ['URL_SKIPPED'] } } } } } },
  { type: 5, timestamp: T + 2_600, data: { tag: 'performanceSpan', payload: { op: 'resource.img', description: 'logo.png', startTimestamp: sec(2_600), endTimestamp: sec(2_700) } } },
  { type: 5, timestamp: T + 4_000, data: { tag: 'breadcrumb', payload: { timestamp: sec(4_000), category: 'navigation', data: { from: '/orders', to: '/orders/42' } } } },
]

test('сегмент zlib (как хранит сервер) и голый JSON', async () => {
  const zlib = deflateSync(Buffer.from(JSON.stringify(events)))
  assert.equal(zlib[0], 0x78)
  assert.deepEqual(await decodeSegment(zlib.buffer.slice(zlib.byteOffset, zlib.byteOffset + zlib.byteLength)), events)

  const raw = new TextEncoder().encode(JSON.stringify(events))
  assert.equal((await decodeSegment(raw.buffer)).length, events.length)
})

test('время в секундах (так SDK пишет запросы и замеры) приводится к миллисекундам', async () => {
  const mixed = [
    { type: 4, timestamp: 1_793_969_303_000, data: { width: 800, height: 600 } },
    { type: 5, timestamp: 1_793_969_348.3985, data: { tag: 'performanceSpan', payload: { op: 'resource.fetch' } } },
  ]
  const body = deflateSync(Buffer.from(JSON.stringify(mixed)))
  const out = await decodeSegment(body.buffer.slice(body.byteOffset, body.byteOffset + body.byteLength))
  assert.equal(out[0].timestamp, 1_793_969_303_000)
  assert.equal(out[1].timestamp, 1_793_969_348_399)
})

test('вкладки: клики, консоль, запросы, переходы по времени', () => {
  const items = replayItems(events)
  assert.deepEqual(items.map(i => i.kind), ['click', 'network', 'console', 'network', 'network', 'navigation'], 'картинки — не запросы к серверу')

  const [click, req, log, , , nav] = items
  assert.equal(click.title, 'button.save')
  assert.equal(click.detail, 'Сохранить')
  assert.equal(req.method, 'POST')
  assert.equal(req.status, 500)
  assert.equal(req.durationMs, 750)
  assert.equal(log.level, 'error')
  assert.match(log.detail, /"id":42/)
  assert.equal(nav.title, '/orders/42')
})

test('заголовки запроса и ответа — только там, где сайт их прислал', () => {
  const [plain, detailed, skipped] = replayItems(events).filter(i => i.kind === 'network')

  assert.equal(plain.request, null, 'SDK без подробностей запросов')
  assert.equal(plain.response, null)

  assert.deepEqual(detailed.request, { headers: [['content-type', 'application/json'], ['x-request-id', 'req-1']], size: 57 }, 'по алфавиту, пустые отброшены')
  assert.deepEqual(detailed.response, { headers: [['content-length', '1280'], ['content-type', 'application/json']], size: 1280 }, 'тело ответа не берём')

  assert.deepEqual(skipped.request, { headers: [], size: 0 })
  assert.deepEqual(skipped.response, { headers: [], size: 43 })

  assert.equal(replayItems(events).find(i => i.kind === 'click').request, null)
})

test('размер окна и время', () => {
  assert.deepEqual(viewport(events), { width: 1440, height: 900 })
  assert.deepEqual(viewport([]), { width: 1280, height: 720 })
  assert.equal(formatOffset(75_400), '1:15')
  assert.equal(formatOffset(3_725_000), '1:02:05')
})

test('не больше limit одновременно, порядок сохраняется', async () => {
  let running = 0
  let peak = 0
  const out = await mapLimit([1, 2, 3, 4, 5, 6, 7], 3, async (v) => {
    running++
    peak = Math.max(peak, running)
    await new Promise(r => setTimeout(r, 5))
    running--
    return v * 10
  })
  assert.deepEqual(out, [10, 20, 30, 40, 50, 60, 70])
  assert.ok(peak <= 3)
})

test('проигрыватель начинает с Meta: служебные события перед снимком отбрасываются', () => {
  const ev = [
    { type: 5, timestamp: T, data: { tag: 'performanceSpan' } },
    { type: 4, timestamp: T + 124, data: {} },
    { type: 2, timestamp: T + 128, data: {} },
  ]
  assert.deepEqual(playable(ev).map(e => e.type), [4, 2])
  assert.equal(playable(events).length, events.length, 'Meta первым — без изменений')
})
