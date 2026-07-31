<template>
  <component :is="embedded ? 'div' : AppLayout">
    <div class="points-workspace-shell">
      <div class="mx-auto max-w-6xl space-y-6">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('pointsSettings.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('pointsSettings.subtitle') }}
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="loading || launching"
            @click="loadStatus"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('pointsSettings.refresh') }}
          </button>
          <button
            type="button"
            class="btn btn-primary inline-flex items-center gap-2"
            data-testid="open-points-console"
            :disabled="!status?.configured || launching"
            @click="openConsole"
          >
            <Icon name="externalLink" size="sm" />
            {{ launching ? t('pointsSettings.opening') : t('pointsSettings.openConsole') }}
          </button>
        </div>
      </div>

      <div v-if="loading && !status" class="points-workspace-skeleton animate-pulse space-y-4 motion-reduce:animate-none" role="status">
        <span class="sr-only">{{ t('common.loading') }}</span>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
          <div v-for="index in 3" :key="index" class="card flex min-h-24 items-center gap-3 px-4 py-3">
            <div class="h-10 w-10 rounded-lg bg-gray-200 dark:bg-dark-700" />
            <div class="flex-1 space-y-2"><div class="h-3 w-20 rounded bg-gray-200 dark:bg-dark-700" /><div class="h-4 w-28 rounded bg-gray-300 dark:bg-dark-600" /></div>
          </div>
        </div>
        <div class="card h-40 p-6"><div class="h-5 w-32 rounded bg-gray-300 dark:bg-dark-600" /><div class="mt-6 grid grid-cols-1 gap-4 md:grid-cols-2"><div v-for="index in 4" :key="index" class="h-10 rounded bg-gray-100 dark:bg-dark-700" /></div></div>
      </div>

      <div
        v-else-if="loadFailed"
        class="card flex flex-col items-center gap-4 p-8 text-center"
        data-testid="points-status-error"
      >
        <Icon name="exclamationTriangle" size="xl" class="text-amber-500" />
        <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('pointsSettings.loadFailed') }}
        </p>
        <button type="button" class="btn btn-secondary" @click="loadStatus">
          {{ t('pointsSettings.retry') }}
        </button>
      </div>

      <template v-else-if="status">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
          <div
            v-for="item in overviewItems"
            :key="item.key"
            class="card flex min-h-24 items-center gap-3 px-4 py-3"
          >
            <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg" :class="item.iconClass">
              <Icon :name="item.icon" size="sm" />
            </div>
            <div class="min-w-0">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
              <p class="mt-1 truncate text-base font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
            </div>
          </div>
        </div>

        <div
          v-if="!status.configured"
          class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
          data-testid="points-not-configured"
        >
          {{ t('pointsSettings.notConfigured') }}
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('pointsSettings.bridgeTitle') }}
            </h2>
          </div>
          <dl class="grid grid-cols-1 divide-y divide-gray-100 px-6 dark:divide-dark-700 md:grid-cols-2 md:divide-x md:divide-y-0">
            <div
              v-for="(item, index) in bridgeDetails"
              :key="item.key"
              class="min-w-0 py-4 md:px-5"
              :class="index % 2 === 0 ? 'md:pl-0' : 'md:pr-0'"
            >
              <dt class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
              <dd class="mt-1 break-all text-sm font-medium text-gray-900 dark:text-white">{{ item.value }}</dd>
            </div>
          </dl>
        </div>

        <div v-if="launchFailed" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200" data-testid="points-console-error" role="alert">
          {{ t('pointsSettings.launchFailed') }}
        </div>
      </template>
      </div>
      <TotpStepUpDialog :controller="pointsStepUp" />
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import '@/styles/points-workspace.css'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import {
  createPointsLaunch,
  getPointsBridgeStatus,
  type PointsBridgeStatus,
} from '@/api/points'

const { locale, t } = useI18n()
withDefaults(defineProps<{ embedded?: boolean }>(), {
  embedded: false,
})
const status = ref<PointsBridgeStatus | null>(null)
const loading = ref(false)
const launching = ref(false)
const loadFailed = ref(false)
const launchFailed = ref(false)
const pointsStepUp = useStepUp()

