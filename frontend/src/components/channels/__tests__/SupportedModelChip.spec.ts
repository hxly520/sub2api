import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'

import SupportedModelChip from '../SupportedModelChip.vue'
import { BILLING_MODE_TOKEN, BILLING_MODE_VIDEO } from '@/constants/channel'
import type { UserSupportedModel } from '@/api/channels'

const videoModel: UserSupportedModel = {
  name: 'seedance-2.0-fast-1080p',
  platform: 'openai',
  pricing: {
    billing_mode: BILLING_MODE_VIDEO,
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: 0.25,
    intervals: [],
  },
}

const intervalMultiplierModel: UserSupportedModel = {
  name: 'gpt-test',
  platform: '',
  pricing: {
    billing_mode: BILLING_MODE_TOKEN,
    input_price: 10e-6,
    output_price: 50e-6,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [
      {
        min_tokens: 272000,
        max_tokens: null,
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        input_multiplier: 2,
        output_multiplier: 1.5,
        per_request_price: null,
      },
    ],
  },
}

function mountChip(model: UserSupportedModel = videoModel, showPlatform = true) {
  const i18n = createI18n({
    legacy: false,
    locale: 'en',
    messages: {
      en: {
        availableChannels: {
          pricing: {
            billingMode: () => 'Billing Mode',
            billingModeToken: () => 'Per Token',
            billingModeVideo: () => 'Per Video',
            inputPrice: () => 'Input Price',
            outputPrice: () => 'Output Price',
            cacheWrite5mPrice: () => 'Cache Write Price',
            cacheReadPrice: () => 'Cache Read Price',
            unitPerMillion: () => '/ 1M tokens',
            intervals: () => 'Intervals',
            videoPrice: () => 'Video Price',
            unitPerSecond: () => '/ second',
          },
        },
      },
    },
  })

  return mount(SupportedModelChip, {
    attachTo: document.body,
    props: {
      model,
      pricingKeyPrefix: 'availableChannels.pricing',
      noPricingLabel: 'No pricing',
      showPlatform,
    },
    global: {
      plugins: [i18n],
      stubs: {
        PlatformIcon: true,
        PricingRow: {
          props: ['label', 'value', 'unit'],
          template: '<div class="pricing-row">{{ label }} {{ value }} {{ unit }}</div>',
        },
      },
    },
  })
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('SupportedModelChip', () => {
  it('pins the pricing popover on tap and closes it on the second tap', async () => {
    const wrapper = mountChip()
    const trigger = wrapper.get('button[aria-haspopup="dialog"]')
    const popoverId = trigger.attributes('aria-controls')
    const popover = document.getElementById(popoverId)

    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(popover).not.toBeNull()
    expect(popover?.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(popover?.style.display).not.toBe('none')
    expect(popover?.textContent).toContain('Video Price')
    expect(popover?.textContent).toContain('/ second')

    await trigger.trigger('click')
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(popover?.style.display).toBe('none')

    wrapper.unmount()
  })

  it('closes a pinned popover on outside pointerdown and Escape', async () => {
    const wrapper = mountChip()
    const trigger = wrapper.get('button[aria-haspopup="dialog"]')

    await trigger.trigger('click')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('false')

    await trigger.trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('false')

    wrapper.unmount()
  })

  it('resolves token interval multipliers from the base prices', async () => {
    const wrapper = mountChip(intervalMultiplierModel, false)

    await wrapper.get('button[aria-haspopup="dialog"]').trigger('mouseenter')
    await nextTick()

    expect(document.body.textContent).toContain('$20 / $75')
    wrapper.unmount()
  })
})
