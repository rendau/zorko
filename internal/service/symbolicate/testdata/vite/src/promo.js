export class PromoEngine {
  constructor(rules) { this.rules = rules }
  applyCode(cart, code) {
    const rule = this.rules.find(r => r.code === code)
    return validateRule(rule, cart)
  }
}

function validateRule(rule, cart) {
  if (rule.minTotal > cart.total) throw new RangeError('Сумма корзины меньше ' + rule.minTotal)
  return rule.discount
}

export const formatPrice = (value) => {
  return value.toFixed(2) + ' ₸'
}

export const cartUtils = {
  totalOf(items) {
    return items.reduce((s, i) => s + i.price * i.qty, 0)
  },
}
