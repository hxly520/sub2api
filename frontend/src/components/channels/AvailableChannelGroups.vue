<template>
  <div class="flex flex-col gap-2">
    <div v-if="exclusiveGroups.length > 0" class="flex flex-wrap items-center gap-1.5">
      <span
        class="inline-flex items-center gap-0.5 text-[10px] font-medium uppercase text-purple-600 dark:text-purple-400"
        :title="t('availableChannels.exclusiveTooltip')"
      >
        <Icon name="shield" size="xs" class="h-3 w-3" />
        {{ t('availableChannels.exclusive') }}
      </span>
      <div
        v-for="group in exclusiveGroups"
        :key="`exclusive-${group.id}`"
        class="inline-flex flex-wrap items-center gap-1"
      >
        <GroupBadge
          :name="group.name"
          :platform="group.platform as GroupPlatform"
          :subscription-type="(group.subscription_type || 'standard') as SubscriptionType"
          :rate-multiplier="group.rate_multiplier"
          :user-rate-multiplier="userGroupRates[group.id] ?? null"
          always-show-rate
        />
        <span
          v-if="hasPeakRate(group)"
          class="inline-flex items-center gap-1 rounded-md bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
          :title="peakRateTitle(group)"
        >
          <Icon name="clock" size="xs" class="h-3 w-3" />
          {{ peakRateLabel(group) }}
        </span>
      </div>
    </div>

    <div v-if="publicGroups.length > 0" class="flex flex-wrap items-center gap-1.5">
      <span
        class="inline-flex items-center gap-0.5 text-[10px] font-medium uppercase text-gray-500 dark:text-gray-400"
        :title="t('availableChannels.publicTooltip')"
      >
        <Icon name="globe" size="xs" class="h-3 w-3" />
        {{ t('availableChannels.public') }}
      </span>
      <div
        v-for="group in publicGroups"
        :key="`public-${group.id}`"
        class="inline-flex flex-wrap items-center gap-1"
      >
        <GroupBadge
          :name="group.name"
          :platform="group.platform as GroupPlatform"
          :subscription-type="(group.subscription_type || 'standard') as SubscriptionType"
          :rate-multiplier="group.rate_multiplier"
          :user-rate-multiplier="userGroupRates[group.id] ?? null"
          always-show-rate
        />
        <span
          v-if="hasPeakRate(group)"
          class="inline-flex items-center gap-1 rounded-md bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
          :title="peakRateTitle(group)"
        >
          <Icon name="clock" size="xs" class="h-3 w-3" />
          {{ peakRateLabel(group) }}
        </span>
      </div>
    </div>

    <span v-if="section.groups.length === 0" class="text-xs text-gray-400">-</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import type { UserAvailableGroup, UserChannelPlatformSection } from '@/api/channels'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { useAppStore } from '@/stores/app'
import {
  hasPeakRate as groupHasPeakRate,
  formatPeakRateWindow,
  serverTimezoneLabel,
} from '@/utils/peak-rate'

const props = defineProps<{
  section: UserChannelPlatformSection
  userGroupRates: Record<number, number>
}>()

const { t } = useI18n()
const appStore = useAppStore()

const exclusiveGroups = computed(() => props.section.groups.filter((group) => group.is_exclusive))
const publicGroups = computed(() => props.section.groups.filter((group) => !group.is_exclusive))

function hasPeakRate(group: UserAvailableGroup): boolean {
  return groupHasPeakRate(group)
}

function peakRateLabel(group: UserAvailableGroup): string {
  return formatPeakRateWindow(
    group,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset),
  )
}

function peakRateTitle(group: UserAvailableGroup): string {
  return t('common.peakRateTooltip', { window: peakRateLabel(group) }) + t('common.peakRateImageNote')
}
</script>
