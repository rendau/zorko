// Случайный пароль для нового или сброшенного входа (как в kusec). Правила сервера — не короче
// 8 символов и не длиннее 72 байт (internal/usecase/usr); сгенерированный — 16 символов и
// содержит все четыре группы, чтобы пройти и более строгие правила.

export const GENERATED_PASSWORD_LEN = 16

// Похожие символы (0/O, 1/l/I) убраны — пароль могут перепечатывать руками. Из спецсимволов —
// только те, что мессенджеры не превращают в разметку или ссылку (без * _ ~ ` @): пароль
// пересылают сообщением, и он должен дойти как есть.
const CHAR_CLASSES = [
  'abcdefghijkmnpqrstuvwxyz',
  'ABCDEFGHJKLMNPQRSTUVWXYZ',
  '23456789',
  '!#$%&+=?',
]

// randomInt — случайное целое в [0, max) без смещения: числа из неполного «хвоста» uint32
// отбрасываются, а не сворачиваются через %
export function randomInt(max: number): number {
  const range = 0x1_0000_0000
  const limit = range - (range % max)
  const buf = new Uint32Array(1)
  for (;;) {
    crypto.getRandomValues(buf)
    const value = buf[0]!
    if (value < limit)
      return value % max
  }
}

// generatePassword — по символу из каждой группы, остальное из всего алфавита, затем перемешать,
// чтобы обязательные символы не стояли на своих местах
export function generatePassword(length = GENERATED_PASSWORD_LEN): string {
  const alphabet = CHAR_CLASSES.join('')
  const pick = (chars: string): string => chars[randomInt(chars.length)]!

  const chars = CHAR_CLASSES.map(pick)
  while (chars.length < length)
    chars.push(pick(alphabet))

  for (let i = chars.length - 1; i > 0; i--) {
    const j = randomInt(i + 1)
    ;[chars[i], chars[j]] = [chars[j]!, chars[i]!]
  }
  return chars.join('')
}
