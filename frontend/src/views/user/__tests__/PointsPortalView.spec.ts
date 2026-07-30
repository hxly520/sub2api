import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PointsPortalView from '../PointsPortalView.vue'
import { POINTS_FRAME_READY_MESSAGE } from '@/utils/embedded-url'

const { createPointsLaunch, routeMeta } = vi.hoisted(() => ({
  createPointsLaunch: vi.fn(),
  routeMeta: { requiresAdmin: false },
}))

vi.mock('@/api/points', () => ({
  createPointsLaunch,
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ meta: routeMeta }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'zh-CN' },
      t: (key: string) => key,
    }),
  }
})

function mountView() {
  return mount(PointsPortalView, {
    global: {
      stubs: {
        AppLayout: { template: '<main data-testid="app-layout"><slot /></main>' },
        Icon: { template: '<span />' },
        RouterLink: {
          props: ['to'],
          template: '<a :data-to="to"><slot /></a>',
        },
      },
    },
  })
}

describe('PointsPortalView', () => {
  beforeEach(() => {
    createPointsLaunch.mockReset()
    routeMeta.requiresAdmin = false
    document.documentElement.classList.remove('dark')
  })

  it('loads the user points launch inside AppLayout without replacing the page', async () => {
    const currentURL = window.location.href
    createPointsLaunch.mockResolvedValue({
      launch_url: '/points/launch?ticket=signed-ticket&source=menu',
    })

    const wrapper = mountView()
    expect(wrapper.get('[data-testid="app-layout"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="points-portal-loading"]').exists()).toBe(true)

    await flushPromises()

    expect(createPointsLaunch).toHaveBeenCalledWith('user', {
      theme: 'light',
      language: 'zh-CN',
    })
    const frame = wrapper.get('[data-testid="points-portal-frame"]')
    const frameURL = new URL(frame.attributes('src'))
    expect(frameURL.searchParams.get('ticket')).toBe('signed-ticket')
    expect(frameURL.searchParams.get('source')).toBe('menu')
    expect(frameURL.searchParams.get('ui_mode')).toBe('embedded')
    expect(frame.attributes('sandbox')).toBe('allow-scripts allow-forms allow-same-origin')
    expect(frame.attributes('allow')).toBeUndefined()
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(window.location.href).toBe(currentURL)
    expect(wrapper.get('[data-testid="points-portal-loading"]').exists()).toBe(true)

    window.dispatchEvent(new MessageEvent('message', {
      data: { type: POINTS_FRAME_READY_MESSAGE, role: 'user' },
      origin: 'https://untrusted.example.test',
      source: frame.element.contentWindow,
    }))
    await flushPromises()
    expect(wrapper.get('[data-testid="points-portal-loading"]').exists()).toBe(true)

    window.dispatchEvent(new MessageEvent('message', {
      data: { type: POINTS_FRAME_READY_MESSAGE, role: 'user' },
      origin: frameURL.origin,
      source: frame.element.contentWindow,
    }))
    await flushPromises()
    expect(wrapper.find('[data-testid="points-portal-loading"]').exists()).toBe(false)
  })

  it('shows a complete retry state when launch creation fails', async () => {
    createPointsLaunch.mockRejectedValueOnce(new Error('network'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="points-portal-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="points-portal-frame"]').exists()).toBe(false)

    createPointsLaunch.mockResolvedValueOnce({
      launch_url: 'https://points.example.test/launch?ticket=retry-ticket',
    })
    await wrapper.get('[data-testid="retry-points-portal"]').trigger('click')
    await flushPromises()

    expect(createPointsLaunch).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="points-portal-frame"]').attributes('src')).toContain('retry-ticket')
    expect(wrapper.find('[data-testid="points-portal-error"]').exists()).toBe(false)
  })

  it('replaces a failed iframe with the retry state', async () => {
    createPointsLaunch.mockResolvedValue({
      launch_url: 'https://points.example.test/launch?ticket=frame-error',
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="points-portal-frame"]').trigger('error')

    expect(wrapper.get('[data-testid="points-portal-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="points-portal-loading"]').exists()).toBe(false)
  })
})
