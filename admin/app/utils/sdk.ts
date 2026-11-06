// Код подключения SDK Sentry к zorko для популярных платформ: меняется только DSN

export interface SdkSnippet {
  id: string
  label: string
  // команды установки: npm и то же через pnpm
  install: string[]
  code: string
}

// Версия и окружение в браузерных примерах на Vite: версию кладёт vite.config (вкладка «Сборка: карты»),
// окружение задаётся явно — MODE на тесте и бою одинаковый (production)
// i18n-data: код примера для разработчиков, комментарии в нём — как в проектах команды
const viteReleaseEnv = `  // Версия сборки — задаётся в vite.config (вкладка «Сборка: карты»). Без версии
  // карты всё равно находятся — по метке сборки (debug id)
  release: import.meta.env.VITE_RELEASE,
  // MODE на тесте и на бою одинаковый (production), поэтому окружение задаём явно
  // при сборке: VITE_SENTRY_ENVIRONMENT=staging
  environment: import.meta.env.DEV
    ? 'development'
    : (import.meta.env.VITE_SENTRY_ENVIRONMENT || 'production'),`

// Отметки сессий (браузерный SDK шлёт по одной на каждое открытие страницы) и отчёты SDK о себе
// zorko принимает и выбрасывает — выключаем, чтобы на сайте с большим трафиком не гонять пустые запросы
// i18n-data: код примера для разработчиков, комментарии в нём — как в проектах команды
function browserNoSessions(indent: string, replay: ReplaySnippet): string {
  if (replay.mode === 'off') {
    return [
      '// Отметки сессий (по одной на каждое открытие страницы) и отчёты SDK о себе zorko',
      '// не хранит — выключаем, иначе на каждый просмотр уходит лишний запрос',
      'integrations: defaults => defaults.filter(i => i.name !== \'BrowserSession\'),',
      'sendClientReports: false,',
    ].map(line => indent + line).join('\n')
  }
  const rate = replay.mode === 'session' ? Math.round(replay.rate * 1000) / 1000 : 0
  return [
    '// Отметки сессий (по одной на каждое открытие страницы) и отчёты SDK о себе zorko',
    '// не хранит — выключаем, иначе на каждый просмотр уходит лишний запрос',
    'integrations: defaults => [',
    '  ...defaults.filter(i => i.name !== \'BrowserSession\'),',
    '  // Запись сессии: весь текст и картинки скрыты (видно, куда нажимали, но не что',
    '  // написано). Безопасные места можно открыть классом sentry-unmask.',
    '  Sentry.replayIntegration({ maskAllText: true, blockAllMedia: true }),',
    '],',
    'sendClientReports: false,',
    '// Запись при ошибке: последняя минута до неё и дальше до конца сессии',
    'replaysOnErrorSampleRate: 1.0,',
    replay.mode === 'session'
      ? `// Доля всех сессий (настройка проекта в zorko: ${Math.round(rate * 1000) / 10} %)`
      : '// Сессии без ошибок не пишем (режим проекта в zorko — «только при ошибке»)',
    `replaysSessionSampleRate: ${rate},`,
  ].map(line => indent + line).join('\n')
}

// ReplaySnippet — запись сессий в примере: режим проекта и доля всех сессий (0..1)
export interface ReplaySnippet {
  mode: 'off' | 'error' | 'session'
  rate: number
}

