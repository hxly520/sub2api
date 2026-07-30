<template>
  <component :is="embedded ? 'div' : AppLayout">
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

      <div v-if="loading && !status" class="flex items-center justify-center py-16" role="status">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
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

        <div v-if="launchFailed && !consoleURL" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200">
          {{ t('pointsSettings.launchFailed') }}
        </div>

        <div
          v-if="consoleURL"
          class="overflow-hidden border-t border-gray-200 dark:border-dark-700"
          data-testid="points-console-panel"
        >
          <div class="flex min-h-14 items-center border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-6">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('pointsSettings.consoleTitle') }}
            </h2>
          </div>
          <div class="relative h-[calc(100dvh-12rem)] min-h-[32rem] overflow-hidden">
            <iframe
              ref="consoleFrameElement"
              :src="consoleURL"
              :title="t('pointsSettings.consoleFrameTitle')"
              class="block h-full min-h-0 w-full border-0 bg-white dark:bg-dark-900"
              sandbox="allow-scripts allow-forms allow-same-origin"
              referrerpolicy="no-referrer"
              data-testid="points-console-frame"
              @error="handleConsoleError"
            ></iframe>
            <div
              v-if="consoleLoading"
              class="absolute inset-0 flex items-center justify-center bg-white/95 dark:bg-dark-900/95"
              role="status"
              data-testid="points-console-loading"
            >
              <div class="flex flex-col items-center gap-3 text-center">
                <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
                <p class="text-sm font-medium text-gray-600 dark:text-dark-300">
                  {{ t('pointsSettings.consoleLoading') }}
                </p>
              </div>
            </div>
            <div
              v-else-if="launchFailed"
              class="absolute inset-0 flex items-center justify-center bg-white px-6 text-center dark:bg-dark-900"
              data-testid="points-console-error"
            >
              <div class="max-w-sm space-y-4">
                <Icon name="exclamationTriangle" size="xl" class="mx-auto text-amber-500" />
                <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
                  {{ t('pointsSettings.launchFailed') }}
                </p>
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="launching"
                  data-testid="retry-points-console"
                  @click="openConsole"
                >
                  {{ t('pointsSettings.retry') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
    <TotpStepUpDialog :controller="pointsStepUp" />
  </component>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import {
  createPointsLaunch,
  getPointsBridgeStatus,
  type PointsBridgeStatus,
} from '@/api/points'
import { buildEmbeddedFrameUrl, isPointsFrameReadyMessage } from '@/utils/embedded-url'

const FRAME_LOAD_TIMEOUT_MS = 20_000

const { locale, t } = useI18n()
withDefaults(defineProps<{ embedded?: boolean }>(), {
  embedded: false,
})
const status = ref<PointsBridgeStatus | null>(null)
const loading = ref(false)
const launching = ref(false)
const loadFailed = ref(false)
const launchFailed = ref(false)
const consoleLoading = ref(false)
const consoleURL = ref('')
const consoleOrigin = ref('')
const consoleFrameElement = ref<HTMLIFrameElement | null>(null)
const pointsStepUp = useStepUp()
let frameLoadTimer: number | null = null

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
        ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300'
        : 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
    },
    {
      key: 'user-entry',
      label: t('pointsSettings.userEntry'),
      value: enabledState(status.value.active),
      icon: 'user' as const,
      iconClass: status.value.active
        ? 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300'
        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300',
    },
    {
      key: 'policy',
      label: t('pointsSettings.policyConsole'),
      value: status.value.configured ? t('pointsSettings.available') : t('pointsSettings.unavailable'),
      icon: 'gift' as const,
      iconClass: status.value.configured
        ? 'bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300'
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

async function openConsole(): Promise<void> {
  if (!status.value?.configured || launching.value) return
  launching.value = true
  try {
    const { launch_url: launchURL } = await pointsStepUp.run(() =>
      createPointsLaunch('admin', {
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light',
        language: locale.value,
      }),
    )
    const embeddedURL = buildEmbeddedFrameUrl(launchURL)
    launchFailed.value = false
    consoleLoading.value = true
    consoleOrigin.value = new URL(embeddedURL).origin
    consoleURL.value = embeddedURL
    waitForConsoleLoad()
  } catch (error) {
    if (isStepUpCancelled(error)) {
      return
    }
    launchFailed.value = true
    consoleLoading.value = false
  } finally {
    launching.value = false
  }
}

function clearFrameLoadTimer(): void {
  if (frameLoadTimer !== null) {
    window.clearTimeout(frameLoadTimer)
    frameLoadTimer = null
  }
}

function handleConsoleReady(): void {
  clearFrameLoadTimer()
  consoleLoading.value = false
  launchFailed.value = false
}

function handleConsoleMessage(event: MessageEvent): void {
  if (!isPointsFrameReadyMessage(event.data, 'admin')) return
  if (event.origin !== consoleOrigin.value || event.source !== consoleFrameElement.value?.contentWindow) return
  handleConsoleReady()
}

function handleConsoleError(): void {
  clearFrameLoadTimer()
  consoleLoading.value = false
  launchFailed.value = true
}

function waitForConsoleLoad(): void {
  clearFrameLoadTimer()
  frameLoadTimer = window.setTimeout(handleConsoleError, FRAME_LOAD_TIMEOUT_MS)
}

onMounted(() => {
  window.addEventListener('message', handleConsoleMessage)
  void loadStatus()
})
onBeforeUnmount(() => {
  clearFrameLoadTimer()
  window.removeEventListener('message', handleConsoleMessage)
})
</script>
