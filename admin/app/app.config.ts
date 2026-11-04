export default defineAppConfig({
  ui: {
    colors: {
      primary: 'violet',
      neutral: 'zinc',
    },
    // шапка страницы на узком экране: кнопки справа переносятся на следующую строку (как в planora)
    dashboardNavbar: {
      slots: {
        root: 'h-auto min-h-(--ui-header-height) flex-wrap gap-y-2 py-2',
        right: 'shrink flex-wrap min-w-0',
      },
    },
  },
})