// i18n-data: код примера для разработчиков, комментарии в нём — как в проектах команды
export function sdkSnippets(dsn: string, replay: ReplaySnippet = { mode: 'off', rate: 0 }): SdkSnippet[] {
  return [
    {
      id: 'vue',
      label: 'Vue',
      install: ['npm install @sentry/vue', 'pnpm add @sentry/vue'],
      code: `import * as Sentry from '@sentry/vue'

Sentry.init({
  app,
  dsn: '${dsn}',
${viteReleaseEnv}
${browserNoSessions('  ', replay)}
})`,
    },
    {
      id: 'nuxt',
      label: 'Nuxt',
      install: ['npm install @sentry/vue', 'pnpm add @sentry/vue'],
      code: `// app/plugins/00.sentry.client.ts (в Nuxt 3 — plugins/00.sentry.client.ts)
// .client — только в браузере; 00. — первым среди ваших плагинов, чтобы ловить и их ошибки
import * as Sentry from '@sentry/vue'

export default defineNuxtPlugin((nuxtApp) => {
  const config = useRuntimeConfig().public

  Sentry.init({
    // app нужен для имён роутов в событиях
    app: nuxtApp.vueApp,
    // Обязательно: иначе Sentry оборачивает app.config.errorHandler, Nuxt не может снять
    // свой обработчик после загрузки, и любая ошибка в компоненте открывает error.vue.
    // Ошибки компонентов берём из хуков Nuxt ниже.
    attachErrorHandler: false,
    dsn: '${dsn}',
    // Версия и окружение — из runtimeConfig в nuxt.config (вкладка «Сборка: карты» → Nuxt).
    // Без версии карты всё равно находятся — по метке сборки (debug id)
    release: config.sentryRelease || undefined,
    // NODE_ENV на тесте и на бою одинаковый (production), поэтому окружение задаём явно
    environment: import.meta.dev ? 'development' : (config.sentryEnvironment || 'production'),
${browserNoSessions('    ', replay)}
  })

  // Ошибка в любом компоненте
  nuxtApp.hook('vue:error', (error, instance, info) => {
    Sentry.captureException(error, {
      mechanism: { handled: false, type: 'auto.function.nuxt.vue_error' },
      captureContext: {
        contexts: {
          vue: {
            componentName: instance?.$options.name ?? instance?.$options.__name,
            lifecycleHook: info,
          },
        },
      },
    })
  })

  // Ошибка при старте приложения и в плагинах
  nuxtApp.hook('app:error', (error) => {
    Sentry.captureException(error, {
      mechanism: { handled: false, type: 'auto.function.nuxt.app_error' },
    })
  })
})`,
    },
    {
      id: 'react',
      label: 'React',
      install: ['npm install @sentry/react', 'pnpm add @sentry/react'],
      code: `import * as Sentry from '@sentry/react'

Sentry.init({
  dsn: '${dsn}',
${viteReleaseEnv}
${browserNoSessions('  ', replay)}
})`,
    },
    {
      id: 'browser',
      label: 'JavaScript',
      install: ['npm install @sentry/browser', 'pnpm add @sentry/browser'],
      code: `import * as Sentry from '@sentry/browser'

Sentry.init({
  dsn: '${dsn}',
  release: 'my-app@1.0.0',
  environment: 'production',
${browserNoSessions('  ', replay)}
})`,
    },
    {
      id: 'flutter',
      label: 'Flutter',
      install: ['flutter pub add sentry_flutter'],
      code: `import 'package:sentry_flutter/sentry_flutter.dart';

Future<void> main() async {
  await SentryFlutter.init(
    (options) {
      options.dsn = '${dsn}';
      options.environment = 'production';
      // Отметки сессий и отчёты SDK о себе zorko не хранит — выключаем лишние запросы
      options.enableAutoSessionTracking = false;
      options.sendClientReports = false;
    },
    appRunner: () => runApp(const MyApp()),
  );
}`,
    },
    {
      id: 'react-native',
      label: 'React Native',
      install: ['npm install @sentry/react-native', 'pnpm add @sentry/react-native'],
      code: `import * as Sentry from '@sentry/react-native'

Sentry.init({
  dsn: '${dsn}',
  environment: 'production',
  // Отметки сессий и отчёты SDK о себе zorko не хранит — выключаем лишние запросы
  enableAutoSessionTracking: false,
  sendClientReports: false,
})

export default Sentry.wrap(App)`,
    },
  ]
}

export const PLATFORMS = ['vue', 'nuxt', 'react', 'javascript', 'flutter', 'react-native', 'node', 'other']

// Мобильные приложения на Capacitor/Cordova шлют события с Origin capacitor://localhost (iOS)
// и https://localhost (Android) — для приёма это хост localhost
export const MOBILE_ORIGIN = 'localhost'

// ingestUrl — адрес сервера приёма из DSN: http(s)://<хост>[/<путь>] без ключа и номера проекта
export function ingestUrl(dsn: string): string {
  try {
    const u = new URL(dsn)
    const path = u.pathname.replace(/\/[^/]*$/, '')
    return `${u.protocol}//${u.host}${path}`
  }
  catch {
    return ''
  }
}

export interface BuildSnippet {
  id: string
  label: string
  install: string[]
  code: string
  // то же голыми командами sentry-cli; пусто — не показываем
  cli: string
}

// Версия сборки — общая для vite.config и nuxt.config: версия из package.json + короткий хеш коммита
// i18n-data: код примера для разработчиков, комментарии в нём — как в проектах команды
function releaseFn(project: string): string {
  return `// Версия сборки: версия из package.json + короткий хеш коммита (${project}@1.4.2-a1b2c3d).
// Та же версия уходит и с ошибками, и с картами
function buildRelease() {
  const { version } = JSON.parse(readFileSync('package.json', 'utf8'))
  let sha = process.env.GITHUB_SHA?.slice(0, 7)
  if (!sha) {
    try {
      sha = execSync('git rev-parse --short HEAD', { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim()
    }
    catch {
      // сборка без git — обходимся версией
    }
  }
  return sha ? \`${project}@\${version}-\${sha}\` : \`${project}@\${version}\`
}

const release = buildRelease()`
}

