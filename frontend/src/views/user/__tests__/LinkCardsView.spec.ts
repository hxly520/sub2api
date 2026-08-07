import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import LinkCardsView from '../LinkCardsView.vue'

const { getAccess, getSettings, listGroups, listCards, listUsage, createCards, showSuccess, showError, authStore } = vi.hoisted(() => ({
  getAccess: vi.fn(), getSettings: vi.fn(), listGroups: vi.fn(), listCards: vi.fn(), listUsage: vi.fn(), createCards: vi.fn(),
  showSuccess: vi.fn(), showError: vi.fn(),
  authStore: { user: { id: 1, balance: 100 } as { id: number; balance: number } | null },
}))

vi.mock('@/api/linkCards', () => ({
  linkCardsAPI: {
    getAccess, getSettings, listGroups, listCards, listUsage, createCards,
    rechargeCard: vi.fn(), refundCard: vi.fn(),
  },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const appLayoutStub = defineComponent({
  name: 'AppLayout',
  props: { hideSidebar: { type: Boolean, default: false } },
  template: '<div><slot /></div>',
})

describe('LinkCardsView', () => {
  beforeEach(() => {
    authStore.user = { id: 1, balance: 100 }
    getAccess.mockResolvedValue({ enabled: true, allowed: true, development_mode: true })
    getSettings.mockResolvedValue({ enabled: true, public_portal_url: 'https://key.52token.org', api_base_url: 'https://api.52token.org/v1', default_concurrency: 5, max_batch_size: 100, minimum_deposit: null })
    listGroups.mockResolvedValue([{ id: 1, group_id: 8, name: 'Link Group', platform: 'openai', rate_multiplier: 0.08, default_concurrency: 5, enabled: true }])
    listCards.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10, pages: 0 })
    listUsage.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10, pages: 0 })
    createCards.mockReset()
    createCards.mockResolvedValue({ cards: [], quantity: 3, amount_per_card: 20, total_debited: 60, remaining_user_balance: 40 })
  })

  function mountView() {
    return mount(LinkCardsView, {
      global: {
        stubs: {
          AppLayout: appLayoutStub, Icon: true, SearchInput: true, Pagination: true,
          BaseDialog: true, ConfirmDialog: true, LinkCardUsageTable: true,
        },
      },
    })
  }

  it('keeps the standard user layout so the sidebar and header remain visible', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findComponent(appLayoutStub).props('hideSidebar')).toBe(false)
  })

  it('submits per-card amount and quantity while the server owns total debit', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[inputmode="decimal"]').setValue('20')
    await wrapper.get('input[type="number"]').setValue('3')
    const createButton = wrapper.findAll('button').find((button) => button.text().includes('linkCards.createAction'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')
    await flushPromises()

    expect(createCards).toHaveBeenCalledTimes(1)
    expect(createCards.mock.calls[0][0]).toEqual({ group_id: 8, quantity: 3, amount: 20 })
    expect(createCards.mock.calls[0][0]).not.toHaveProperty('total_debit')
    expect(createCards.mock.calls[0][1]).toEqual(expect.any(String))
    expect(authStore.user?.balance).toBe(40)
  })

  it('does not submit a batch whose displayed total exceeds the current balance', async () => {
    authStore.user = { id: 1, balance: 10 }
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[inputmode="decimal"]').setValue('20')
    const createButton = wrapper.findAll('button').find((button) => button.text().includes('linkCards.createAction'))
    expect(createButton?.attributes('disabled')).toBeDefined()
    await createButton!.trigger('click')

    expect(createCards).not.toHaveBeenCalled()
  })

  it('reuses the same idempotency key when an identical create request is retried', async () => {
    createCards.mockRejectedValueOnce(new Error('response lost')).mockResolvedValueOnce({
      cards: [], quantity: 1, amount_per_card: 10, total_debited: 10, remaining_user_balance: 90,
    })
    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((button) => button.text().includes('linkCards.createAction'))
    await createButton!.trigger('click')
    await flushPromises()
    await createButton!.trigger('click')
    await flushPromises()

    expect(createCards).toHaveBeenCalledTimes(2)
    expect(createCards.mock.calls[1][1]).toBe(createCards.mock.calls[0][1])
  })
})
