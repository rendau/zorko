// Уровень события → цвет метки (семантические цвета Nuxt UI)
export type LevelColor = 'error' | 'warning' | 'info' | 'neutral'

export function levelColor(level: string): LevelColor {
  switch (level) {
    case 'fatal':
    case 'error':
      return 'error'
    case 'warning':
      return 'warning'
    case 'info':
      return 'info'
    default:
      return 'neutral'
  }
}

// классы целиком строками: Tailwind находит их в исходниках (собранные динамически — не находит)
const BAR_CLASS: Record<LevelColor, string> = {
  error: 'bg-error',
  warning: 'bg-warning',
  info: 'bg-info',
  neutral: 'bg-accented',
}

export function levelBarClass(level: string): string {
  return BAR_CLASS[levelColor(level)]
}
