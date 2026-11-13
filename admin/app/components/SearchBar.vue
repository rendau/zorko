<script setup lang="ts">
import type { IssueScope } from '~/api/issue.api'
import type { SearchToken } from '~/utils/search'

// Строка поиска как в Sentry: подсказки ключей тегов, а после двоеточия — значений
const props = defineProps<{ scope: IssueScope }>()
const model = defineModel<string>({ required: true })
const { t } = useI18n()

const text = ref(model.value)
watch(model, (v) => {
  text.value = v
})

const input = ref<{ inputRef: HTMLInputElement } | null>(null)
const open = ref(false)
const active = ref(0)
const keys = ref<string[]>([])
const values = ref<string[]>([])
const token = ref<SearchToken>(tokenAt('', 0))

let keysFor = ''
async function loadKeys() {
  const scopeKey = JSON.stringify(props.scope)
  if (keysFor === scopeKey)
    return
  keysFor = scopeKey
  try {
    keys.value = (await tagApi.keys(props.scope)).keys
  }
  catch {
    keys.value = []
  }
}

// Подсказки значений. Каждый запрос на сервере читает теги всех событий периода, поэтому:
// запрос уходит после паузы в наборе, а не на каждую букву; прежний, ещё не отвеченный, отменяется;
// повтор того же запроса (щелчок, фокус, стрелки) не шлётся.
const VALUES_DELAY_MS = 250
let valuesTimer: ReturnType<typeof setTimeout> | undefined
let valuesAbort: AbortController | null = null
let valuesFor = ''
let valuesKey = ''

function cancelValues() {
  clearTimeout(valuesTimer)
  valuesAbort?.abort()
  valuesAbort = null
}

function loadValues(key: string, prefix: string) {
  const want = JSON.stringify([props.scope, key, prefix])
  if (want === valuesFor)
    return
  valuesFor = want
  cancelValues()

  // значения прежнего ключа под новым ключом не показываем
  if (key !== valuesKey) {
    valuesKey = key
    values.value = []
  }

  // сразу после двоеточия ждать нечего; дальше — пауза, пока человек печатает
  valuesTimer = setTimeout(async () => {
    const controller = new AbortController()
    valuesAbort = controller
    try {
      const rep = await tagApi.values(props.scope, key, prefix, controller.signal)
      if (!controller.signal.aborted)
        values.value = rep.values
    }
    catch {
      if (!controller.signal.aborted) {
        values.value = []
        valuesFor = ''
      }
    }
  }, prefix ? VALUES_DELAY_MS : 0)
}

onBeforeUnmount(cancelValues)

const suggestions = computed<{ label: string, apply: () => void }[]>(() => {
  const tk = token.value
  if (tk.hasColon)
    return values.value.map(v => ({ label: `${tk.key}:${v}`, apply: () => applyValue(v) }))
  const prefix = tk.key.toLowerCase()
  return keys.value
    .filter(k => k.toLowerCase().startsWith(prefix))
    .slice(0, 12)
    .map(k => ({ label: `${k}:`, apply: () => applyKey(k) }))
})

function onInput() {
  const el = input.value?.inputRef
  token.value = tokenAt(text.value, el?.selectionStart ?? text.value.length)
  open.value = true
  active.value = 0
  if (token.value.hasColon)
    loadValues(token.value.key, token.value.value)
}

function focusAt(pos: number) {
  nextTick(() => {
    const el = input.value?.inputRef
    el?.focus()
    el?.setSelectionRange(pos, pos)
    onInput()
  })
}

function applyKey(key: string) {
  const r = replaceToken(text.value, token.value, key)
  text.value = r.q
  focusAt(r.cursor)
}

function applyValue(value: string) {
  const r = replaceToken(text.value, token.value, token.value.key, value)
  text.value = r.q
  submit()
}

function submit() {
  open.value = false
  model.value = text.value.trim()
}

function onKeydown(e: KeyboardEvent) {
  const list = suggestions.value
  if (e.key === 'ArrowDown' && list.length) {
    e.preventDefault()
    open.value = true
    active.value = (active.value + 1) % list.length
  }
  else if (e.key === 'ArrowUp' && list.length) {
    e.preventDefault()
    active.value = (active.value - 1 + list.length) % list.length
  }
  else if (e.key === 'Enter') {
    e.preventDefault()
    const pick = list[active.value]
    if (open.value && pick && (token.value.key || token.value.hasColon))
      pick.apply()
    else
      submit()
  }
  else if (e.key === 'Escape') {
    open.value = false
  }
}

function clear() {
  text.value = ''
  submit()
}
</script>

<template>
  <div class="relative w-full">
    <UInput
      ref="input"
      v-model="text"
      icon="i-lucide-search"
      :placeholder="t('issues.search.placeholder')"
      class="w-full"
      autocomplete="off"
      spellcheck="false"
      @focus="loadKeys(); onInput()"
      @input="onInput"
      @click="onInput"
      @keydown="onKeydown"
      @blur="open = false"
    >
      <template v-if="text" #trailing>
        <UButton color="neutral" variant="link" size="xs" icon="i-lucide-x" :aria-label="t('common.clear')" @mousedown.prevent @click="clear" />
      </template>
    </UInput>
    <div
      v-if="open && suggestions.length"
      class="absolute z-30 mt-1 max-h-72 w-full overflow-y-auto rounded-md border border-default bg-default py-1 shadow-lg"
      role="listbox"
    >
      <button
        v-for="(s, i) in suggestions"
        :key="s.label"
        type="button"
        role="option"
        :aria-selected="i === active"
        class="block w-full truncate px-3 py-1.5 text-left font-mono text-xs"
        :class="i === active ? 'bg-elevated text-highlighted' : 'text-default'"
        @mousedown.prevent="s.apply()"
        @mouseenter="active = i"
      >
        {{ s.label }}
      </button>
    </div>
  </div>
</template>
