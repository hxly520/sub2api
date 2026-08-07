import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, deleteRequest } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  deleteRequest: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: deleteRequest }
}))

import { adminLinkCardsAPI, linkCardsAPI, publicLinkCardsAPI } from '@/api/linkCards'

describe('link cards API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    deleteRequest.mockReset()
    get.mockResolvedValue({ data: {} })
    post.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
    deleteRequest.mockResolvedValue({ data: {} })
  })

  it('uses the authenticated user access, settings, and group endpoints', async () => {
    await linkCardsAPI.getAccess()
    await linkCardsAPI.getSettings()
    await linkCardsAPI.listGroups()

    expect(get.mock.calls).toEqual([
      ['/link-cards/access'],
      ['/link-cards/settings'],
      ['/link-cards/groups']
    ])
  })

  it('passes card filters and explicit pagination unchanged', async () => {
    const params = {
      page: 3,
      page_size: 10,
      search: 'sk-card-live',
      status: 'active' as const,
      group_id: 8
    }

    await linkCardsAPI.listCards(params)

    expect(get).toHaveBeenCalledWith('/link-cards/cards', { params })
  })

  it('creates an atomic batch with an idempotency header and server-calculated totals', async () => {
    const request = { group_id: 8, quantity: 6, amount: 100 }

    await linkCardsAPI.createCards(request, 'issue-6-cards')

    expect(post).toHaveBeenCalledWith('/link-cards/cards', request, {
      headers: { 'Idempotency-Key': 'issue-6-cards' }
    })
  })

  it('recharges and refunds through card-specific idempotent endpoints', async () => {
    await linkCardsAPI.rechargeCard(41, { amount: 25 }, 'recharge-41')
    await linkCardsAPI.refundCard(41, 'refund-41', { reason: 'not activated' })

    expect(post.mock.calls).toEqual([
      [
        '/link-cards/cards/41/recharge',
        { amount: 25 },
        { headers: { 'Idempotency-Key': 'recharge-41' } }
      ],
      [
        '/link-cards/cards/41/refund',
        { reason: 'not activated' },
        { headers: { 'Idempotency-Key': 'refund-41' } }
      ]
    ])
  })

  it('passes usage filters and explicit pagination unchanged', async () => {
    const params = {
      page: 2,
      page_size: 10,
      card_id: 41,
      model: 'gpt-5.6',
      stream: true
    }

    await linkCardsAPI.listUsage(params)

    expect(get).toHaveBeenCalledWith('/link-cards/usage', { params })
  })

  it('uses the admin settings contract', async () => {
    const update = {
      enabled: true,
      development_mode: true,
      development_user_ids: [1],
      default_concurrency: 5
    }

    await adminLinkCardsAPI.getSettings()
    await adminLinkCardsAPI.updateSettings(update)

    expect(get).toHaveBeenCalledWith('/admin/link-cards/settings')
    expect(put).toHaveBeenCalledWith('/admin/link-cards/settings', update)
  })

  it('creates, updates, lists, and removes authorized groups on the collection endpoint', async () => {
    await adminLinkCardsAPI.listGroups()
    await adminLinkCardsAPI.authorizeGroup({ group_id: 8, default_concurrency: 5 })
    await adminLinkCardsAPI.updateGroup({ group_id: 8, default_concurrency: 3, enabled: true })
    await adminLinkCardsAPI.removeGroup(8)

    expect(get).toHaveBeenCalledWith('/admin/link-cards/groups')
    expect(post).toHaveBeenCalledWith('/admin/link-cards/groups', {
      group_id: 8,
      default_concurrency: 5
    })
    expect(put).toHaveBeenCalledWith('/admin/link-cards/groups', {
      group_id: 8,
      default_concurrency: 3,
      enabled: true
    })
    expect(deleteRequest).toHaveBeenCalledWith('/admin/link-cards/groups', {
      data: { group_id: 8 }
    })
  })

  it('lists all cards and usage for administrators', async () => {
    const cardParams = { page: 1, page_size: 10, creator_email: 'owner@example.test' }
    const usageParams = { page: 4, page_size: 10, key: 'sk-card-live' }

    await adminLinkCardsAPI.listCards(cardParams)
    await adminLinkCardsAPI.listUsage(usageParams)

    expect(get.mock.calls).toEqual([
      ['/admin/link-cards/cards', { params: cardParams }],
      ['/admin/link-cards/usage', { params: usageParams }]
    ])
  })

  it('sends privileged card actions with an idempotency header', async () => {
    const action = { action: 'set_limits' as const, concurrency: 5, rpm_limit: 120 }

    await adminLinkCardsAPI.runCardAction(91, action, 'admin-action-91')

    expect(post).toHaveBeenCalledWith('/admin/link-cards/cards/91/actions', action, {
      headers: { 'Idempotency-Key': 'admin-action-91' }
    })
  })

  it('activates using the full key and returns the unwrapped session response', async () => {
    const response = { session_token: 'session-token', expires_at: '2026-08-08T00:00:00Z' }
    post.mockResolvedValueOnce({ data: response })

    await expect(
      publicLinkCardsAPI.activate({ key: 'sk-card-live-full-value' })
    ).resolves.toEqual(response)
    expect(post).toHaveBeenCalledWith('/public/link-cards/activate', {
      key: 'sk-card-live-full-value'
    })
  })

  it('adds the link-card session header to public profile and usage requests', async () => {
    const params = { page: 1, page_size: 10, start_date: '2026-08-01' }

    await publicLinkCardsAPI.getMe('session-token')
    await publicLinkCardsAPI.listUsage('session-token', params)

    expect(get.mock.calls).toEqual([
      [
        '/public/link-cards/me',
        { headers: { 'X-Link-Card-Session': 'session-token' } }
      ],
      [
        '/public/link-cards/usage',
        {
          headers: { 'X-Link-Card-Session': 'session-token' },
          params
        }
      ]
    ])
  })
})
