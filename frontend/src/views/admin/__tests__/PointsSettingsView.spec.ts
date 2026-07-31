import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PointsSettingsView from '../PointsSettingsView.vue'

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

interface PopupFixture {
  window: Window
  document: Document
  replace: ReturnType<typeof vi.fn>
  close: ReturnType<typeof vi.fn>
}

function createPopupFixture(): PopupFixture {
  const popupDocument = document.implementation.createHTMLDocument('')
  const replace = vi.fn()
  const close = vi.fn()
  const popupWindow = {
    opener: window,
    document: popupDocument,
    location: { replace },
    close,
    closed: false,
  } as unknown as Window

  return {
    window: popupWindow,
    document: popupDocument,
    replace,
    close,
  }
}

let openWindow: ReturnType<typeof vi.spyOn>
let popup: PopupFixture

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
    popup = createPopupFixture()
    openWindow = vi.spyOn(window, 'open').mockReturnValue(popup.window)
  })

  afterEach(() => {
    openWindow.mockRestore()
  })

  it('keeps the admin entry usable while user access is disabled', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('main').classes()).not.toContain('points-workspace-shell')
    expect(wrapper.get('.points-workspace-shell').exists()).toBe(true)
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

  it('opens and isolates a blank tab synchronously, then closes it when step-up is cancelled', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    createPointsLaunch.mockRejectedValue({
      status: 403,
      code: 'STEP_UP_REQUIRED',
    })
    const wrapper = mountView()
    await flushPromises()

    const click = wrapper.get('[data-testid="open-points-console"]').trigger('click')
    expect(openWindow).toHaveBeenCalledWith('about:blank', '_blank')
    expect(popup.window.opener).toBeNull()
    const referrerMeta = popup.document.querySelector('meta[name="referrer"]')
    expect(referrerMeta?.getAttribute('content')).toBe('no-referrer')
    expect(popup.document.body.textContent).toBe('pointsSettings.consoleLoading')
    expect(popup.replace).not.toHaveBeenCalled()

    await click
    await flushPromises()
    expect(wrapper.get('[data-testid="cancel-step-up"]').exists()).toBe(true)

    await wrapper.get('[data-testid="cancel-step-up"]').trigger('click')
    await flushPromises()
    expect(popup.close).toHaveBeenCalledTimes(1)
    expect(popup.replace).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('pointsSettings.launchFailed')
    expect(wrapper.get('[data-testid="open-points-console"]').attributes('disabled')).toBeUndefined()
  })

  it('navigates the isolated tab after verified admin launch and keeps the settings page in place', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    createPointsLaunch
      .mockRejectedValueOnce({ status: 403, code: 'STEP_UP_REQUIRED' })
      .mockResolvedValueOnce({
        launch_url: 'https://points.example.test/launch?ticket=verified-ticket&scope=admin',
      })
    const wrapper = mountView()
    await flushPromises()

    const click = wrapper.get('[data-testid="open-points-console"]').trigger('click')
    expect(openWindow).toHaveBeenCalledWith('about:blank', '_blank')
    expect(popup.replace).not.toHaveBeenCalled()

    await click
    await flushPromises()
    expect(wrapper.get('[data-testid="verify-step-up"]').exists()).toBe(true)

    await wrapper.get('[data-testid="verify-step-up"]').trigger('click')
    await flushPromises()

    expect(createPointsLaunch).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('https://points.example.test')
    expect(openWindow).toHaveBeenCalledTimes(1)
    expect(popup.replace).toHaveBeenCalledWith(
      'https://points.example.test/launch?ticket=verified-ticket&scope=admin',
    )
    expect(popup.close).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="points-console-frame"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="points-console-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="points-console-error"]').exists()).toBe(false)
  })

  it('shows an error without requesting a ticket when the browser blocks the blank tab', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    openWindow.mockReturnValue(null)
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="open-points-console"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="points-console-error"]').exists()).toBe(true)
    expect(openWindow).toHaveBeenCalledWith('about:blank', '_blank')
    expect(createPointsLaunch).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="points-console-frame"]').exists()).toBe(false)
  })

  it('closes the blank tab and reports an API launch failure', async () => {
    getPointsBridgeStatus.mockResolvedValue(status)
    createPointsLaunch.mockRejectedValue(new Error('network'))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="open-points-console"]').trigger('click')
    await flushPromises()

    expect(popup.close).toHaveBeenCalledTimes(1)
    expect(popup.replace).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="points-console-error"]').exists()).toBe(true)
  })
})
