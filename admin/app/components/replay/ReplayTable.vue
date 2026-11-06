<script setup lang="ts">
import type { ReplaySummary } from '~/types/replay'

// Таблица записей: в разделе «Записи» и в карточке проблемы (там — время ошибки и переход к ней)
const props = defineProps<{ items: ReplaySummary[], showProject?: boolean }>()
const { t } = useI18n()
const org = useOrg()

function link(r: ReplaySummary) {
  return { path: `/replays/${r.id}`, query: { project: r.project_id, at: r.error_at || undefined } }
}

function user(r: ReplaySummary): string {
  return r.user_email || r.user_id || r.user_ip
}

const cols = computed(() => props.showProject
  ? 'grid-cols-[9rem_1fr_9rem_9rem_5rem_4rem]'
  : 'grid-cols-[9rem_1fr_9rem_5rem_4rem]')
</script>

<template>
  <div class="overflow-x-auto rounded-lg border border-default">
    <div class="min-w-[760px] divide-y divide-default">
      <div class="grid items-center gap-4 bg-elevated/50 px-4 py-2 text-xs font-medium text-muted" :class="cols">
        <span>{{ t('replay.column.time') }}</span>
        <span>{{ t('replay.column.page') }}</span>
        <span>{{ t('replay.column.user') }}</span>
        <span v-if="showProject">{{ t('replay.column.project') }}</span>
        <span class="text-right">{{ t('replay.column.duration') }}</span>
        <span class="text-right">{{ t('replay.column.errors') }}</span>
      </div>
      <NuxtLink
        v-for="r in items"
        :key="r.id"
        :to="link(r)"
        class="grid items-center gap-4 px-4 py-2.5 text-sm hover:bg-elevated/50"
        :class="cols"
      >
        <div class="min-w-0">
          <div class="truncate"><RelativeTime :value="r.error_at || r.started_at" /></div>
          <div class="truncate text-xs text-dimmed">
            <UIcon :name="r.replay_type === 'session' ? 'i-lucide-film' : 'i-lucide-bug-play'" class="size-3 align-[-2px]" />
            {{ r.replay_type === 'session' ? t('replay.type.session') : t('replay.type.buffer') }}
          </div>
        </div>
        <div class="min-w-0">
          <div class="truncate font-mono text-xs" :title="r.url">{{ r.url || '—' }}</div>
          <div class="truncate text-xs text-muted">{{ [r.browser, r.os].filter(Boolean).join(' · ') }}<template v-if="r.release"> · {{ r.release }}</template></div>
        </div>
        <span class="truncate text-xs" :title="user(r)">{{ user(r) || t('replay.anonymous') }}</span>
        <span v-if="showProject" class="truncate text-xs">{{ org.projectName(r.project_id) }}</span>
        <span class="text-right text-xs tabular-nums">{{ formatOffset(Number(r.duration_ms)) }}</span>
        <span class="text-right">
          <UBadge v-if="r.error_count" color="error" variant="subtle" size="sm">{{ r.error_count }}</UBadge>
          <span v-else class="text-xs text-dimmed">0</span>
        </span>
      </NuxtLink>
    </div>
  </div>
</template>
