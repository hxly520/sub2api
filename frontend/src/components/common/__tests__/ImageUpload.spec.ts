import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ImageUpload from '@/components/common/ImageUpload.vue'

describe('ImageUpload', () => {
  it('accepts only raster formats in image mode', () => {
    const wrapper = mount(ImageUpload, {
      props: { modelValue: '', mode: 'image' },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.get('input[type="file"]').attributes('accept')).toBe(
      'image/png,image/jpeg,image/webp,image/gif',
    )
  })

  it('rejects SVG files in image mode', async () => {
    const wrapper = mount(ImageUpload, {
      props: { modelValue: '', mode: 'image' },
      global: { stubs: { Icon: true } },
    })
    const input = wrapper.get('input[type="file"]')
    const file = new File(['<svg xmlns="http://www.w3.org/2000/svg"/>'], 'logo.svg', {
      type: 'image/svg+xml',
    })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')

    expect(wrapper.text()).toContain('Please select a PNG, JPG, WebP, or GIF image')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
