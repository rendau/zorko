// node --test tests/ — разбор события Sentry (app/utils/event.ts)
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

import { parseEvent, secondsBefore } from '../app/utils/event.ts'

// настоящая пачка @sentry/node 11.10.0 из тестов сервера
const fixture = readFileSync(new URL('../../internal/service/normalize/service/testdata/node-exception.json', import.meta.url), 'utf8')

test('исключение настоящего SDK', () => {
  const ev = parseEvent(fixture)
  assert.equal(ev.exceptions.length, 1)
  const exc = ev.exceptions[0]
  assert.equal(exc.type, 'TypeError')
  assert.equal(exc.mechanism, 'generic')
  assert.equal(exc.handled, true)
  assert.equal(exc.frames[0].function, 'addItem', 'место падения — первым')
  assert.equal(exc.frames[0].inApp, true)
  assert.ok(exc.frames[0].context.some(l => l.current), 'строка ошибки отмечена')
  assert.ok(ev.breadcrumbs.length > 0)
  assert.deepEqual(ev.tags.find(([k]) => k === 'page'), ['page', 'cart'])
  assert.equal(ev.contexts[0].key, 'user', 'пользователь — первым')
})

test('формы полей и in_app по пути', () => {
  const ev = parseEvent(JSON.stringify({
    exception: [{ type: 'Error', value: 'boom', stacktrace: { frames: [
      { filename: '/app/node_modules/vue/x.js', function: 'callHook' },
      { filename: '/src/cart.js', function: 'addItem', lineno: 10, context_line: 'x()', pre_context: ['a', 'b'], post_context: ['c'] },
    ] } }],
    tags: [['b', 2], ['a', '1']],
    breadcrumbs: [{ timestamp: 1700000000.5, category: 'ui.click', message: 'button', data: { id: 7 } }],
    request: { url: 'https://shop/cart', headers: [['User-Agent', 'X']] },
    logentry: { message: 'Order %s', formatted: 'Order 1' },
  }))
  const [exc] = ev.exceptions
  assert.equal(exc.frames[0].inApp, true)
  assert.equal(exc.frames[1].inApp, false, 'node_modules — не свой код')
  assert.deepEqual(exc.frames[0].context.map(l => l.line), [8, 9, 10, 11])
  assert.deepEqual(ev.tags, [['a', '1'], ['b', '2']])
  assert.equal(ev.breadcrumbs[0].timestamp, '2023-11-14T22:13:20.500Z')
  assert.deepEqual(ev.breadcrumbs[0].data, [['id', '7']])
  assert.ok(ev.contexts.find(c => c.key === 'request').items.some(([k, v]) => k === 'header: User-Agent' && v === 'X'))
  assert.equal(ev.message, 'Order 1')
})

test('цепочка исключений — пойманное первым', () => {
  const ev = parseEvent(JSON.stringify({ exception: { values: [{ type: 'Cause' }, { type: 'Outer' }] } }))
  assert.deepEqual(ev.exceptions.map(e => e.type), ['Outer', 'Cause'])
})

test('битый JSON не роняет карточку', () => {
  const ev = parseEvent('{oops')
  assert.deepEqual(ev.exceptions, [])
})

test('секунды до ошибки', () => {
  assert.equal(secondsBefore('2026-01-01T00:00:00.000Z', '2026-01-01T00:00:03.200Z'), -3.2)
  assert.equal(secondsBefore('', 'x'), null)
})

test('карты исходников: переведённые кадры, исходный стек и причины', async () => {
  const { sourcemapProblem } = await import('../app/utils/event.ts')
  const ev = parseEvent(JSON.stringify({
    exception: { values: [{
      type: 'RangeError',
      value: 'x',
      stacktrace: { frames: [
        { filename: 'https://s.kz/assets/vendor.js', function: 'a', lineno: 1, colno: 9, data: { zorko_error: 'map_not_found', zorko_site_error: 'site_disabled' } },
        { filename: 'src/promo.js', function: 'validateRule', lineno: 10, colno: 35, in_app: true, data: { symbolicated: true, resolved_with: 'debug_id' } },
      ] },
      raw_stacktrace: { frames: [
        { filename: 'https://s.kz/assets/vendor.js', function: 'a', lineno: 1, colno: 9 },
        { filename: 'https://s.kz/assets/index.js', function: 'yv', lineno: 24, colno: 5350 },
      ] },
    }] },
  }))
  const [exc] = ev.exceptions
  assert.equal(exc.frames[0].symbolicated, true)
  assert.equal(exc.frames[0].function, 'validateRule')
  assert.equal(exc.rawFrames.length, 2)
  assert.equal(exc.rawFrames[0].function, 'yv', 'сжатый вид — тоже место падения первым')
  assert.deepEqual(sourcemapProblem(exc), { count: 1, error: 'map_not_found', siteError: 'site_disabled' })

  const plain = parseEvent(JSON.stringify({ exception: [{ type: 'E', stacktrace: { frames: [{ filename: 'a.js' }] } }] }))
  assert.equal(plain.exceptions[0].rawFrames, null)
  assert.equal(sourcemapProblem(plain.exceptions[0]), null)
})
