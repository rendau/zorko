// Код подключения SDK Sentry к zorko для популярных платформ: меняется только DSN

export interface SdkSnippet {
  id: string
  label: string
  install: string
  code: string
}

export function sdkSnippets(dsn: string): SdkSnippet[] {
  return [
    {
      id: 'vue',
      label: 'Vue',
      install: 'npm install @sentry/vue',
      code: `import * as Sentry from '@sentry/vue'

Sentry.init({
  app,
  dsn: '${dsn}',
  release: import.meta.env.VITE_RELEASE,
  environment: import.meta.env.MODE,
})`,
    },
    {
      id: 'react',
      label: 'React',
      install: 'npm install @sentry/react',
      code: `import * as Sentry from '@sentry/react'

Sentry.init({
  dsn: '${dsn}',
  release: import.meta.env.VITE_RELEASE,
  environment: import.meta.env.MODE,
})`,
    },
    {
      id: 'browser',
      label: 'JavaScript',
      install: 'npm install @sentry/browser',
      code: `import * as Sentry from '@sentry/browser'

Sentry.init({
  dsn: '${dsn}',
  release: 'my-app@1.0.0',
  environment: 'production',
})`,
    },
    {
      id: 'flutter',
      label: 'Flutter',
      install: 'flutter pub add sentry_flutter',
      code: `import 'package:sentry_flutter/sentry_flutter.dart';

Future<void> main() async {
  await SentryFlutter.init(
    (options) {
      options.dsn = '${dsn}';
      options.environment = 'production';
    },
    appRunner: () => runApp(const MyApp()),
  );
}`,
    },
    {
      id: 'react-native',
      label: 'React Native',
      install: 'npm install @sentry/react-native',
      code: `import * as Sentry from '@sentry/react-native'

Sentry.init({
  dsn: '${dsn}',
  environment: 'production',
})

export default Sentry.wrap(App)`,
    },
  ]
}

export const PLATFORMS = ['vue', 'react', 'javascript', 'flutter', 'react-native', 'node', 'other']

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
  install: string
  code: string
  cli: string
}

// buildSnippet — заливка карт при сборке: @sentry/vite-plugin и то же голым sentry-cli.
// Токен CI организации (SENTRY_AUTH_TOKEN) несёт адрес и организацию; .map не уезжают на сайт.
export function buildSnippet(url: string, org: string, project: string): BuildSnippet {
  return {
    install: 'npm install --save-dev @sentry/vite-plugin',
    code: `// vite.config.js
import { defineConfig } from 'vite'
import { sentryVitePlugin } from '@sentry/vite-plugin'

export default defineConfig({
  build: { sourcemap: 'hidden' },
  plugins: [
    sentryVitePlugin({
      url: '${url}/',
      org: '${org}',
      project: '${project}',
      authToken: process.env.SENTRY_AUTH_TOKEN,
      release: { name: process.env.VITE_RELEASE },
      sourcemaps: { filesToDeleteAfterUpload: ['./dist/**/*.map'] },
      telemetry: false,
    }),
  ],
})`,
    cli: `npx @sentry/cli sourcemaps inject ./dist
npx @sentry/cli sourcemaps upload --org ${org} --project ${project} --release "$VITE_RELEASE" ./dist
find ./dist -name '*.map' -delete`,
  }
}
