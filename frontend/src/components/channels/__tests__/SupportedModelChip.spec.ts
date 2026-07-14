import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { nextTick } from 'vue'
import SupportedModelChip from '../SupportedModelChip.vue'
import { BILLING_MODE_VIDEO } from '@/constants/channel'
import type { UserSupportedModel } from '@/api/channels'

const model: UserSupportedModel = {
  name: 'seedance-2.0-fast-1080p',
  platform: 'openai',
  pricing: {
    billing_mode: BILLING_MODE_VIDEO,
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: 0.25,
    intervals: [],
  },
}

function mountChip() {
  const i18n = createI18n({
    legacy: false,
    locale: 'en',
    messages: {
      en: {
        availableChannels: {
          pricing: {
            billingMode: () => 'Billing Mode',
            billingModeVideo: () => 'Per Video',
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
})
