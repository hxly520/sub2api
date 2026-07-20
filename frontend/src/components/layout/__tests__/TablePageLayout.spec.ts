import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TablePageLayout from '../TablePageLayout.vue'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../TablePageLayout.vue')
const componentSource = readFileSync(componentPath, 'utf8')

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

describe('TablePageLayout responsive table scrolling', () => {
  it('does not disable the table horizontal scroll container in mobile mode', () => {
    const tableWrapperBlocks = Array.from(
      componentSource.matchAll(/([^{}]*:deep\(\.table-wrapper\)[^{}]*)\{([^{}]*)\}/g)
    )

    expect(tableWrapperBlocks.length).toBeGreaterThan(0)

    const baseBlock = tableWrapperBlocks.find(([selector]) => !selector.includes('.mobile-mode'))
    const mobileBlocks = tableWrapperBlocks.filter(([selector]) => selector.includes('.mobile-mode'))

    expect(baseBlock?.[2]).toContain('overflow-x-auto')
    expect(mobileBlocks.every(([, , declarations]) => !declarations.includes('overflow-visible'))).toBe(
      true
    )
  })
})
