<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <!-- iframe mode -->
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

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" :alt="siteName" class="h-9 w-9 shrink-0 rounded-lg object-contain" />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <button
            type="button"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img :src="siteLogo || '/logo.svg'" :alt="siteName" class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain" />
        <h1 class="break-words text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap break-words text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <div
    v-else
    data-testid="default-home"
    class="terminal-container min-h-screen overflow-x-hidden bg-white text-gray-950 dark:bg-dark-950 dark:text-white"
  >
    <header
      data-testid="home-header"
      class="sticky top-0 z-50 border-b border-gray-200/80 bg-white/90 backdrop-blur-md dark:border-dark-800 dark:bg-dark-950/90"
    >
      <nav
        class="mx-auto flex h-16 max-w-7xl items-center justify-between gap-3 px-4 sm:px-6 lg:px-8"
        :aria-label="t('home.nav.primary')"
      >
        <a href="#top" class="flex min-w-0 items-center gap-2.5" @click="closeMobileNav">
          <img
            :src="siteLogo || '/logo.svg'"
            :alt="siteName"
            class="h-9 w-9 flex-none rounded-md border border-gray-200 bg-white object-contain p-0.5 dark:border-dark-700 dark:bg-dark-900"
          />
          <span class="max-w-32 truncate text-sm font-semibold text-gray-900 sm:max-w-56 sm:text-base dark:text-white">
            {{ siteName }}
          </span>
        </a>

        <div
          data-testid="home-desktop-nav"
          class="hidden items-center gap-1 lg:flex"
        >
          <a
            v-for="item in sectionNavItems"
            :key="item.href"
            :href="item.href"
            class="rounded-md px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-950 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            {{ item.label }}
          </a>
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-md px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-950 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            {{ t('home.nav.docs') }}
          </a>
        </div>

        <div class="flex flex-none items-center gap-1.5 sm:gap-2">
          <div class="hidden sm:block">
            <LocaleSwitcher />
          </div>
          <button
            type="button"
            class="inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>

          <template v-if="isAuthenticated">
            <router-link
              :to="dashboardPath"
              class="inline-flex h-9 items-center justify-center rounded-md bg-blue-600 px-3 text-sm font-semibold text-white transition-colors hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 dark:bg-blue-500 dark:text-white dark:hover:bg-blue-400 sm:px-4"
            >
              {{ t('home.dashboard') }}
            </router-link>
          </template>
          <template v-else>
            <router-link
              to="/login"
              data-testid="home-login-link"
              class="hidden h-9 items-center justify-center rounded-md px-3 text-sm font-semibold text-gray-700 transition-colors hover:bg-gray-100 hover:text-gray-950 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:text-dark-200 dark:hover:bg-dark-800 dark:hover:text-white md:inline-flex"
            >
              {{ t('home.login') }}
            </router-link>
            <router-link
              v-if="registrationEnabled"
              to="/register"
              data-testid="home-register-link"
              class="inline-flex h-9 items-center justify-center rounded-md bg-blue-600 px-3 text-sm font-semibold text-white transition-colors hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 dark:bg-blue-500 dark:text-white dark:hover:bg-blue-400 sm:px-4"
            >
              {{ t('home.register') }}
            </router-link>
          </template>

          <button
            type="button"
            data-testid="home-mobile-menu-button"
            class="inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-950 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white lg:hidden"
            :aria-expanded="mobileNavOpen"
            :aria-label="t('home.nav.toggle')"
            @click="mobileNavOpen = !mobileNavOpen"
          >
            <Icon :name="mobileNavOpen ? 'x' : 'menu'" size="sm" />
          </button>
        </div>
      </nav>

      <div
        v-if="mobileNavOpen"
        data-testid="home-mobile-menu"
        class="border-t border-gray-200 bg-white px-4 py-3 dark:border-dark-800 dark:bg-dark-950 lg:hidden"
      >
        <div class="mx-auto grid max-w-7xl grid-cols-2 gap-1 sm:grid-cols-3">
          <a
            v-for="item in sectionNavItems"
            :key="`mobile-${item.href}`"
            :href="item.href"
            class="rounded-md px-3 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-dark-200 dark:hover:bg-dark-800"
            @click="closeMobileNav"
          >
            {{ item.label }}
          </a>
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-md px-3 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-dark-200 dark:hover:bg-dark-800"
            @click="closeMobileNav"
          >
            {{ t('home.nav.docs') }}
          </a>
          <router-link
            v-if="!isAuthenticated"
            to="/login"
            class="rounded-md px-3 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-dark-200 dark:hover:bg-dark-800 md:hidden"
            @click="closeMobileNav"
          >
            {{ t('home.login') }}
          </router-link>
          <div class="flex items-center px-3 py-1 sm:hidden">
            <LocaleSwitcher />
          </div>
        </div>
      </div>
    </header>

    <main>
      <section
        id="top"
        data-testid="home-hero"
        class="home-hero relative isolate flex min-h-[calc(100svh-7rem)] items-center overflow-hidden border-b border-gray-200 bg-[#f5f8fc] dark:border-dark-800 dark:bg-dark-950"
        aria-labelledby="home-title"
      >
        <div class="home-scene pointer-events-none absolute inset-0 -z-10" aria-hidden="true">
          <span class="home-scene-frame"></span>
          <span class="home-scene-line home-scene-line-one"></span>
          <span class="home-scene-line home-scene-line-two"></span>
        </div>

        <div class="mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 sm:py-10 lg:px-8">
          <div class="grid items-center gap-8 lg:grid-cols-[minmax(0,0.92fr)_minmax(0,1.08fr)] lg:gap-4">
            <div class="home-hero-copy max-w-2xl">
              <p class="flex items-center gap-2.5 text-xs font-semibold text-blue-700 dark:text-blue-300">
                <span class="h-px w-7 bg-blue-500" aria-hidden="true"></span>
              {{ t('home.hero.eyebrow') }}
              </p>
              <h1
                id="home-title"
                class="mt-5 break-words text-4xl font-bold leading-tight text-gray-950 sm:text-5xl dark:text-white"
              >
                {{ siteName }}
              </h1>
              <p class="home-hero-display-title mt-3 break-words text-4xl font-bold leading-tight text-gray-950 sm:text-5xl dark:text-white">
                <span class="block">{{ t('home.hero.titlePrimary') }}</span>
                <span class="block text-blue-600 dark:text-blue-400">{{ t('home.hero.titleSecondary') }}</span>
              </p>
              <p class="mt-5 max-w-xl text-lg font-semibold leading-8 text-gray-700 dark:text-dark-100">
                {{ siteSubtitle }}
              </p>
              <p class="mt-2 max-w-xl text-sm leading-7 text-gray-600 sm:text-base dark:text-dark-300">
                {{ t('home.hero.description') }}
              </p>

              <div class="mt-7 flex flex-col items-stretch gap-3 sm:flex-row sm:items-center">
                <router-link
                  :to="primaryEntryPath"
                  data-testid="home-primary-entry"
                  class="inline-flex min-h-11 items-center justify-center rounded-md bg-blue-600 px-5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 dark:bg-blue-500 dark:hover:bg-blue-400"
                >
                  {{ primaryEntryLabel }}
                  <Icon name="arrowRight" size="sm" class="ml-2" />
                </router-link>
                <a
                  href="#overview"
                  class="inline-flex min-h-11 items-center justify-center rounded-md border border-gray-300 bg-white px-5 text-sm font-semibold text-gray-800 transition-colors hover:border-blue-300 hover:text-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:border-dark-600 dark:bg-dark-900 dark:text-white dark:hover:border-blue-500 dark:hover:text-blue-300"
                >
                  {{ t('home.hero.secondaryAction') }}
                </a>
              </div>

              <ul class="mt-6 flex flex-wrap gap-x-5 gap-y-2 text-xs font-medium text-gray-600 dark:text-dark-300">
                <li v-for="item in heroAssurances" :key="item" class="inline-flex items-center gap-1.5">
                  <Icon name="checkCircle" size="xs" class="text-blue-600 dark:text-blue-400" />
                  {{ item }}
                </li>
              </ul>
            </div>

            <figure class="home-data-visual relative min-w-0" aria-labelledby="home-visual-caption">
              <img
                src="/home-data-visual.png"
                :alt="t('home.hero.visualAlt')"
                class="home-data-visual-image block h-auto w-full object-contain"
              />
              <figcaption id="home-visual-caption" class="sr-only">
                {{ t('home.hero.visualAlt') }}
              </figcaption>
            </figure>
          </div>

          <div
            class="home-service-flow mt-8 hidden grid-cols-4 border-y border-gray-200 bg-white/75 md:grid dark:border-dark-700 dark:bg-dark-900/70"
            aria-hidden="true"
          >
            <div
              v-for="(item, index) in serviceFlowItems"
              :key="item.label"
              class="home-flow-item relative flex min-h-24 flex-col items-center justify-center gap-2 px-3 py-4 text-center"
            >
              <span
                :class="[
                  'inline-flex h-9 w-9 items-center justify-center rounded-md',
                  item.iconClass,
                ]"
              >
                <Icon :name="item.icon" size="sm" />
              </span>
              <span class="text-xs font-semibold text-gray-700 dark:text-dark-200">{{ item.label }}</span>
              <span v-if="index < serviceFlowItems.length - 1" class="home-flow-pulse"></span>
            </div>
          </div>
        </div>
      </section>

      <section
        id="overview"
        data-testid="home-overview"
        class="scroll-mt-20 border-b border-gray-200 bg-white py-16 sm:py-20 dark:border-dark-800 dark:bg-dark-950"
        aria-labelledby="overview-title"
      >
        <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div class="max-w-3xl">
            <p class="flex items-center gap-2 text-xs font-semibold text-blue-700 dark:text-blue-300">
              <span class="h-px w-6 bg-blue-500" aria-hidden="true"></span>
              {{ t('home.overview.eyebrow') }}
            </p>
            <h2 id="overview-title" class="mt-3 text-3xl font-bold leading-tight text-gray-950 sm:text-4xl dark:text-white">
              {{ t('home.overview.title') }}
            </h2>
            <p class="mt-4 text-base leading-7 text-gray-600 dark:text-dark-300">
              {{ t('home.overview.description') }}
            </p>
          </div>

          <div class="home-overview-grid mt-10 grid border-y border-gray-200 sm:grid-cols-2 lg:grid-cols-4 dark:border-dark-800">
            <article
              v-for="item in overviewItems"
              :key="item.title"
              class="home-overview-item px-1 py-7 sm:px-6"
            >
              <span :class="['inline-flex h-10 w-10 items-center justify-center rounded-md', item.iconClass]">
                <Icon :name="item.icon" size="md" />
              </span>
              <h3 class="mt-5 text-base font-semibold text-gray-950 dark:text-white">{{ item.title }}</h3>
              <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ item.description }}</p>
            </article>
          </div>

          <div class="mt-10">
            <p class="text-xs font-semibold text-gray-500 dark:text-dark-400">
              {{ t('home.overview.catalogLabel') }}
            </p>
            <div class="home-capability-grid mt-4 flex flex-wrap gap-px border-y border-gray-200 bg-gray-200 dark:border-dark-700 dark:bg-dark-700">
              <div
                v-for="item in capabilityItems"
                :key="item.label"
                class="home-capability-item flex min-h-20 items-center gap-3 px-4 py-4"
              >
                <Icon :name="item.icon" size="sm" :class="item.iconClass" />
                <span class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ item.label }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section
        id="reliability"
        data-testid="home-reliability"
        class="scroll-mt-20 border-b border-gray-900 bg-[#111318] py-16 text-white sm:py-20"
        aria-labelledby="reliability-title"
      >
        <div class="mx-auto grid max-w-7xl gap-10 px-4 sm:px-6 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] lg:items-start lg:px-8">
          <div>
            <p class="flex items-center gap-2 text-xs font-semibold text-blue-300">
              <span class="h-px w-6 bg-blue-400" aria-hidden="true"></span>
              {{ t('home.reliability.eyebrow') }}
            </p>
            <h2 id="reliability-title" class="mt-3 text-3xl font-bold leading-tight sm:text-4xl">
              {{ t('home.reliability.title') }}
            </h2>
            <p class="mt-4 max-w-xl text-base leading-7 text-gray-300">
              {{ t('home.reliability.description') }}
            </p>
            <a
              href="#guide"
              class="mt-7 inline-flex min-h-10 items-center rounded-md border border-gray-600 px-4 text-sm font-semibold text-white transition-colors hover:border-blue-400 hover:text-blue-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-400/60"
            >
              {{ t('home.reliability.action') }}
              <Icon name="arrowRight" size="sm" class="ml-2" />
            </a>
          </div>

          <div class="border-t border-gray-700">
            <article
              v-for="item in reliabilityItems"
              :key="item.title"
              class="grid gap-3 border-b border-gray-700 py-6 sm:grid-cols-[auto_minmax(0,1fr)] sm:gap-5"
            >
              <span class="inline-flex h-10 w-10 items-center justify-center rounded-md border border-blue-400/30 bg-blue-500/10 text-blue-300">
                <Icon :name="item.icon" size="md" />
              </span>
              <div>
                <h3 class="text-base font-semibold">{{ item.title }}</h3>
                <p class="mt-2 text-sm leading-6 text-gray-400">{{ item.description }}</p>
              </div>
            </article>
          </div>
        </div>
      </section>

      <section
        id="guide"
        data-testid="home-guide"
        class="scroll-mt-20 border-b border-gray-200 bg-[#f7f9fc] py-16 sm:py-20 dark:border-dark-800 dark:bg-dark-900"
        aria-labelledby="guide-title"
      >
        <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div class="mx-auto max-w-3xl text-center">
            <p class="text-xs font-semibold text-blue-700 dark:text-blue-300">{{ t('home.guide.eyebrow') }}</p>
            <h2 id="guide-title" class="mt-3 text-3xl font-bold leading-tight text-gray-950 sm:text-4xl dark:text-white">
              {{ t('home.guide.title') }}
            </h2>
            <p class="mt-4 text-base leading-7 text-gray-600 dark:text-dark-300">{{ t('home.guide.description') }}</p>
          </div>

          <ol class="mt-12 grid gap-8 md:grid-cols-3 md:gap-0">
            <li
              v-for="(item, index) in guideItems"
              :key="item.title"
              class="relative border-t border-gray-300 pt-7 md:border-l md:border-t-0 md:px-8 md:pt-0 first:md:border-l-0 dark:border-dark-600"
            >
              <span class="inline-flex h-8 min-w-8 items-center justify-center rounded-md bg-blue-100 px-2 text-xs font-bold text-blue-800 dark:bg-blue-900/40 dark:text-blue-200">
                {{ String(index + 1).padStart(2, '0') }}
              </span>
              <h3 class="mt-5 text-lg font-semibold text-gray-950 dark:text-white">{{ item.title }}</h3>
              <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ item.description }}</p>
            </li>
          </ol>

          <div class="mt-10 flex justify-center">
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex min-h-10 items-center rounded-md border border-gray-300 bg-white px-4 text-sm font-semibold text-gray-800 transition-colors hover:border-blue-300 hover:text-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:border-dark-600 dark:bg-dark-950 dark:text-white dark:hover:border-blue-500 dark:hover:text-blue-300"
            >
              <Icon name="book" size="sm" class="mr-2" />
              {{ t('home.guide.docsAction') }}
            </a>
          </div>
        </div>
      </section>

      <section
        id="faq"
        data-testid="home-faq"
        class="scroll-mt-20 border-b border-gray-200 bg-white py-16 sm:py-20 dark:border-dark-800 dark:bg-dark-950"
        aria-labelledby="faq-title"
      >
        <div class="mx-auto grid max-w-7xl gap-10 px-4 sm:px-6 lg:grid-cols-[minmax(0,0.7fr)_minmax(0,1.3fr)] lg:px-8">
          <div>
            <p class="flex items-center gap-2 text-xs font-semibold text-blue-700 dark:text-blue-300">
              <span class="h-px w-6 bg-blue-500" aria-hidden="true"></span>
              {{ t('home.faq.eyebrow') }}
            </p>
            <h2 id="faq-title" class="mt-3 text-3xl font-bold leading-tight text-gray-950 sm:text-4xl dark:text-white">
              {{ t('home.faq.title') }}
            </h2>
            <p class="mt-4 max-w-md text-base leading-7 text-gray-600 dark:text-dark-300">{{ t('home.faq.description') }}</p>
          </div>

          <div class="border-t border-gray-200 dark:border-dark-700">
            <details
              v-for="item in faqItems"
              :key="item.question"
              class="home-faq-item border-b border-gray-200 dark:border-dark-700"
            >
              <summary
                class="flex cursor-pointer list-none items-center justify-between gap-4 py-5 text-left text-sm font-semibold text-gray-950 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:text-white sm:text-base"
              >
                <span>{{ item.question }}</span>
                <Icon name="chevronDown" size="sm" class="faq-chevron flex-none text-gray-400 transition-transform" />
              </summary>
              <p class="max-w-3xl pb-5 pr-8 text-sm leading-7 text-gray-600 dark:text-dark-300">
                {{ item.answer }}
              </p>
            </details>
          </div>
        </div>
      </section>

      <section class="bg-blue-600 py-14 text-white dark:bg-blue-700" aria-labelledby="home-cta-title">
        <div class="mx-auto flex max-w-7xl flex-col gap-7 px-4 sm:px-6 lg:flex-row lg:items-center lg:justify-between lg:px-8">
          <div class="max-w-3xl">
            <p class="text-xs font-semibold text-blue-100">{{ t('home.cta.eyebrow') }}</p>
            <h2 id="home-cta-title" class="mt-2 text-3xl font-bold leading-tight">{{ t('home.cta.title') }}</h2>
            <p class="mt-3 text-sm leading-6 text-blue-50/90 sm:text-base">{{ t('home.cta.description') }}</p>
          </div>
          <div class="flex flex-col gap-3 sm:flex-row lg:flex-none">
            <router-link
              :to="primaryEntryPath"
              class="inline-flex min-h-11 items-center justify-center rounded-md bg-white px-5 text-sm font-semibold text-blue-800 transition-colors hover:bg-blue-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/70"
            >
              {{ primaryEntryLabel }}
              <Icon name="arrowRight" size="sm" class="ml-2" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex min-h-11 items-center justify-center rounded-md border border-white/50 px-5 text-sm font-semibold text-white transition-colors hover:border-white hover:bg-white/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/70"
            >
              {{ t('home.cta.docsAction') }}
            </a>
          </div>
        </div>
      </section>
    </main>

    <footer class="border-t border-gray-200 bg-gray-50 dark:border-dark-800 dark:bg-dark-950">
      <div class="mx-auto flex max-w-7xl flex-col gap-5 px-4 py-8 sm:px-6 md:flex-row md:items-center md:justify-between lg:px-8">
        <div class="flex min-w-0 items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" alt="" class="h-8 w-8 flex-none rounded-md object-contain" />
          <div class="min-w-0">
            <p class="truncate text-sm font-semibold text-gray-950 dark:text-white">{{ siteName }}</p>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('home.footer.tagline') }}</p>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-gray-500 dark:text-dark-400">
          <a href="#overview" class="hover:text-gray-900 dark:hover:text-white">{{ t('home.nav.overview') }}</a>
          <a href="#guide" class="hover:text-gray-900 dark:hover:text-white">{{ t('home.nav.guide') }}</a>
          <a href="#faq" class="hover:text-gray-900 dark:hover:text-white">{{ t('home.nav.faq') }}</a>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="hover:text-gray-900 dark:hover:text-white">
            {{ t('home.nav.docs') }}
          </a>
        </div>
        <p class="text-xs text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
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
  sanitizeHomeContentHtml,
} from '@/utils/homeContent'
import { sanitizeUrl } from '@/utils/url'

