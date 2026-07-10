import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import AvailableChannelModels from '../AvailableChannelModels.vue'
import type { UserSupportedModel } from '@/api/channels'

function createModels(count: number): UserSupportedModel[] {
  return Array.from({ length: count }, (_, index) => ({
    name: `video-model-${index + 1}`,
    platform: 'openai',
    pricing: null,
  }))
}

function mountModels(models: UserSupportedModel[], forceExpand = false) {
  const i18n = createI18n({
    legacy: false,
    locale: 'en',
    messages: {
      en: {
        availableChannels: {
          modelCount: (ctx: { named: (key: string) => unknown }) => `${ctx.named('count')} models`,
          showAllModels: (ctx: { named: (key: string) => unknown }) =>
            `Show all ${ctx.named('count')} models`,
          collapseModels: () => 'Collapse models',
        },
      },
    },
  })

  return mount(AvailableChannelModels, {
    props: {
      models,
      platform: 'openai',
      pricingKeyPrefix: 'availableChannels.pricing',
      noPricingLabel: 'No pricing',
      noModelsLabel: 'No models',
      forceExpand,
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        SupportedModelChip: {
          props: ['model'],
          template: '<button type="button" class="model-chip">{{ model.name }}</button>',
        },
      },
    },
  })
}

describe('AvailableChannelModels', () => {
  it('keeps every model in the DOM while applying mobile and desktop limits', () => {
    const wrapper = mountModels(createModels(10))
    const items = wrapper.findAll('[data-model-index]')

    expect(items).toHaveLength(10)
    expect(wrapper.text()).toContain('10 models')
    expect(items[3].classes()).toContain('inline-flex')
    expect(items[4].classes()).toContain('hidden')
    expect(items[4].classes()).toContain('lg:inline-flex')
    expect(items[7].classes()).toContain('lg:inline-flex')
    expect(items[8].classes()).toContain('hidden')
    expect(items[8].classes()).not.toContain('lg:inline-flex')
  })

  it('reveals all models and can collapse them again', async () => {
    const wrapper = mountModels(createModels(10))
    const toggle = wrapper.get('button[aria-expanded]')

    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')

    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.findAll('[data-model-index]').every((item) => item.classes().includes('inline-flex'))).toBe(true)
    expect(wrapper.text()).toContain('Collapse models')

    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-model-index="8"]').classes()).toContain('hidden')
  })

  it('force-expands search results without rendering a collapse control', () => {
    const wrapper = mountModels(createModels(10), true)

    expect(wrapper.find('button[aria-expanded]').exists()).toBe(false)
    expect(wrapper.findAll('[data-model-index]').every((item) => item.classes().includes('inline-flex'))).toBe(true)
  })
})
