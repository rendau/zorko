// Проверка сборки (ADR-007): ровно один JS и один CSS, без динамических подгрузок.
import { readdirSync } from 'node:fs'
import { extname, join } from 'node:path'

const dir = process.argv[2] ?? '.output/public'
const files = readdirSync(dir, { recursive: true, withFileTypes: true })
  .filter(e => e.isFile())
  .map(e => join(e.parentPath, e.name))

const js = files.filter(f => extname(f) === '.js' && !f.endsWith('config.js'))
const css = files.filter(f => extname(f) === '.css')

const problems = []
if (js.length !== 1)
  problems.push(`ожидался 1 JS-файл, найдено ${js.length}: ${js.join(', ')}`)
if (css.length !== 1)
  problems.push(`ожидался 1 CSS-файл, найдено ${css.length}: ${css.join(', ')}`)

if (problems.length) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log(`bundle ok: ${js[0]}, ${css[0]}`)
