// node --test tests/ — сегменты записей сессий (app/utils/replay.ts)
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { deflateSync } from 'node:zlib'

import { decodeSegment, formatOffset, mapLimit, replayItems, viewport } from '../app/utils/replay.ts'

// события так, как пишет @sentry/replay 11.x: rrweb + служебные события SDK (type 5)
const events = [
  { type: 4, timestamp: 1_000, data: { href: 'https://seller.mechta.kz/orders', width: 1440, height: 900 } },
  { type: 2, timestamp: 1_001, data: { node: {}, initialOffset: { top: 0, left: 0 } } },
  { type: 5, timestamp: 2_000, data: { tag: 'breadcrumb', payload: { timestamp: 2.0, type: 'default', category: 'ui.click', message: 'button.save', data: { node: { tagName: 'button', textContent: 'Сохранить' } } } } },
  { type: 5, timestamp: 3_000, data: { tag: 'breadcrumb', payload: { timestamp: 3.0, type: 'default', category: 'console', level: 'error', message: 'Order failed', data: { arguments: ['Order failed', { id: 42 }], logger: 'console' } } } },
  { type: 5, timestamp: 2_500, data: { tag: 'performanceSpan', payload: { op: 'resource.fetch', description: 'https://seller.mechta.kz/api/order', startTimestamp: 2.5, endTimestamp: 3.25, data: { method: 'POST', statusCode: 500 } } } },
  { type: 5, timestamp: 2_600, data: { tag: 'performanceSpan', payload: { op: 'resource.img', description: 'logo.png', startTimestamp: 2.6, endTimestamp: 2.7 } } },
  { type: 5, timestamp: 4_000, data: { tag: 'breadcrumb', payload: { timestamp: 4.0, category: 'navigation', data: { from: '/orders', to: '/orders/42' } } } },
]

test('сегмент zlib (как хранит сервер) и голый JSON', async () => {
  const zlib = deflateSync(Buffer.from(JSON.stringify(events)))
  assert.equal(zlib[0], 0x78)
  assert.deepEqual(await decodeSegment(zlib.buffer.slice(zlib.byteOffset, zlib.byteOffset + zlib.byteLength)), events)

  const raw = new TextEncoder().encode(JSON.stringify(events))
  assert.equal((await decodeSegment(raw.buffer)).length, events.length)
})

test('вкладки: клики, консоль, запросы, переходы по времени', () => {
  const items = replayItems(events)
  assert.deepEqual(items.map(i => i.kind), ['click', 'network', 'console', 'navigation'], 'картинки — не запросы к серверу')

  const [click, req, log, nav] = items
  assert.equal(click.title, 'button.save')
  assert.equal(click.detail, 'Сохранить')
  assert.equal(req.method, 'POST')
  assert.equal(req.status, 500)
  assert.equal(req.durationMs, 750)
  assert.equal(log.level, 'error')
  assert.match(log.detail, /"id":42/)
  assert.equal(nav.title, '/orders/42')
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
