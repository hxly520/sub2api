<template>
  <div>
    <div v-if="models.length > 0" class="mb-2 flex flex-wrap items-center justify-between gap-2">
      <span class="text-[11px] text-gray-500 dark:text-gray-400">
        {{ t('availableChannels.modelCount', { count: models.length }) }}
      </span>
      <button
        v-if="showToggle"
        type="button"
        class="inline-flex min-h-8 items-center gap-1 rounded-md px-2 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 focus:outline-none focus:ring-2 focus:ring-primary-500/40 dark:text-primary-400 dark:hover:bg-primary-900/20"
        :class="{ 'lg:hidden': models.length <= desktopLimit }"
        :aria-expanded="expanded || forceExpand"
        @click="expanded = !expanded"
      >
        <Icon :name="expanded ? 'chevronUp' : 'chevronDown'" size="xs" />
        {{
          expanded
            ? t('availableChannels.collapseModels')
            : t('availableChannels.showAllModels', { count: models.length })
        }}
      </button>
    </div>

    <div v-if="models.length > 0" class="flex flex-wrap items-center gap-1.5">
      <span
        v-for="(model, index) in models"
        :key="`${platform}-${model.name}`"
        class="max-w-full"
        :class="modelVisibilityClass(index)"
        :data-model-index="index"
      >
        <SupportedModelChip
          :model="model"
          :pricing-key-prefix="pricingKeyPrefix"
          :no-pricing-label="noPricingLabel"
          :show-platform="false"
          :platform-hint="platform"
        />
      </span>
    </div>

    <span v-else class="text-xs text-gray-400">{{ noModelsLabel }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import SupportedModelChip from './SupportedModelChip.vue'
import type { UserSupportedModel } from '@/api/channels'

const mobileLimit = 4
const desktopLimit = 8

const props = withDefaults(
  defineProps<{
    models: UserSupportedModel[]
    platform: string
    pricingKeyPrefix: string
    noPricingLabel: string
    noModelsLabel: string
    forceExpand?: boolean
  }>(),
  {
    forceExpand: false,
  },
)

const { t } = useI18n()
const expanded = ref(false)

const showToggle = computed(
  () => !props.forceExpand && props.models.length > mobileLimit,
)

watch(
  () => [props.platform, props.models.length],
  () => {
    expanded.value = false
  },
)

function modelVisibilityClass(index: number): string {
  if (expanded.value || props.forceExpand) return 'inline-flex'
  if (index >= desktopLimit) return 'hidden'
  if (index >= mobileLimit) return 'hidden lg:inline-flex'
  return 'inline-flex'
}
</script>
