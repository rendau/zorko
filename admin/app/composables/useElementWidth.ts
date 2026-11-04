import type { Ref } from 'vue'

// Ширина элемента в пикселях — для графиков, которые рисуются в настоящей
// ширине контейнера, а не растягивают viewBox фиксированной ширины: при
// растяжении подписи ужимаются вместе с графиком (на телефоне — втрое) и
// частота подписей не зависит от места. Элемент может появиться позже
// монтирования (внутри v-if) — наблюдение вешается по его появлению.
export function useElementWidth(el: Ref<HTMLElement | null>): Ref<number> {
  const width = ref(0)
  let observer: ResizeObserver | undefined
  watch(el, (node) => {
    observer?.disconnect()
    observer = undefined
    if (!node)
      return
    observer = new ResizeObserver(([entry]) => {
      width.value = Math.floor(entry?.contentRect.width ?? 0)
    })
    observer.observe(node)
  }, { flush: 'post', immediate: true })
  onBeforeUnmount(() => observer?.disconnect())
  return width
}
