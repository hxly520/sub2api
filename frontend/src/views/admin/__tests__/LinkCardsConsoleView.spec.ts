import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LinkCardsConsoleView from '../LinkCardsConsoleView.vue'

const {
  getSettings, updateSettings, listGroups, authorizeGroup, removeGroup,
  listCards, listUsage, runCardAction, getAllGroups, showSuccess, showError,
} = vi.hoisted(() => ({
  getSettings: vi.fn(), updateSettings: vi.fn(), listGroups: vi.fn(), authorizeGroup: vi.fn(), removeGroup: vi.fn(),
  listCards: vi.fn(), listUsage: vi.fn(), runCardAction: vi.fn(), getAllGroups: vi.fn(), showSuccess: vi.fn(), showError: vi.fn(),
}))

vi.mock('@/api/linkCards', () => ({
  adminLinkCardsAPI: { getSettings, updateSettings, listGroups, authorizeGroup, removeGroup, listCards, listUsage, runCardAction },
}))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAll: getAllGroups } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const simpleStub = { template: '<div><slot /></div>' }

describe('LinkCardsConsoleView', () => {
  beforeEach(() => {
    getSettings.mockReset()
    updateSettings.mockReset()
    listGroups.mockReset()
    authorizeGroup.mockReset()
    listCards.mockReset()
    listUsage.mockReset()
    getAllGroups.mockReset()

    getSettings.mockResolvedValue({
      enabled: false,
      development_mode: true,
      development_user_ids: [1],
      public_portal_url: 'https://key.52token.org',
      api_base_url: 'https://api.52token.org/v1',
      default_concurrency: 5,
      default_rpm_limit: 0,
      max_batch_size: 100,
      minimum_deposit: null,
      public_session_ttl_seconds: 3600,
    })
    updateSettings.mockImplementation(async (request) => ({ ...request }))
    listGroups.mockResolvedValue([{ id: 10, group_id: 1, name: 'Authorized', platform: 'openai', rate_multiplier: 0.08, default_concurrency: 5, enabled: true }])
    getAllGroups.mockResolvedValue([
      { id: 1, name: 'Authorized', platform: 'openai', rate_multiplier: 0.08, status: 'active', subscription_type: 'standard' },
      { id: 2, name: 'Candidate', platform: 'anthropic', rate_multiplier: 0.1, status: 'active', subscription_type: 'standard' },
    ])
    listCards.mockResolvedValue({ items: [], total: 128, page: 1, page_size: 10, pages: 13 })
    listUsage.mockResolvedValue({ items: [], total: 42819, page: 1, page_size: 10, pages: 4282 })
    authorizeGroup.mockResolvedValue({ id: 11, group_id: 2, name: 'Candidate', enabled: true })
  })

  function mountView() {
    return mount(LinkCardsConsoleView, {
      global: {
        stubs: {
          AppLayout: simpleStub,
          Icon: true,
          SearchInput: true,
          Pagination: true,
          BaseDialog: true,
          LinkCardUsageTable: true,
        },
      },
    })
  }

  it('saves the global switch while preserving the ID 1 development rollout', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listCards).toHaveBeenCalledWith(expect.objectContaining({ page: 1, page_size: 10 }))
    expect(listUsage).toHaveBeenCalledWith(expect.objectContaining({ page: 1, page_size: 10 }))

    const featureToggle = wrapper.get('[data-testid="link-card-feature-toggle"]')
    await featureToggle.trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      enabled: true,
      development_mode: true,
      development_user_ids: [1],
      default_concurrency: 5,
      default_rpm_limit: 0,
      max_batch_size: 100,
      clear_minimum_deposit: true,
      public_portal_url: 'https://key.52token.org',
      api_base_url: 'https://api.52token.org/v1',
    }))
  })

  it('authorizes a group selected from the real Sub2API group list', async () => {
    const wrapper = mountView()
    await flushPromises()

    const groupsTab = wrapper.findAll('button').find((button) => button.text().includes('linkCards.groups'))
    await groupsTab!.trigger('click')
    const select = wrapper.get('[data-testid="authorize-group-select"]')
    await select.setValue('2')
    const authorizeButton = wrapper.findAll('button').find((button) => button.text().includes('linkCards.authorizeGroup'))
    await authorizeButton!.trigger('click')
    await flushPromises()

    expect(authorizeGroup).toHaveBeenCalledWith({ group_id: 2, default_concurrency: 5 })
  })
})
