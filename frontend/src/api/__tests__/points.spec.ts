import { beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())
const get = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { get, post },
}))

import { createPointsLaunch, getPointsBridgeStatus } from '@/api/points'

describe('points launch API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    post.mockResolvedValue({ data: { launch_url: 'https://example.test/launch?ticket=test' } })
  })

  it.each([
    ['user', '/points/launch'],
    ['admin', '/admin/points/launch'],
  ] as const)('uses the authenticated %s launch endpoint', async (role, path) => {
    const request = { theme: 'dark' as const, language: 'zh-CN' }
    await expect(createPointsLaunch(role, request)).resolves.toEqual({
      launch_url: 'https://example.test/launch?ticket=test',
    })
    expect(post).toHaveBeenCalledWith(path, request)
  })

  it('loads the admin-only bridge status', async () => {
    const status = {
      enabled: false,
      configured: true,
      active: false,
      public_url: 'https://points.example.test',
      menu_label: 'Points',
      launch_key_id: 'launch-v1',
      launch_secret_configured: true,
      credit_key_id: 'credit-v1',
      credit_secret_configured: true,
      launch_ttl_seconds: 60,
      clock_skew_seconds: 60,
    }
    get.mockResolvedValue({ data: status })

    await expect(getPointsBridgeStatus()).resolves.toEqual(status)
    expect(get).toHaveBeenCalledWith('/admin/points/status')
  })
})
