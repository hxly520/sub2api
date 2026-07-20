<template>
  <div class="relative inline-block max-w-full">
    <button
      ref="triggerEl"
      type="button"
      :class="[
        'inline-flex min-h-7 max-w-full cursor-help items-center gap-1 rounded-md border px-2 py-0.5 text-left text-xs font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500/40',
        effectivePlatform
          ? platformBadgeClass(effectivePlatform)
          : 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300',
      ]"
      :aria-expanded="show"
      :aria-controls="popoverId"
      aria-haspopup="dialog"
      @mouseenter="onMouseEnter"
      @mouseleave="onMouseLeave"
      @focus="onFocus"
      @blur="onBlur"
      @click.stop="togglePinned"
    >
      <PlatformIcon
        v-if="effectivePlatform"
        :platform="effectivePlatform as GroupPlatform"
        size="xs"
        class="flex-shrink-0"
      />
      <span
        v-if="showPlatform && model.platform"
        class="flex-shrink-0 rounded bg-gray-200/60 px-1 text-[10px] uppercase text-gray-600 dark:bg-dark-700 dark:text-gray-400"
      >
        {{ model.platform }}
      </span>
      <span class="break-all">{{ model.name }}</span>
    </button>

    <Teleport to="body">
      <div
        v-show="show"
        :id="popoverId"
        ref="popoverEl"
        role="dialog"
        :aria-label="model.name"
        class="fixed z-[99999] max-h-[calc(100vh-1rem)] w-80 max-w-[min(22rem,calc(100vw-1rem))] overflow-y-auto rounded-lg border bg-white text-xs shadow-xl dark:bg-dark-800"
        :class="[popoverBorderClass]"
        :style="popoverStyle"
      >
        <div
          class="flex items-center justify-between gap-2 rounded-t-lg border-b px-3 py-2"
          :class="[popoverHeaderClass, popoverBorderClass]"
        >
          <span class="min-w-0 break-all font-semibold">{{ model.name }}</span>
          <span
            v-if="model.platform"
            class="flex-shrink-0 rounded bg-white/70 px-1.5 py-0.5 text-[10px] uppercase tracking-wide dark:bg-dark-900/60"
          >
            {{ model.platform }}
          </span>
        </div>

        <div class="p-3">
          <div v-if="!model.pricing" class="text-gray-500 dark:text-gray-400">
            {{ noPricingLabel }}
          </div>

          <div v-else class="space-y-2 text-gray-700 dark:text-gray-300">
            <div class="flex justify-between gap-4">
              <span class="text-gray-500 dark:text-gray-400">{{ t(prefixKey('billingMode')) }}</span>
              <span class="text-right">{{ billingModeLabel }}</span>
            </div>

            <template v-if="model.pricing.billing_mode === BILLING_MODE_TOKEN">
              <PricingRow
                :label="t(prefixKey('inputPrice'))"
                :value="model.pricing.input_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                :label="t(prefixKey('outputPrice'))"
                :value="model.pricing.output_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                :label="t(prefixKey('cacheWritePrice'))"
                :value="model.pricing.cache_write_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                :label="t(prefixKey('cacheReadPrice'))"
                :value="model.pricing.cache_read_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                v-if="model.pricing.image_input_price != null && model.pricing.image_input_price > 0"
                :label="t(prefixKey('imageInputPrice'))"
                :value="model.pricing.image_input_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                v-if="model.pricing.image_output_price != null && model.pricing.image_output_price > 0"
                :label="t(prefixKey('imageOutputPrice'))"
                :value="model.pricing.image_output_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
            </template>

            <PricingRow
              v-if="
                model.pricing.billing_mode === BILLING_MODE_PER_REQUEST &&
                model.pricing.per_request_price != null
              "
              :label="t(prefixKey('perRequestPrice'))"
              :value="model.pricing.per_request_price"
              :unit="t(prefixKey('unitPerRequest'))"
              :scale="1"
            />

            <PricingRow
              v-if="
                model.pricing.billing_mode === BILLING_MODE_IMAGE &&
                model.pricing.image_output_price != null
              "
              :label="t(prefixKey('imageOutputPrice'))"
              :value="model.pricing.image_output_price"
              :unit="t(prefixKey('unitPerRequest'))"
              :scale="1"
            />

            <PricingRow
              v-if="
                model.pricing.billing_mode === BILLING_MODE_VIDEO &&
                model.pricing.per_request_price != null
              "
              :label="t(prefixKey('videoPrice'))"
              :value="model.pricing.per_request_price"
              :unit="t(prefixKey('unitPerSecond'))"
              :scale="1"
            />

            <div
              v-if="model.pricing.intervals && model.pricing.intervals.length > 0"
              class="mt-2 border-t pt-2"
              :class="[popoverBorderClass]"
            >
              <div class="mb-1 font-medium text-gray-600 dark:text-gray-400">
                {{ t(prefixKey('intervals')) }}
              </div>
              <div class="space-y-1">
                <div
                  v-for="(interval, index) in model.pricing.intervals"
                  :key="index"
                  class="flex justify-between gap-4 text-[11px]"
                >
                  <span class="text-gray-500 dark:text-gray-400">
                    <template v-if="interval.tier_label">{{ interval.tier_label }}</template>
                    <template v-else>{{ formatRange(interval.min_tokens, interval.max_tokens) }}</template>
                  </span>
                  <span class="text-right">
                    {{ formatInterval(interval, model.pricing.billing_mode) }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import {
  computed,
  getCurrentInstance,
  nextTick,
  onBeforeUnmount,
  ref,
  watch,
} from 'vue'
import { useI18n } from 'vue-i18n'
import PricingRow from './PricingRow.vue'
import { formatScaled } from '@/utils/pricing'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  type BillingMode,
} from '@/constants/channel'
import type { UserPricingInterval, UserSupportedModel } from '@/api/channels'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import {
  platformBadgeClass,
  platformBorderClass,
  platformBadgeLightClass,
} from '@/utils/platformColors'

