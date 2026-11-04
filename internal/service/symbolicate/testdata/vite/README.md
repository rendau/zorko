Сборка Vite 8.3.5 (`build.sourcemap: 'hidden'`) из `src/`: два куска с `import`/`export`,
`import.meta.url`, динамическим `import()`, регулярным выражением и кириллицей в строке.
Пересобрать: проект с `vite` и `vite.config.js` `{ build: { sourcemap: 'hidden', modulePreload: false } }`.
