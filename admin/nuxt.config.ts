// Интерфейс zorko. SPA без SSR: собранная статика (nuxt generate → .output/public)
// раздаётся Go-сервером на ADMIN_PORT (internal/app/admin_http_server.go), адрес API
// подставляется в рантайме через /config.js (window.__APP_CONFIG__): один билд на все
// окружения. Дев: `pnpm dev` + .env (NUXT_PUBLIC_API_BASE_URL).
export default defineNuxtConfig({
  ssr: false,

  modules: ['@nuxt/ui', '@nuxt/eslint', '@nuxtjs/i18n'],

  // Тексты — ключами в i18n/locales/*.json, подключены статически в i18n/i18n.config.ts:
  // никаких отдельных загрузок локалей (ADR-007). Русский по умолчанию, выбор хранит
  // plugins/locale.client.ts; язык в URL не попадает.
  i18n: {
    defaultLocale: 'ru',
    strategy: 'no_prefix',
    detectBrowserLanguage: false,
    locales: [
      { code: 'ru', language: 'ru-RU', name: 'Русский' },
      { code: 'en', language: 'en-GB', name: 'English' },
    ],
  },

  // Иконки и шрифты — только из сборки, без запросов в сеть (как в planora)
  icon: {
    provider: 'none',
    clientBundle: {
      scan: { globInclude: ['**/*.{vue,ts}'] },
    },
  },
  ui: {
    fonts: false,
  },

  // клиенты API (app/api/*.api.ts) — автоимпортом, как utils и composables
  imports: {
    dirs: ['api'],
  },

  css: ['~/assets/css/main.css'],

  app: {
    head: {
      title: 'zorko',
      htmlAttrs: { lang: 'ru' },
      link: [{ rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
      // рантайм-конфиг: в проде /config.js отдаёт Go-сервер, в деве — заглушка public/config.js
      script: [{ src: '/config.js' }],
    },
  },

  runtimeConfig: {
    public: {
      apiBaseUrl: '',
    },
  },

  // Один бандл без динамических подгрузок (ADR-007): страницы и всё остальное — в одном JS,
  // стили — в одном CSS. Проверяет scripts/check-bundle.mjs после сборки.
  vite: {
    build: {
      cssCodeSplit: false,
      chunkSizeWarningLimit: 5000,
      rollupOptions: {
        output: {
          inlineDynamicImports: true,
        },
      },
    },
  },

  // без опроса /_nuxt/builds/latest.json: интерфейс не ходит в сеть сам по себе
  experimental: {
    appManifest: false,
  },

  typescript: {
    strict: true,
    typeCheck: false, // проверка — `pnpm typecheck`
  },

  devtools: { enabled: false },
  compatibilityDate: '2026-01-01',
})