type HomeIconName =
  | 'key'
  | 'sync'
  | 'chart'
  | 'shield'
  | 'chat'
  | 'terminal'
  | 'sparkles'
  | 'play'
  | 'search'
  | 'globe'
  | 'server'
  | 'clock'
  | 'document'
  | 'checkCircle'

interface HomeContentItem {
  icon: HomeIconName
  title: string
  description: string
  iconClass?: string
}

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API',
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true,
  }),
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || t('home.defaultSubtitle'),
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''),
)
const registrationEnabled = computed(
  () => appStore.cachedPublicSettings?.registration_enabled === true,
)
const rawHomeContent = computed(
  () => appStore.cachedPublicSettings?.home_content?.trim() || '',
)
const customHomeFrameUrl = computed(() => sanitizeHomeContentFrameUrl(rawHomeContent.value))
const customHomeHtml = computed(() =>
  customHomeFrameUrl.value ? '' : sanitizeHomeContentHtml(rawHomeContent.value),
)
const hasCustomHomeContent = computed(
  () => Boolean(customHomeFrameUrl.value || customHomeHtml.value.trim()),
)
const compactHomeEnabled = computed(
  () => appStore.cachedPublicSettings?.compact_home_enabled === true,
)

const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() =>
  authStore.isAdmin ? '/admin/dashboard' : '/dashboard',
)
const primaryEntryPath = computed(() => {
  if (isAuthenticated.value) return dashboardPath.value
  return registrationEnabled.value ? '/register' : '/login'
})
const primaryEntryLabel = computed(() => {
  if (isAuthenticated.value) return t('home.goToDashboard')
  return registrationEnabled.value ? t('home.startNow') : t('home.loginConsole')
})
const currentYear = new Date().getFullYear()