const yesNo = (value: boolean) => value ? t('pointsSettings.ready') : t('pointsSettings.missing')
const enabledState = (value: boolean) => value ? t('pointsSettings.enabled') : t('pointsSettings.disabled')

const overviewItems = computed(() => {
  if (!status.value) return []
  return [
    {
      key: 'bridge',
      label: t('pointsSettings.bridgeStatus'),
      value: status.value.configured ? t('pointsSettings.ready') : t('pointsSettings.missing'),
      icon: 'server' as const,
      iconClass: status.value.configured
        ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
        : 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
    },
    {
      key: 'user-entry',
      label: t('pointsSettings.userEntry'),
      value: enabledState(status.value.active),
      icon: 'user' as const,
      iconClass: status.value.active
        ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300',
    },
    {
      key: 'policy',
      label: t('pointsSettings.policyConsole'),
      value: status.value.configured ? t('pointsSettings.available') : t('pointsSettings.unavailable'),
      icon: 'gift' as const,
      iconClass: status.value.configured
        ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300',
    },
  ]
})

const bridgeDetails = computed(() => {
  if (!status.value) return []
  return [
    { key: 'public-url', label: t('pointsSettings.publicUrl'), value: status.value.public_url || '-' },
    { key: 'menu-label', label: t('pointsSettings.menuLabel'), value: status.value.menu_label || '-' },
    { key: 'launch-key', label: t('pointsSettings.launchKey'), value: `${status.value.launch_key_id || '-'} · ${yesNo(status.value.launch_secret_configured)}` },
    { key: 'credit-key', label: t('pointsSettings.creditKey'), value: `${status.value.credit_key_id || '-'} · ${yesNo(status.value.credit_secret_configured)}` },
    { key: 'ticket-ttl', label: t('pointsSettings.ticketTtl'), value: `${status.value.launch_ttl_seconds || 0} s` },
    { key: 'clock-skew', label: t('pointsSettings.clockSkew'), value: `${status.value.clock_skew_seconds || 0} s` },
  ]
})

async function loadStatus(): Promise<void> {
  loading.value = true
  loadFailed.value = false
  try {
    status.value = await getPointsBridgeStatus()
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}

function prepareConsolePopup(popup: Window): void {
  // The popup is still same-origin about:blank here, so isolate it and set a
  // no-referrer policy before it is navigated to the points service.
  popup.opener = null
  const popupDocument = popup.document
  const referrerMeta = popupDocument.createElement('meta')
  referrerMeta.name = 'referrer'
  referrerMeta.content = 'no-referrer'
  popupDocument.head.appendChild(referrerMeta)
  popupDocument.title = t('pointsSettings.consoleTitle')
  popupDocument.body.textContent = t('pointsSettings.consoleLoading')
}

function closeConsolePopup(popup: Window): void {
  try {
    popup.close()
  } catch {
    // A popup can be closed by the user while the launch request is pending.
  }
}

async function openConsole(): Promise<void> {
  if (!status.value?.configured || launching.value) return
  launchFailed.value = false

  // Open while the click still has user activation; opening after the async
  // step-up/API call is commonly rejected by popup blockers.
  let popup: Window | null = null
  try {
    popup = window.open('about:blank', '_blank')
  } catch {
    launchFailed.value = true
    return
  }
  if (!popup) {
    launchFailed.value = true
    return
  }

  launching.value = true
  try {
    prepareConsolePopup(popup)
    const { launch_url: launchURL } = await pointsStepUp.run(() =>
      createPointsLaunch('admin', {
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light',
        language: locale.value,
      }),
    )
    if (popup.closed) throw new Error('points console popup was closed')
    popup.location.replace(launchURL)
  } catch (error) {
    closeConsolePopup(popup)
    if (isStepUpCancelled(error)) {
      return
    }
    launchFailed.value = true
  } finally {
    launching.value = false
  }
}

onMounted(() => {
  void loadStatus()
})
</script>
