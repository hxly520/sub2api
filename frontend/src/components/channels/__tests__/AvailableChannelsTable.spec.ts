import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AvailableChannelsTable from '../AvailableChannelsTable.vue'
import type { UserAvailableChannel } from '@/api/channels'

const rows: UserAvailableChannel[] = [
  {
    name: 'Video Channel',
    description: 'Grok and Seedance',
    platforms: [
      {
        platform: 'openai',
        groups: [],
        supported_models: [
          { name: 'grok-video', platform: 'openai', pricing: null },
          { name: 'seedance-2.0', platform: 'openai', pricing: null },
        ],
      },
    ],
  },
]

function mountTable() {
  return mount(AvailableChannelsTable, {
    props: {
      columns: {
        name: 'Channel',
        description: 'Description',
        platform: 'Platform',
        groups: 'Groups',
        supportedModels: 'Models',
      },
      rows,
      loading: false,
      pricingKeyPrefix: 'availableChannels.pricing',
      noPricingLabel: 'No pricing',
      noModelsLabel: 'No models',
      emptyLabel: 'Empty',
      userGroupRates: {},
    },
    global: {
      stubs: {
        Icon: true,
        AvailableChannelPlatformBadge: {
          props: ['platform'],
          template: '<span class="platform-badge">{{ platform }}</span>',
        },
        AvailableChannelGroups: true,
        AvailableChannelModels: {
          props: ['models'],
          template: '<div class="available-models">{{ models.map((model) => model.name).join(",") }}</div>',
        },
      },
    },
  })
}

describe('AvailableChannelsTable', () => {
  it('keeps the desktop table footprint and renders a mobile card alternative', () => {
    const wrapper = mountTable()
    const table = wrapper.get('table')

    expect(table.classes()).toContain('table-fixed')
    expect(table.classes()).not.toContain('min-w-[1040px]')
    expect(wrapper.find('.overflow-x-auto').exists()).toBe(false)
    expect(wrapper.find('article.card').exists()).toBe(true)
    expect(wrapper.text()).toContain('grok-video')
    expect(wrapper.text()).toContain('seedance-2.0')
  })

  it('mounts on the vertical scroll hook without clipping the desktop table', () => {
    const wrapper = mountTable()
    const desktopWrapper = wrapper.findAll('div').find((node) => node.classes().includes('lg:block'))

    expect(wrapper.classes()).toContain('table-wrapper')
    expect(desktopWrapper).toBeDefined()
    expect(desktopWrapper?.classes()).not.toContain('card')
    expect(desktopWrapper?.classes()).not.toContain('overflow-hidden')
  })
})
