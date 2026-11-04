import { PromoEngine, formatPrice } from './promo.js'

const engine = new PromoEngine([{ code: 'SALE', minTotal: 1000, discount: 10 }])

export default function openCart() {
  const url = new URL('cart', import.meta.url)
  return formatPrice(engine.applyCode({ total: url.href.length }, 'NOPE'))
}
