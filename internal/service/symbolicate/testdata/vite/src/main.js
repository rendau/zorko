import { cartUtils } from './promo.js'

document.getElementById('go').addEventListener('click', async function onGoClick() {
  const { default: openCart } = await import('./page.js')
  console.log(cartUtils.totalOf([{ price: 1, qty: 2 }]), /\/cart\//.test(location.pathname))
  openCart()
})