const props = withDefaults(
  defineProps<{
    model: UserSupportedModel
    pricingKeyPrefix?: string
    noPricingLabel?: string
    showPlatform?: boolean
    platformHint?: string
  }>(),
  {
    pricingKeyPrefix: 'availableChannels.pricing',
    noPricingLabel: '',
    showPlatform: true,
    platformHint: '',
  },
)

const { t } = useI18n()
const effectivePlatform = computed(() => props.model.platform || props.platformHint || '')
const perMillionScale = 1_000_000
const instanceUID = getCurrentInstance()?.uid ?? 0
const popoverId = `supported-model-pricing-${instanceUID}`

const popoverBorderClass = computed(() =>
  effectivePlatform.value
    ? platformBorderClass(effectivePlatform.value)
    : 'border-gray-200 dark:border-dark-600',
)
const popoverHeaderClass = computed(() =>
  effectivePlatform.value
    ? platformBadgeLightClass(effectivePlatform.value)
    : 'bg-gray-50 text-gray-700 dark:bg-dark-700/60 dark:text-gray-300',
)

function prefixKey(key: string): string {
  return `${props.pricingKeyPrefix}.${key}`
}

const billingModeLabel = computed(() => {
  switch (props.model.pricing?.billing_mode) {
    case BILLING_MODE_TOKEN:
      return t(prefixKey('billingModeToken'))
    case BILLING_MODE_PER_REQUEST:
      return t(prefixKey('billingModePerRequest'))
    case BILLING_MODE_IMAGE:
      return t(prefixKey('billingModeImage'))
    case BILLING_MODE_VIDEO:
      return t(prefixKey('billingModeVideo'))
    default:
      return '-'
  }
})

function formatRange(min: number, max: number | null): string {
  return `(${min}, ${max == null ? '∞' : String(max)}]`
}

function formatInterval(interval: UserPricingInterval, mode: BillingMode): string {
  if (
    mode === BILLING_MODE_PER_REQUEST ||
    mode === BILLING_MODE_IMAGE ||
    mode === BILLING_MODE_VIDEO
  ) {
    return formatScaled(interval.per_request_price, 1)
  }
  return `${formatScaled(interval.input_price, perMillionScale)} / ${formatScaled(
    interval.output_price,
    perMillionScale,
  )}`
}

const hovered = ref(false)
const focused = ref(false)
const pinned = ref(false)
const dismissed = ref(false)
const show = computed(
  () => pinned.value || (!dismissed.value && (hovered.value || focused.value)),
)
const triggerEl = ref<HTMLButtonElement | null>(null)
const popoverEl = ref<HTMLElement | null>(null)
const popoverStyle = ref<Record<string, string>>({ top: '0px', left: '0px' })
let listenersBound = false

function updatePosition() {
  const trigger = triggerEl.value
  if (!trigger) return

  const rect = trigger.getBoundingClientRect()
  const margin = 8
  const popover = popoverEl.value
  const popoverWidth = popover?.offsetWidth ?? 320
  const popoverHeight = popover?.offsetHeight ?? 240
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight

  let top = rect.bottom + margin
  if (top + popoverHeight > viewportHeight - margin) {
    top = Math.max(margin, rect.top - popoverHeight - margin)
  }

  let left = rect.left + rect.width / 2 - popoverWidth / 2
  left = Math.max(margin, Math.min(left, viewportWidth - margin - popoverWidth))

  popoverStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
  }
}

function onDocumentPointerDown(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Node)) return
  if (triggerEl.value?.contains(target) || popoverEl.value?.contains(target)) return
  pinned.value = false
  dismissed.value = true
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  pinned.value = false
  // focus() can synchronously emit focus and clear dismissed; restore the
  // dismissal afterwards so Escape never re-opens the popover.
  triggerEl.value?.focus({ preventScroll: true })
  dismissed.value = true
}

function bindListeners() {
  if (listenersBound) return
  listenersBound = true
  window.addEventListener('scroll', updatePosition, true)
  window.addEventListener('resize', updatePosition)
  document.addEventListener('pointerdown', onDocumentPointerDown, true)
  document.addEventListener('keydown', onDocumentKeydown)
}

function unbindListeners() {
  if (!listenersBound) return
  listenersBound = false
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
  document.removeEventListener('pointerdown', onDocumentPointerDown, true)
  document.removeEventListener('keydown', onDocumentKeydown)
}

function onMouseEnter() {
  hovered.value = true
  dismissed.value = false
}

function onMouseLeave() {
  hovered.value = false
}

function onFocus() {
  focused.value = true
  dismissed.value = false
}

function onBlur() {
  focused.value = false
  if (!hovered.value && !pinned.value) dismissed.value = false
}

function togglePinned() {
  if (pinned.value) {
    pinned.value = false
    dismissed.value = true
    return
  }
  pinned.value = true
  dismissed.value = false
}

watch(show, (visible) => {
  if (!visible) {
    unbindListeners()
    return
  }
  bindListeners()
  nextTick(updatePosition)
})

onBeforeUnmount(unbindListeners)
</script>
