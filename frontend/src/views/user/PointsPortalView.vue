<template>
  <AppLayout>
    <div class="flex min-h-[60vh] items-center justify-center px-4 py-12">
      <div class="w-full max-w-md text-center">
        <div v-if="launching" class="flex flex-col items-center gap-4" role="status">
          <div class="h-9 w-9 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
          <p class="text-sm font-medium text-gray-600 dark:text-dark-300">
            {{ t('pointsPortal.opening') }}
          </p>
        </div>

        <div v-else class="space-y-5">
          <Icon name="infoCircle" size="xl" class="mx-auto text-red-500" />
          <p class="font-medium text-gray-900 dark:text-white">
            {{ t('pointsPortal.failed') }}
          </p>
          <div class="flex justify-center gap-3">
            <button type="button" class="btn btn-primary" @click="launch">
              {{ t('pointsPortal.retry') }}
            </button>
            <router-link :to="backPath" class="btn btn-secondary">
              {{ t('pointsPortal.back') }}
            </router-link>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { createPointsLaunch } from '@/api/points'

const route = useRoute()
const { locale, t } = useI18n()
const launching = ref(true)
const isAdminPortal = computed(() => route.meta.requiresAdmin === true)
const backPath = computed(() => isAdminPortal.value ? '/admin/settings' : '/dashboard')

async function launch(): Promise<void> {
  launching.value = true
  try {
    const { launch_url: launchURL } = await createPointsLaunch(
      isAdminPortal.value ? 'admin' : 'user',
      {
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light',
        language: locale.value,
      },
    )
    const destination = new URL(launchURL, window.location.origin)
    if (destination.protocol !== 'https:' && destination.protocol !== 'http:') {
      throw new Error('invalid points launch URL')
    }
    window.location.replace(destination.toString())
  } catch {
    launching.value = false
  }
}

onMounted(launch)
</script>
