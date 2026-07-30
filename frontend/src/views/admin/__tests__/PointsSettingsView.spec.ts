import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PointsSettingsView from '../PointsSettingsView.vue'
import { POINTS_FRAME_READY_MESSAGE } from '@/utils/embedded-url'

const { getPointsBridgeStatus, createPointsLaunch } = vi.hoisted(() => ({
  getPointsBridgeStatus: vi.fn(),
  createPointsLaunch: vi.fn(),
}))

vi.mock('@/api/points', () => ({
  getPointsBridgeStatus,
  createPointsLaunch,
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

const status = {
  enabled: false,
  configured: true,
  active: false,
  public_url: 'https://points.example.test',
  menu_label: '积分中心',
  launch_key_id: 'launch-v1',
  launch_secret_configured: true,
  credit_key_id: 'credit-v1',
  credit_secret_configured: true,
  launch_ttl_seconds: 60,
  clock_skew_seconds: 60,
}

function mountView() {
  return mount(PointsSettingsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: { template: '<span />' },
        TotpStepUpDialog: {
          props: ['controller'],
          template: `
            <div v-if="controller.visible.value">
              <button data-testid="verify-step-up" @click="controller.onVerified()">verify</button>
              <button data-testid="cancel-step-up" @click="controller.onCancel()">cancel</button>
            </div>
          `,
        },
      },
    },
  })
}

describe('PointsSettingsView', () => {
  beforeEach(() => {
    getPointsBridgeStatus.mockReset()
    createPointsLaunch.mockReset()
    document.documentElement.classList.remove('dark')
  })

  it('keeps the admin entry usable while user access is disabled', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('https://points.example.test')
    expect(wrapper.text()).toContain('pointsSettings.disabled')
    expect(wrapper.get('[data-testid="open-points-console"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[data-testid="points-not-configured"]').exists()).toBe(false)
  })

  it('shows the missing bridge state without hiding the settings page', async () => {
    getPointsBridgeStatus.mockResolvedValue({
      ...status,
      configured: false,
      public_url: '',
      launch_secret_configured: false,
      credit_secret_configured: false,
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="points-not-configured"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="open-points-console"]').attributes('disabled')).toBeDefined()
  })

  it('offers a retry state when status loading fails', async () => {
    getPointsBridgeStatus.mockRejectedValue(new Error('network'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="points-status-error"]').exists()).toBe(true)
  })

  it('prompts for step-up verification and treats cancellation as a no-op', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    createPointsLaunch.mockRejectedValue({
      status: 403,
      code: 'STEP_UP_REQUIRED',
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="open-points-console"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="cancel-step-up"]').exists()).toBe(true)

    await wrapper.get('[data-testid="cancel-step-up"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('pointsSettings.launchFailed')
    expect(wrapper.get('[data-testid="open-points-console"]').attributes('disabled')).toBeUndefined()
  })

  it('embeds the verified admin launch beside the bridge status', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    createPointsLaunch
      .mockRejectedValueOnce({ status: 403, code: 'STEP_UP_REQUIRED' })
      .mockResolvedValueOnce({
        launch_url: 'https://points.example.test/launch?ticket=verified-ticket&scope=admin',
      })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="open-points-console"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="verify-step-up"]').exists()).toBe(true)

    await wrapper.get('[data-testid="verify-step-up"]').trigger('click')
    await flushPromises()

    expect(createPointsLaunch).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('https://points.example.test')
    const frame = wrapper.get('[data-testid="points-console-frame"]')
    const frameURL = new URL(frame.attributes('src'))
    expect(frameURL.searchParams.get('ticket')).toBe('verified-ticket')
    expect(frameURL.searchParams.get('scope')).toBe('admin')
    expect(frameURL.searchParams.get('ui_mode')).toBe('embedded')
    expect(frame.attributes('sandbox')).toBe('allow-scripts allow-forms allow-same-origin')
    expect(frame.attributes('allow')).toBeUndefined()
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(wrapper.get('[data-testid="points-console-loading"]').exists()).toBe(true)

    window.dispatchEvent(new MessageEvent('message', {
      data: { type: POINTS_FRAME_READY_MESSAGE, role: 'admin' },
      origin: frameURL.origin,
      source: frame.element.contentWindow,
    }))
    await flushPromises()
    expect(wrapper.find('[data-testid="points-console-loading"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="points-console-error"]').exists()).toBe(false)
  })

  it('keeps the iframe in place and offers a fresh launch after a frame error', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    createPointsLaunch.mockResolvedValue({
      launch_url: 'https://points.example.test/launch?ticket=frame-error',
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="open-points-console"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="points-console-frame"]').trigger('error')

    expect(wrapper.get('[data-testid="points-console-error"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="retry-points-console"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="points-console-frame"]').exists()).toBe(true)
  })
})
