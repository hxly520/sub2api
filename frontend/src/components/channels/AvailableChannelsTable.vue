<template>
  <div>
    <div v-if="loading" class="card py-10 text-center">
      <Icon name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
    </div>

    <div v-else-if="rows.length === 0" class="card py-12 text-center">
      <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyLabel }}</p>
    </div>

    <template v-else>
      <!-- Desktop/tablet-wide: content-driven columns plus horizontal overflow as a final safety net. -->
      <div class="card hidden overflow-hidden lg:block">
        <div class="overflow-x-auto">
          <table class="w-full min-w-[1040px] border-collapse text-sm">
            <thead>
              <tr class="border-b border-gray-100 bg-gray-50/50 text-xs font-medium uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-400">
                <th class="w-[160px] px-4 py-3 text-center">{{ columns.name }}</th>
                <th class="w-[200px] px-4 py-3 text-left">{{ columns.description }}</th>
                <th class="w-[130px] px-4 py-3 text-left">{{ columns.platform }}</th>
                <th class="min-w-[280px] px-4 py-3 text-left">{{ columns.groups }}</th>
                <th class="min-w-[360px] px-4 py-3 text-left">{{ columns.supportedModels }}</th>
              </tr>
            </thead>
            <tbody
              v-for="(channel, channelIndex) in rows"
              :key="`${channel.name}-${channelIndex}`"
              class="border-b-2 border-gray-200 last:border-b-0 dark:border-dark-600"
            >
              <tr
                v-for="(section, sectionIndex) in channel.platforms"
                :key="`${channel.name}-${section.platform}`"
                class="transition-colors hover:bg-gray-50/40 dark:hover:bg-dark-800/40"
                :class="{ 'border-t border-gray-100/70 dark:border-dark-700/50': sectionIndex > 0 }"
              >
                <td
                  v-if="sectionIndex === 0"
                  :rowspan="channel.platforms.length"
                  class="px-4 py-4 text-center align-middle font-medium text-gray-900 dark:text-white"
                >
                  <span class="break-words">{{ channel.name }}</span>
                </td>
                <td
                  v-if="sectionIndex === 0"
                  :rowspan="channel.platforms.length"
                  class="px-4 py-4 align-middle text-xs leading-5 text-gray-500 dark:text-gray-400"
                >
                  <template v-if="channel.description">{{ channel.description }}</template>
                  <span v-else class="text-gray-400">-</span>
                </td>
                <td class="align-top px-4 py-4">
                  <PlatformBadge :platform="section.platform" />
                </td>
                <td class="align-top px-4 py-4">
                  <AvailableChannelGroups
                    :section="section"
                    :user-group-rates="userGroupRates"
                  />
                </td>
                <td class="align-top px-4 py-4">
                  <AvailableChannelModels
                    :models="section.supported_models"
                    :platform="section.platform"
                    :pricing-key-prefix="pricingKeyPrefix"
                    :no-pricing-label="noPricingLabel"
                    :no-models-label="noModelsLabel"
                    :force-expand="forceExpandModels"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Mobile/tablet: cards avoid fixed table widths and preserve every model/group. -->
      <div class="space-y-3 lg:hidden">
        <article
          v-for="(channel, channelIndex) in rows"
          :key="`mobile-${channel.name}-${channelIndex}`"
          class="card overflow-hidden"
        >
          <header class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <h2 class="break-words text-sm font-semibold text-gray-900 dark:text-white">
              {{ channel.name }}
            </h2>
            <p v-if="channel.description" class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ channel.description }}
            </p>
          </header>

          <div
            v-for="section in channel.platforms"
            :key="`mobile-${channel.name}-${section.platform}`"
            class="space-y-4 border-b border-gray-100 px-4 py-4 last:border-b-0 dark:border-dark-700"
          >
            <div class="flex items-center justify-between gap-3">
              <span class="text-[11px] font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ columns.platform }}
              </span>
              <PlatformBadge :platform="section.platform" />
            </div>

            <section>
              <h3 class="mb-2 text-[11px] font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ columns.groups }}
              </h3>
              <AvailableChannelGroups
                :section="section"
                :user-group-rates="userGroupRates"
              />
            </section>

            <section>
              <h3 class="mb-2 text-[11px] font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ columns.supportedModels }}
              </h3>
              <AvailableChannelModels
                :models="section.supported_models"
                :platform="section.platform"
                :pricing-key-prefix="pricingKeyPrefix"
                :no-pricing-label="noPricingLabel"
                :no-models-label="noModelsLabel"
                :force-expand="forceExpandModels"
              />
            </section>
          </div>
        </article>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import PlatformBadge from './AvailableChannelPlatformBadge.vue'
import AvailableChannelGroups from './AvailableChannelGroups.vue'
import AvailableChannelModels from './AvailableChannelModels.vue'
import type { UserAvailableChannel } from '@/api/channels'

withDefaults(
  defineProps<{
    columns: {
      name: string
      description: string
      platform: string
      groups: string
      supportedModels: string
    }
    rows: UserAvailableChannel[]
    loading: boolean
    pricingKeyPrefix: string
    noPricingLabel: string
    noModelsLabel: string
    emptyLabel: string
    userGroupRates: Record<number, number>
    forceExpandModels?: boolean
  }>(),
  {
    forceExpandModels: false,
  },
)
</script>
