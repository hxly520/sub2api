<template>
  <div v-if="hasCustomHomeContent" class="min-h-screen bg-white dark:bg-dark-950">
    <iframe
      v-if="customHomeFrameUrl"
      :src="customHomeFrameUrl"
      :title="t('home.customPageTitle')"
      class="h-screen w-full border-0"
      sandbox=""
      referrerpolicy="no-referrer"
    ></iframe>
    <div
      v-else
      class="mx-auto min-h-screen max-w-5xl px-6 py-12 text-gray-700 dark:text-dark-200"
    >
      <div class="safe-home-content" v-html="customHomeHtml"></div>
    </div>
  </div>

  <div v-else class="flex min-h-screen flex-col bg-gray-50 dark:bg-dark-950">
    <header class="border-b border-gray-200 bg-white/95 px-5 dark:border-dark-800 dark:bg-dark-900/95">
      <nav class="mx-auto flex min-h-16 max-w-6xl items-center justify-between gap-4">
        <router-link to="/" class="flex min-w-0 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            :alt="siteName"
            class="h-9 w-9 flex-none rounded-lg object-contain"
          />
          <span class="truncate text-sm font-semibold text-gray-900 dark:text-white">
            {{ siteName }}
          </span>
        </router-link>

        <div class="flex items-center gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <button
            type="button"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-9 items-center rounded-lg bg-gray-900 px-3 text-sm font-medium text-white transition-colors hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex-1 px-5 py-12 sm:py-16">
      <div class="mx-auto max-w-6xl">
        <section class="mx-auto max-w-3xl text-center" aria-labelledby="home-title">
          <span
            class="inline-flex min-h-7 items-center rounded-full border border-primary-200 bg-primary-50 px-3 text-xs font-semibold text-primary-700 dark:border-primary-800 dark:bg-primary-950/40 dark:text-primary-300"
          >
            {{ t('home.consoleLabel') }}
          </span>
          <h1
            id="home-title"
            class="mt-5 text-4xl font-bold leading-tight text-gray-950 dark:text-white sm:text-5xl"
          >
            {{ siteName }}
          </h1>
          <p class="mt-4 text-lg text-gray-600 dark:text-dark-300">
            {{ siteSubtitle }}
          </p>
          <p class="mx-auto mt-3 max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-400">
            {{ t('home.consoleDescription') }}
          </p>
          <div class="mt-8 flex flex-wrap justify-center gap-3">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="btn btn-primary min-h-10 px-5"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.loginConsole') }}
              <Icon name="arrowRight" size="sm" class="ml-2" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="btn btn-secondary min-h-10 px-5"
            >
              <Icon name="book" size="sm" class="mr-2" />
              {{ t('home.viewDocs') }}
            </a>
          </div>
        </section>

        <section
          class="mt-14 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-800 dark:bg-dark-900"
          :aria-label="t('home.capabilitiesTitle')"
        >
          <div
            class="flex flex-col gap-2 border-b border-gray-200 px-5 py-4 sm:flex-row sm:items-center sm:justify-between dark:border-dark-800"
          >
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('home.capabilitiesTitle') }}
            </h2>
            <span class="inline-flex items-center gap-2 text-xs font-medium text-primary-700 dark:text-primary-300">
              <span class="h-2 w-2 rounded-full bg-green-500"></span>
              {{ t('home.entryAvailable') }}
            </span>
          </div>
          <div class="grid md:grid-cols-3">
            <article class="min-w-0 px-5 py-6 md:border-r md:border-gray-200 dark:md:border-dark-800">
              <span class="text-xs font-semibold text-gray-400 dark:text-dark-500">01</span>
              <h3 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.keyManagement') }}
              </h3>
              <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                {{ t('home.features.keyManagementDesc') }}
              </p>
            </article>
            <article
              class="min-w-0 border-t border-gray-200 px-5 py-6 md:border-r md:border-t-0 dark:border-dark-800"
            >
              <span class="text-xs font-semibold text-gray-400 dark:text-dark-500">02</span>
              <h3 class="mt-3 text-base font-semibold text-blue-700 dark:text-blue-300">
                {{ t('home.features.usageRecords') }}
              </h3>
              <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                {{ t('home.features.usageRecordsDesc') }}
              </p>
            </article>
            <article class="min-w-0 border-t border-gray-200 px-5 py-6 md:border-t-0 dark:border-dark-800">
              <span class="text-xs font-semibold text-gray-400 dark:text-dark-500">03</span>
              <h3 class="mt-3 text-base font-semibold text-amber-700 dark:text-amber-300">
                {{ t('home.features.quotaControls') }}
              </h3>
              <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                {{ t('home.features.quotaControlsDesc') }}
              </p>
            </article>
          </div>
        </section>

        <aside
          class="mt-5 flex gap-3 rounded-lg border border-gray-200 bg-white px-4 py-4 dark:border-dark-800 dark:bg-dark-900"
          :aria-label="t('home.securityNoticeTitle')"
        >
          <span
            class="flex h-6 w-6 flex-none items-center justify-center rounded-full bg-primary-50 text-xs font-bold text-primary-700 dark:bg-primary-950/50 dark:text-primary-300"
          >i</span>
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('home.securityNoticeTitle') }}
            </h2>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
              {{ t('home.securityNoticeDescription') }}
            </p>
          </div>
        </aside>
      </div>
    </main>

    <footer class="border-t border-gray-200 bg-white px-5 dark:border-dark-800 dark:bg-dark-900">
      <div
        class="mx-auto flex min-h-[72px] max-w-6xl flex-col items-center justify-center gap-2 py-4 text-center text-xs text-gray-500 sm:flex-row sm:justify-between sm:text-left dark:text-dark-400"
      >
        <span>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</span>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="hover:text-gray-800 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
          <router-link to="/login" class="hover:text-gray-800 dark:hover:text-white">
            {{ t('home.login') }}
          </router-link>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import {
  sanitizeHomeContentFrameUrl,
  sanitizeHomeContentHtml
} from '@/utils/homeContent'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || t('home.defaultSubtitle')
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const rawHomeContent = computed(() => appStore.cachedPublicSettings?.home_content?.trim() || '')
const customHomeFrameUrl = computed(() => sanitizeHomeContentFrameUrl(rawHomeContent.value))
const customHomeHtml = computed(() =>
  customHomeFrameUrl.value ? '' : sanitizeHomeContentHtml(rawHomeContent.value)
)
const hasCustomHomeContent = computed(
  () => Boolean(customHomeFrameUrl.value || customHomeHtml.value.trim())
)

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() =>
  authStore.isAdmin ? '/admin/dashboard' : '/dashboard'
)
const currentYear = new Date().getFullYear()

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.safe-home-content :deep(h1),
.safe-home-content :deep(h2),
.safe-home-content :deep(h3) {
  margin: 1.4em 0 0.55em;
  color: inherit;
  font-weight: 700;
  line-height: 1.3;
}

.safe-home-content :deep(p),
.safe-home-content :deep(ul),
.safe-home-content :deep(ol),
.safe-home-content :deep(pre),
.safe-home-content :deep(blockquote) {
  margin: 0.85em 0;
}

.safe-home-content :deep(a) {
  color: #0f766e;
  text-decoration: underline;
}

.safe-home-content :deep(pre) {
  overflow-x: auto;
  border: 1px solid #dfe6eb;
  border-radius: 8px;
  padding: 1rem;
  background: #f7f9fb;
}
</style>
