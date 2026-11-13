<script setup lang="ts">
import type { DropdownMenuItem, NavigationMenuItem } from '@nuxt/ui'

const route = useRoute()
const auth = useAuth()
const org = useOrg()
const { t } = useI18n()
const localeSwitcher = useLocaleSwitcher()
const passwordOpen = ref(false)
const mcpOpen = ref(false)
const mcpEnabled = !!useMcpUrl()

onMounted(() => {
  void org.load()
})

const languageItems = computed<DropdownMenuItem[]>(() => localeSwitcher.items.value.map(l => ({
  label: l.label,
  type: 'checkbox' as const,
  checked: localeSwitcher.current.value === l.code,
  onSelect: () => localeSwitcher.select(l.code),
})))

const userMenu = computed<DropdownMenuItem[][]>(() => [
  [{
    label: auth.user.value?.name || auth.user.value?.username || '',
    description: auth.isAdmin.value ? t('usr.role.admin') : t('usr.role.member'),
    type: 'label',
  }],
  [
    { label: t('layout.language'), icon: 'i-lucide-languages', children: languageItems.value },
    { label: isExternalAuth() ? t('profile.title') : t('layout.profile'), icon: isExternalAuth() ? 'i-lucide-user' : 'i-lucide-key-round', onSelect: () => { passwordOpen.value = true } },
    ...(mcpEnabled ? [{ label: t('mcp.title'), icon: 'i-lucide-bot', onSelect: () => { mcpOpen.value = true } }] : []),
    { label: t('layout.logout'), icon: 'i-lucide-log-out', onSelect: () => auth.logout() },
  ],
])

const orgItems = computed(() => org.orgs.value.map(o => ({ label: o.name || o.slug, value: o.id })))

function inSection(to: string): boolean {
  return route.path === to || route.path.startsWith(`${to}/`)
}

const navItems = computed<NavigationMenuItem[][]>(() => [
  [
    { label: t('nav.issues'), icon: 'i-lucide-bug', to: '/issues', active: inSection('/issues') },
    { label: t('nav.replays'), icon: 'i-lucide-video', to: '/replays', active: inSection('/replays') },
  ],
  [
    { label: t('nav.settings'), icon: 'i-lucide-folder-cog', to: '/settings', active: inSection('/settings') },
    ...(auth.isAdmin.value ? [{ label: t('nav.users'), icon: 'i-lucide-users', to: '/users', active: inSection('/users') }] : []),
  ],
])
</script>

<template>
  <UDashboardGroup unit="rem">
    <UDashboardSidebar
      collapsible
      :min-size="11"
      :default-size="14"
      :max-size="18"
      class="group"
      :ui="{
        header: 'group-data-[collapsed=true]:px-2',
        body: 'group-data-[collapsed=true]:px-2',
      }"
    >
      <template #header="{ collapsed }">
        <NuxtLink v-if="!collapsed" to="/issues" class="flex items-center gap-2 font-bold text-highlighted">
          <AppLogo class="size-6" />
          zorko
        </NuxtLink>
        <UDashboardSidebarCollapse :class="collapsed ? 'mx-auto' : 'ms-auto'" />
      </template>

      <template #default="{ collapsed }">
        <USelect
          v-if="!collapsed && orgItems.length"
          :model-value="org.currentId.value"
          :items="orgItems"
          icon="i-lucide-building-2"
          class="w-full"
          :aria-label="t('layout.org')"
          @update:model-value="v => org.select(String(v))"
        />
        <UNavigationMenu
          :collapsed="collapsed"
          :items="navItems"
          orientation="vertical"
          :ui="{ linkLabel: 'whitespace-normal' }"
        />
      </template>

      <template #footer="{ collapsed }">
        <UDropdownMenu :items="userMenu" :content="{ side: 'top', align: 'start' }" class="w-full">
          <UButton
            color="neutral"
            variant="ghost"
            class="w-full"
            :class="collapsed ? 'justify-center px-0' : 'justify-start'"
            :label="collapsed ? undefined : (auth.user.value?.name || auth.user.value?.username || '')"
            icon="i-lucide-circle-user"
            :trailing-icon="collapsed ? undefined : 'i-lucide-chevrons-up-down'"
          />
        </UDropdownMenu>
      </template>
    </UDashboardSidebar>

    <slot />

    <ProfileModal v-model:open="passwordOpen" />
    <McpModal v-if="mcpEnabled" v-model:open="mcpOpen" />
  </UDashboardGroup>
</template>