// Общие поля плагина: адрес, организация и проект обязательны — плагин 11.x не берёт их из токена
// i18n-data: код примера для разработчиков, комментарии в нём — как в проектах команды
function pluginOptions(url: string, org: string, project: string, indent: string): string {
  return [
    `url: '${url}/',`,
    `org: '${org}',`,
    `project: '${project}',`,
    'authToken,',
    '// Карта ищется прежде всего по метке (debug id), которую плагин ставит в каждый файл;',
    '// версия — запасной путь. Коммиты zorko не хранит — их отправку выключаем',
    'release: { name: release, setCommits: false },',
  ].map(line => indent + line).join('\n')
}

const pluginInstall = ['npm install --save-dev @sentry/vite-plugin', 'pnpm add -D @sentry/vite-plugin']

// buildSnippets — заливка карт при сборке: @sentry/vite-plugin для Vite и для Nuxt, то же голым sentry-cli.
// Токен CI организации — в SENTRY_AUTH_TOKEN; без него карты не строятся; .map не уезжают на сайт.
// i18n-data: код примера для разработчиков, комментарии в нём — как в проектах команды
export function buildSnippets(url: string, org: string, project: string): BuildSnippet[] {
  const cli = `npx @sentry/cli sourcemaps inject ./dist
# карта находится по метке (debug id), которую ставит inject; --release "<версия>" — по желанию
npx @sentry/cli sourcemaps upload --org ${org} --project ${project} ./dist
find ./dist -name '*.map' -delete`

  return [
    {
      id: 'vite',
      label: 'Vite',
      install: pluginInstall,
      cli,
      code: `// vite.config.js
import { execSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import { sentryVitePlugin } from '@sentry/vite-plugin'

${releaseFn(project)}

// Без токена (локальная сборка) карты не строятся и не заливаются
const authToken = process.env.SENTRY_AUTH_TOKEN

export default defineConfig({
  // В коде — import.meta.env.VITE_RELEASE (вкладка Vue / React)
  define: { 'import.meta.env.VITE_RELEASE': JSON.stringify(release) },
  // 'hidden' — карты строятся, но ссылки на них в сжатом JS нет
  build: { sourcemap: authToken ? 'hidden' : false },
  plugins: [
    // ...ваши плагины: vue(), react()…
    authToken && sentryVitePlugin({
${pluginOptions(url, org, project, '      ')}
      sourcemaps: { filesToDeleteAfterUpload: ['./dist/**/*.map'] },
      // иначе плагин шлёт свою телеметрию в sentry.io
      telemetry: false,
    }),
  ],
})`,
    },
    {
      id: 'nuxt',
      label: 'Nuxt',
      install: pluginInstall,
      cli: '',
      code: `// nuxt.config.ts
import { execSync } from 'node:child_process'
import { readdirSync, readFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { sentryVitePlugin } from '@sentry/vite-plugin'

${releaseFn(project)}

// Без токена (локальная сборка) карты не строятся вовсе: иначе .map без заливки остались бы на сайте
const authToken = process.env.SENTRY_AUTH_TOKEN ?? ''

// Плагин заливки — только для браузерной части сборки (окружение client)
function sentryPlugins() {
  if (!authToken)
    return []
  return sentryVitePlugin({
${pluginOptions(url, org, project, '    ')}
    // иначе плагин шлёт свою телеметрию в sentry.io
    telemetry: false,
  }).map(plugin => ({
    ...plugin,
    applyToEnvironment: (env: { name: string }) => env.name === 'client',
  }))
}

export default defineNuxtConfig({
  runtimeConfig: {
    public: {
      // В плагине — useRuntimeConfig().public (вкладка Nuxt)
      sentryRelease: release,
      // production / staging: NUXT_PUBLIC_SENTRY_ENVIRONMENT=staging на тестовом стенде
      sentryEnvironment: 'production',
    },
  },

  // Плагин — в vite.plugins, не в vite.$client: там plugins задать нельзя
  vite: { plugins: [...sentryPlugins()] },

  // 'hidden' — карты строятся, но ссылки на них в сжатом JS нет
  sourcemap: { server: false, client: authToken ? 'hidden' : false },

  hooks: {
    // Карты уже залиты — убираем их из готовой статики. Опция плагина
    // filesToDeleteAfterUpload в Nuxt промахивается: промежуточная папка сборки
    // плавает (.nuxt или node_modules/.cache/nuxt/.nuxt), а publicDir — одна
    'nitro:build:public-assets': (nitro) => {
      if (!authToken)
        return
      const publicDir = nitro.options.output.publicDir
      for (const file of readdirSync(publicDir, { recursive: true, encoding: 'utf8' })) {
        if (file.endsWith('.map'))
          rmSync(join(publicDir, file))
      }
    },
  },
})`,
    },
  ]
}
