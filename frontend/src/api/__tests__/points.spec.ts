import { beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { post },
}))

import { createPointsLaunch } from '@/api/points'

describe('points launch API', () => {
  beforeEach(() => {
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
})
