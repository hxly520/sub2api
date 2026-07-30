import { beforeEach, describe, expect, it, vi } from 'vitest'
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

function mountView() {
  return mount(PointsSettingsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: { template: '<span />' },
        TotpStepUpDialog: {
          props: ['controller'],
          template: '<button v-if="controller.visible.value" data-testid="cancel-step-up" @click="controller.onCancel()">cancel</button>',
        },
      },
    },
  })
}

describe('PointsSettingsView', () => {
  beforeEach(() => {
    getPointsBridgeStatus.mockReset()
    createPointsLaunch.mockReset()
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
})
