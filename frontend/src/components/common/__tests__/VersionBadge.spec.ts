import { defineComponent, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import VersionBadge from '../VersionBadge.vue'

const state = vi.hoisted(() => ({
  auth: { isAdmin: true },
  app: {
    versionLoading: false,
    currentVersion: '0.1.172-52t.1',
    latestVersion: '0.1.173-52t.1',
    hasUpdate: true,
    releaseInfo: {
      name: 'v0.1.173-52t.1',
      body: '',
      published_at: '2026-08-10T00:00:00Z',
      html_url: 'https://github.com/hxly520/sub2api/releases/tag/v0.1.173-52t.1'
    },
    buildType: 'release',
    updateRepository: 'hxly520/sub2api',
    updateDockerImage: 'ghcr.io/hxly520/sub2api',
    updateChannel: 'stable',
    hotUpdatePolicy: 'image-update-required',
    hotUpdateAllowed: false,
    hotUpdateReasons: ['container layout changed'],
    fetchVersion: vi.fn(),
    clearVersionCache: vi.fn()
  }
}))

const api = vi.hoisted(() => ({
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  getRollbackVersions: vi.fn(),
  rollback: vi.fn()
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => state.auth,
  useAppStore: () => state.app
}))

vi.mock('@/api/admin/system', () => ({
  performUpdate: api.performUpdate,
  restartService: api.restartService,
  getRollbackVersions: api.getRollbackVersions,
  rollback: api.rollback
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: ref(false), copyToClipboard: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const IconStub = defineComponent({
  name: 'Icon',
  template: '<span />'
})

async function mountOpenBadge() {
  const wrapper = mount(VersionBadge, {
    global: { stubs: { Icon: IconStub } }
  })
  await wrapper.get('button').trigger('click')
  await nextTick()
  return wrapper
}

describe('VersionBadge private update policy', () => {
  beforeEach(() => {
    state.app.hotUpdatePolicy = 'image-update-required'
    state.app.hotUpdateAllowed = false
    state.app.hotUpdateReasons = ['container layout changed']
    state.app.fetchVersion.mockReset()
    state.app.clearVersionCache.mockReset()
    api.performUpdate.mockReset()
  })

  it('shows only the private Compose path for image-required releases', async () => {
    const wrapper = await mountOpenBadge()

    expect(wrapper.text()).toContain('version.composeUpdateRequired')
    expect(wrapper.text()).toContain('hxly520/sub2api')
    expect(wrapper.text()).toContain('ghcr.io/hxly520/sub2api')
    expect(wrapper.text()).toContain('docker compose pull sub2api')
    expect(wrapper.text()).toContain('docker compose up -d --no-deps sub2api')
    expect(wrapper.text()).not.toContain('version.updateNow')
  })

  it('keeps one-click update for releases explicitly marked hot-update-safe', async () => {
    state.app.hotUpdatePolicy = 'hot-update-safe'
    state.app.hotUpdateAllowed = true
    state.app.hotUpdateReasons = []

    const wrapper = await mountOpenBadge()

    expect(wrapper.text()).toContain('version.updateNow')
    expect(wrapper.text()).not.toContain('version.composeUpdateRequired')
  })
})
