// node --test tests/ — генерация пароля (app/utils/password.ts)
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { generatePassword, randomInt } from '../app/utils/password.ts'

test('длина и все группы символов', () => {
  for (let i = 0; i < 200; i++) {
    const p = generatePassword()
    assert.equal(p.length, 16)
    assert.match(p, /[a-z]/)
    assert.match(p, /[A-Z]/)
    assert.match(p, /[2-9]/)
    assert.match(p, /[!#$%&+=?]/)
    assert.doesNotMatch(p, /[0O1lI*_~`@\s]/, 'без похожих символов и разметки мессенджеров')
  }
})

test('пароли разные', () => {
  const set = new Set(Array.from({ length: 100 }, () => generatePassword()))
  assert.equal(set.size, 100)
})

test('randomInt в диапазоне', () => {
  for (let i = 0; i < 1000; i++) {
    const v = randomInt(7)
    assert.ok(v >= 0 && v < 7)
  }
})