const sectionNavItems = computed(() => [
  { href: '#overview', label: t('home.nav.overview') },
  { href: '#reliability', label: t('home.nav.reliability') },
  { href: '#guide', label: t('home.nav.guide') },
  { href: '#faq', label: t('home.nav.faq') },
])

const heroAssurances = computed(() => [
  t('home.hero.assurances.clearUsage'),
  t('home.hero.assurances.flexibleAccess'),
  t('home.hero.assurances.visibleStatus'),
])

const serviceFlowItems = computed(() => [
  { icon: 'key' as const, label: t('home.flow.access'), iconClass: 'bg-blue-50 text-blue-700 dark:bg-blue-900/40 dark:text-blue-200' },
  { icon: 'sync' as const, label: t('home.flow.routing'), iconClass: 'bg-blue-50 text-blue-700 dark:bg-blue-900/35 dark:text-blue-200' },
  { icon: 'server' as const, label: t('home.flow.service'), iconClass: 'bg-blue-50 text-blue-700 dark:bg-blue-900/35 dark:text-blue-200' },
  { icon: 'chart' as const, label: t('home.flow.records'), iconClass: 'bg-blue-50 text-blue-700 dark:bg-blue-900/35 dark:text-blue-200' },
])

const overviewItems = computed<HomeContentItem[]>(() => [
  {
    icon: 'key',
    title: t('home.overview.items.access.title'),
    description: t('home.overview.items.access.description'),
    iconClass: 'bg-gray-100 text-blue-600 dark:bg-dark-800 dark:text-blue-300',
  },
  {
    icon: 'sync',
    title: t('home.overview.items.routing.title'),
    description: t('home.overview.items.routing.description'),
    iconClass: 'bg-gray-100 text-blue-600 dark:bg-dark-800 dark:text-blue-300',
  },
  {
    icon: 'chart',
    title: t('home.overview.items.usage.title'),
    description: t('home.overview.items.usage.description'),
    iconClass: 'bg-gray-100 text-blue-600 dark:bg-dark-800 dark:text-blue-300',
  },
  {
    icon: 'shield',
    title: t('home.overview.items.control.title'),
    description: t('home.overview.items.control.description'),
    iconClass: 'bg-gray-100 text-blue-600 dark:bg-dark-800 dark:text-blue-300',
  },
])

