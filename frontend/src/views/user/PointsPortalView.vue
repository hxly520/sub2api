<template>
  <AppLayout>
    <section
      class="points-workspace-shell -m-4 h-[calc(100dvh-4rem)] overflow-hidden bg-gray-50 dark:bg-dark-950 md:-m-6 lg:-m-8"
      data-testid="points-portal"
    >
      <div class="relative h-full min-h-0 w-full">
        <iframe
          v-if="frameURL"
          ref="frameElement"
          :src="frameURL"
          :title="t('pointsPortal.frameTitle')"
          class="block h-full min-h-0 w-full border-0 bg-white dark:bg-dark-900"
          sandbox="allow-scripts allow-forms allow-same-origin"
          referrerpolicy="no-referrer"
          data-testid="points-portal-frame"
          @error="handleFrameError"
        ></iframe>

        <div
          v-if="launching || frameLoading"
          class="absolute inset-0 overflow-y-auto bg-gray-50 dark:bg-dark-950"
          role="status"
          aria-live="polite"
          data-testid="points-portal-loading"
        >
          <span class="sr-only">{{ t('pointsPortal.opening') }}</span>
          <div class="points-workspace-skeleton mx-auto w-full max-w-[1480px] animate-pulse space-y-3 p-3 motion-reduce:animate-none md:p-4">
            <div class="flex h-12 items-center justify-between">
              <div class="space-y-2">
                <div class="h-2.5 w-16 rounded bg-gray-200 dark:bg-dark-700" />
                <div class="h-5 w-28 rounded bg-gray-300 dark:bg-dark-600" />
              </div>
              <div class="h-9 w-24 rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" />
            </div>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <div v-for="index in 4" :key="index" class="h-[124px] rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
                <div class="h-5 w-24 rounded bg-gray-200 dark:bg-dark-700" />
                <div class="mt-4 h-7 w-36 rounded bg-gray-300 dark:bg-dark-600" />
                <div class="mt-3 h-3 w-28 rounded bg-gray-200 dark:bg-dark-700" />
              </div>
            </div>
            <div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
              <div class="flex h-[70px] items-center justify-between border-b border-gray-200 px-5 dark:border-dark-700">
                <div class="space-y-2"><div class="h-2.5 w-16 rounded bg-gray-200 dark:bg-dark-700" /><div class="h-4 w-32 rounded bg-gray-300 dark:bg-dark-600" /></div>
                <div class="h-9 w-56 rounded-md bg-gray-200 dark:bg-dark-700" />
              </div>
              <div class="h-64 p-5 md:h-80"><div class="h-full rounded-md bg-gray-100 dark:bg-dark-900" /></div>
              <div class="grid grid-cols-3 divide-x divide-gray-200 border-t border-gray-200 dark:divide-dark-700 dark:border-dark-700">
                <div v-for="index in 3" :key="index" class="h-16 p-4"><div class="h-3 w-16 rounded bg-gray-200 dark:bg-dark-700" /><div class="mt-2 h-4 w-24 rounded bg-gray-300 dark:bg-dark-600" /></div>
              </div>
            </div>
            <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
              <div v-for="index in 2" :key="index" class="h-48 rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" />
            </div>
          </div>
        </div>

        <div
          v-else-if="failed"
          class="absolute inset-0 flex items-center justify-center bg-gray-50 px-4 py-12 dark:bg-dark-950"
          data-testid="points-portal-error"
        >
          <div class="w-full max-w-md space-y-5 text-center">
            <Icon name="infoCircle" size="xl" class="mx-auto text-red-500" />
            <p class="font-medium text-gray-900 dark:text-white">
              {{ t('pointsPortal.failed') }}
            </p>
            <div class="flex justify-center gap-3">
              <button
                type="button"
                class="btn btn-primary"
                data-testid="retry-points-portal"
                @click="launch"
              >
                {{ t('pointsPortal.retry') }}
              </button>
              <router-link :to="backPath" class="btn btn-secondary">
                {{ t('pointsPortal.back') }}
              </router-link>
            </div>
          </div>
        </div>
      </div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import '@/styles/points-workspace.css'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { createPointsLaunch } from '@/api/points'
import {
  buildEmbeddedFrameUrl,
  isPointsFrameReadyMessage,
  POINTS_FRAME_THEME_MESSAGE,
} from '@/utils/embedded-url'

const FRAME_LOAD_TIMEOUT_MS = 20_000

const route = useRoute()
const { locale, t } = useI18n()
const launching = ref(true)
const frameLoading = ref(false)
const failed = ref(false)
const frameURL = ref('')
const frameOrigin = ref('')
const frameElement = ref<HTMLIFrameElement | null>(null)
const isAdminPortal = computed(() => route.meta.requiresAdmin === true)
const backPath = computed(() => isAdminPortal.value ? '/admin/settings' : '/dashboard')
let frameLoadTimer: number | null = null
let themeObserver: MutationObserver | null = null

function activeTheme(): 'light' | 'dark' {
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

function syncFrameTheme(): void {
  if (!frameOrigin.value || !frameElement.value?.contentWindow) return
  frameElement.value.contentWindow.postMessage(
    { type: POINTS_FRAME_THEME_MESSAGE, theme: activeTheme() },
    frameOrigin.value,
  )
}

function clearFrameLoadTimer(): void {
  if (frameLoadTimer !== null) {
    window.clearTimeout(frameLoadTimer)
    frameLoadTimer = null
  }
}

function handleFrameReady(): void {
  clearFrameLoadTimer()
  frameLoading.value = false
  failed.value = false
  syncFrameTheme()
}

function handleFrameMessage(event: MessageEvent): void {
  if (!isPointsFrameReadyMessage(event.data, 'user')) return
  if (event.origin !== frameOrigin.value || event.source !== frameElement.value?.contentWindow) return
  handleFrameReady()
}

function handleFrameError(): void {
  clearFrameLoadTimer()
  frameLoading.value = false
  failed.value = true
}

function waitForFrameLoad(): void {
  clearFrameLoadTimer()
  frameLoadTimer = window.setTimeout(handleFrameError, FRAME_LOAD_TIMEOUT_MS)
}

async function launch(): Promise<void> {
  clearFrameLoadTimer()
  launching.value = true
  frameLoading.value = false
  failed.value = false
  frameURL.value = ''
  frameOrigin.value = ''
  try {
    const { launch_url: launchURL } = await createPointsLaunch(
      isAdminPortal.value ? 'admin' : 'user',
      {
        theme: activeTheme(),
        language: locale.value,
      },
    )
    const destination = buildEmbeddedFrameUrl(launchURL)
    frameLoading.value = true
    frameOrigin.value = new URL(destination).origin
    frameURL.value = destination
    waitForFrameLoad()
  } catch {
    failed.value = true
  } finally {
    launching.value = false
  }
}

onMounted(() => {
  window.addEventListener('message', handleFrameMessage)
  themeObserver = new MutationObserver(syncFrameTheme)
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  void launch()
})
onBeforeUnmount(() => {
  clearFrameLoadTimer()
  themeObserver?.disconnect()
  themeObserver = null
  window.removeEventListener('message', handleFrameMessage)
})
</script>
