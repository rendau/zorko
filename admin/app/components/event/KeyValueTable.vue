<script setup lang="ts">
// Пары «ключ — значение» (контекст, теги, extra)
defineProps<{ items: [string, string][] }>()
</script>

<template>
  <table class="w-full table-fixed text-xs">
    <tbody>
      <tr v-for="[k, v] in items" :key="k" class="border-b border-default last:border-b-0">
        <td class="w-1/3 py-1 pe-3 align-top text-muted break-words">{{ k }}</td>
        <td class="py-1 align-top font-mono break-all text-highlighted">{{ v }}</td>
      </tr>
    </tbody>
  </table>
</template>