const capabilityItems = computed(() => [
  { icon: 'chat' as const, label: t('home.overview.catalog.conversation'), iconClass: 'text-blue-600 dark:text-blue-300' },
  { icon: 'terminal' as const, label: t('home.overview.catalog.code'), iconClass: 'text-blue-600 dark:text-blue-300' },
  { icon: 'sparkles' as const, label: t('home.overview.catalog.image'), iconClass: 'text-blue-600 dark:text-blue-300' },
  { icon: 'play' as const, label: t('home.overview.catalog.video'), iconClass: 'text-blue-600 dark:text-blue-300' },
  { icon: 'search' as const, label: t('home.overview.catalog.tools'), iconClass: 'text-blue-600 dark:text-blue-300' },
])

const reliabilityItems = computed<HomeContentItem[]>(() => [
  {
    icon: 'globe',
    title: t('home.reliability.items.status.title'),
    description: t('home.reliability.items.status.description'),
  },
  {
    icon: 'sync',
    title: t('home.reliability.items.continuity.title'),
    description: t('home.reliability.items.continuity.description'),
  },
  {
    icon: 'document',
    title: t('home.reliability.items.records.title'),
    description: t('home.reliability.items.records.description'),
  },
])

const guideItems = computed(() => [
  {
    title: t('home.guide.items.account.title'),
    description: t('home.guide.items.account.description'),
  },
  {
    title: t('home.guide.items.credential.title'),
    description: t('home.guide.items.credential.description'),
  },
  {
    title: t('home.guide.items.configure.title'),
    description: t('home.guide.items.configure.description'),
  },
])

