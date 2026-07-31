<template>
  <AppLayout class="points-workspace-shell">
    <section
      class="-m-4 h-[calc(100dvh-4rem)] overflow-hidden bg-gray-50 dark:bg-dark-950 md:-m-6 lg:-m-8"
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
          class="absolute inset-0 flex items-center justify-center bg-gray-50 dark:bg-dark-950"
          role="status"
          data-testid="points-portal-loading"
        >
          <div class="flex flex-col items-center gap-4 px-6 text-center">
            <div class="h-9 w-9 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
            <p class="text-sm font-medium text-gray-600 dark:text-dark-300">
              {{ t('pointsPortal.opening') }}
            </p>
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
import { buildEmbeddedFrameUrl, isPointsFrameReadyMessage } from '@/utils/embedded-url'

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
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light',
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
  void launch()
})
onBeforeUnmount(() => {
  clearFrameLoadTimer()
  window.removeEventListener('message', handleFrameMessage)
})
</script>
