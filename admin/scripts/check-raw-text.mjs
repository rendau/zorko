// Проверка, что видимый текст админки идёт через ключи локалей, а не вписан
// в код (pnpm check:i18n, вместе с check-locales.mjs). Ищет кириллицу в
// строках app/**/*.vue и app/**/*.ts: в тексте и атрибутах шаблона, в
// выражениях шаблона ({{ }}, :prop, v-if…) и в строковых литералах скрипта.
// Разбор — синтаксическими деревьями (vue/compiler-sfc, typescript), а не
// регулярками, поэтому комментарии (//, /* */, <!-- -->) не проверяются
// вовсе: их в дереве нет.
//
// Данные — не подписи и переводиться не должны: статусы Jira, имя поля
// Jira, примеры на схеме «Как это работает». Они проходят двумя путями:
//   - строка целиком из DATA_VALUES (статусы Jira, «План на месяц») —
//     разрешена где угодно;
//   - пометка `i18n-data` с причиной — комментарий `// i18n-data: …` перед
//     объявлением или выражением скрипта, `<!-- i18n-data: … -->` перед
//     элементом шаблона. Пометка снимает проверку с этого узла целиком
//     (объявления, элемента со всем содержимым), не с файла.
// Что остаётся непокрытым — в CLAUDE.md, раздел «Многоязычность админки».
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { parse as parseSfc } from 'vue/compiler-sfc'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const appDir = join(root, 'app')

// Строки-данные, разрешённые везде: статусы Jira (metric/model.StatusOrder
// данные, а не тексты интерфейса (пока таких нет)
const DATA_VALUES = new Set([])

const CYRILLIC = /[Ѐ-ӿ]/
const PRAGMA = 'i18n-data'

const problems = []

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name)
    if (statSync(path).isDirectory())
      walk(path, out)
    else if (/\.(vue|ts)$/.test(name) && !name.endsWith('.d.ts'))
      out.push(path)
  }
  return out
}

function report(file, line, text) {
  problems.push(`${relative(root, file)}:${line}  ${JSON.stringify(text.trim().slice(0, 90))}`)
}

function isData(text) {
  return DATA_VALUES.has(text.trim())
}

// ── скрипт ──
// lineOffset — номер строки файла, с которой начинается разбираемый кусок
// (блок <script> или выражение шаблона), для сообщения
function checkScript(file, code, lineOffset) {
  const sf = ts.createSourceFile(file, code, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  const marked = (node) => {
    const ranges = ts.getLeadingCommentRanges(code, node.getFullStart()) ?? []
    return ranges.some(r => code.slice(r.pos, r.end).includes(PRAGMA))
  }
  const visit = (node) => {
    if (marked(node))
      return
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)
      || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) {
      const text = node.text
      // ключ import и имя модуля не текст, а путь
      if (CYRILLIC.test(text) && !isData(text) && !ts.isImportDeclaration(node.parent)) {
        const { line } = sf.getLineAndCharacterOfPosition(node.getStart())
        report(file, lineOffset + line, text)
      }
    }
    ts.forEachChild(node, visit)
  }
  visit(sf)
}

// ── шаблон ──
const NodeTypes = { ELEMENT: 1, TEXT: 2, COMMENT: 3, INTERPOLATION: 5, ATTRIBUTE: 6, DIRECTIVE: 7 }

function checkExpression(file, exp, line) {
  if (!exp || !CYRILLIC.test(exp))
    return
  checkScript(file, `(${exp})`, line)
}

function checkChildren(file, children) {
  let skipNext = false
  for (const node of children) {
    if (node.type === NodeTypes.COMMENT) {
      if (node.content.includes(PRAGMA))
        skipNext = true
      continue
    }
    if (node.type === NodeTypes.TEXT && !node.content.trim())
      continue
    if (skipNext) {
      skipNext = false
      continue
    }
    checkNode(file, node)
  }
}

function checkNode(file, node) {
  const line = node.loc.start.line
  if (node.type === NodeTypes.TEXT) {
    // узел текста начинается сразу после тега — с переноса строки
    const textLine = line + (node.loc.source.match(/^\s*/)[0].match(/\n/g)?.length ?? 0)
    if (CYRILLIC.test(node.content) && !isData(node.content))
      report(file, textLine, node.content)
    return
  }
  if (node.type === NodeTypes.INTERPOLATION) {
    checkExpression(file, node.content.content, line)
    return
  }
  if (node.type !== NodeTypes.ELEMENT)
    return
  for (const prop of node.props) {
    if (prop.type === NodeTypes.ATTRIBUTE && prop.value && CYRILLIC.test(prop.value.content) && !isData(prop.value.content))
      report(file, prop.loc.start.line, `${prop.name}="${prop.value.content}"`)
    if (prop.type === NodeTypes.DIRECTIVE) {
      if (prop.exp)
        checkExpression(file, prop.exp.content, prop.exp.loc.start.line)
      // динамический аргумент слота с текстом — #[`status:${s}-header`]
      if (prop.arg && !prop.arg.isStatic)
        checkExpression(file, prop.arg.content, prop.arg.loc.start.line)
    }
  }
  checkChildren(file, node.children)
}

for (const file of walk(appDir)) {
  const src = readFileSync(file, 'utf8')
  if (!CYRILLIC.test(src))
    continue
  if (file.endsWith('.ts')) {
    checkScript(file, src, 1)
    continue
  }
  const { descriptor } = parseSfc(src, { filename: file })
  for (const block of [descriptor.script, descriptor.scriptSetup]) {
    if (block)
      checkScript(file, block.content, block.loc.start.line)
  }
  if (descriptor.template?.ast)
    checkChildren(file, descriptor.template.ast.children)
}

if (problems.length) {
  console.error(`Текст вне локалей (${problems.length}) — вынесите в i18n/locales/*.json или пометьте данные i18n-data:`)
  for (const p of problems)
    console.error(`  ${p}`)
  process.exit(1)
}
console.log('raw text ok: кириллицы вне локалей нет')