const faqItems = computed(() => [
  {
    question: t('home.faq.items.start.question'),
    answer: t('home.faq.items.start.answer'),
  },
  {
    question: t('home.faq.items.capabilities.question'),
    answer: t('home.faq.items.capabilities.answer'),
  },
  {
    question: t('home.faq.items.usage.question'),
    answer: t('home.faq.items.usage.answer'),
  },
  {
    question: t('home.faq.items.failure.question'),
    answer: t('home.faq.items.failure.answer'),
  },
])

function closeMobileNav() {
  mobileNavOpen.value = false
}

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
.home-scene-frame {
  position: absolute;
  inset: 7% 5%;
  border-right: 1px solid rgba(148, 163, 184, 0.22);
  border-left: 1px solid rgba(148, 163, 184, 0.22);
}

.home-scene-line {
  position: absolute;
  right: 0;
  left: 0;
  height: 1px;
  background: rgba(148, 163, 184, 0.2);
}

.home-scene-line-one {
  top: 18%;
}

.home-scene-line-two {
  bottom: 14%;
}

.home-brand-signal img {
  box-shadow: 0 8px 24px rgba(37, 99, 235, 0.1);
}

.home-data-visual {
  display: flex;
  min-height: 310px;
  align-items: center;
  justify-content: center;
  isolation: isolate;
}

