import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TablePageLayout from '../TablePageLayout.vue'

describe('TablePageLayout', () => {
  it('keeps horizontal table scrolling as the default', () => {
    const wrapper = mount(TablePageLayout, {
      slots: { table: '<table><tbody><tr><td>row</td></tr></tbody></table>' },
    })

    expect(wrapper.get('.table-page-layout').classes()).not.toContain('fit-table-content')
  })

  it('supports a page-local fit mode for vertically expanding tables', () => {
    const wrapper = mount(TablePageLayout, {
      props: { tableWidthMode: 'fit' },
      slots: { table: '<table><tbody><tr><td>row</td></tr></tbody></table>' },
    })

    expect(wrapper.get('.table-page-layout').classes()).toContain('fit-table-content')
  })
})