.home-data-visual-image {
  max-height: 390px;
  filter: drop-shadow(0 28px 42px rgba(49, 92, 167, 0.11));
  mix-blend-mode: multiply;
  animation: home-visual-enter 700ms ease-out both;
}

.dark .home-data-visual-image {
  border-radius: 8px;
  background: #f5f8fc;
  filter: none;
  mix-blend-mode: normal;
  opacity: 0.88;
}

.home-overview-item:not(:last-child) {
  border-bottom: 1px solid rgb(229 231 235);
}

.dark .home-overview-item:not(:last-child) {
  border-color: rgb(51 65 85);
}

.home-capability-item {
  min-width: 0;
  flex: 1 1 10rem;
  background: rgb(249 250 251);
}

.dark .home-capability-item {
  background: rgb(15 23 42);
}

.home-flow-pulse {
  display: none;
}

.home-faq-item summary::-webkit-details-marker {
  display: none;
}

.home-faq-item[open] .faq-chevron {
  transform: rotate(180deg);
}

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
  color: #2563eb;
  text-decoration: underline;
}

.safe-home-content :deep(pre) {
  overflow-x: auto;
  border: 1px solid #dbe2ea;
  border-radius: 8px;
  padding: 1rem;
  background: #f7f9fc;
}

@media (min-width: 640px) {
  .home-overview-item {
    border-bottom: 0;
  }

  .home-overview-item:nth-child(-n + 2) {
    border-bottom: 1px solid rgb(229 231 235);
  }

  .home-overview-item:nth-child(odd) {
    border-right: 1px solid rgb(229 231 235);
  }

  .dark .home-overview-item:nth-child(-n + 2),
  .dark .home-overview-item:nth-child(odd) {
    border-color: rgb(51 65 85);
  }
}

@media (min-width: 768px) {
  .home-flow-item:not(:last-child) {
    border-right: 1px solid rgb(229 231 235);
  }

  .dark .home-flow-item:not(:last-child) {
    border-color: rgb(51 65 85);
  }

  .home-flow-pulse {
    position: absolute;
    top: 50%;
    right: -18px;
    z-index: 2;
    display: block;
    width: 36px;
    height: 2px;
    background: #2563eb;
    animation: home-signal 2.8s ease-in-out infinite;
  }
}

@media (min-width: 1024px) {
  .home-overview-item,
  .home-overview-item:nth-child(-n + 2),
  .home-overview-item:nth-child(odd) {
    border-right: 0;
    border-bottom: 0;
  }

  .home-overview-item:not(:last-child) {
    border-right: 1px solid rgb(229 231 235);
  }

  .dark .home-overview-item:not(:last-child) {
    border-color: rgb(51 65 85);
  }
}

@keyframes home-signal {
  0%,
  100% {
    opacity: 0.18;
    transform: scaleX(0.35);
  }
  50% {
    opacity: 0.8;
    transform: scaleX(1);
  }
}

@keyframes home-visual-enter {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 1023px) {
  .home-data-visual {
    min-height: 220px;
    max-width: 760px;
    margin: 0 auto;
  }

  .home-data-visual-image {
    max-height: 300px;
  }
}

@media (max-width: 639px) {
  .home-hero > div:last-child {
    padding-top: 24px;
    padding-bottom: 24px;
  }

  .home-data-visual {
    min-height: 125px;
  }

  .home-data-visual-image {
    max-height: 125px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-flow-pulse,
  .home-data-visual-image {
    animation: none;
  }
}
</style>
