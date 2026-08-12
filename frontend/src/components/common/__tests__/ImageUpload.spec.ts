import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ImageUpload from '@/components/common/ImageUpload.vue'

function mountImageUpload() {
  const i18n = createI18n({
    legacy: false,
    locale: 'en',
    messages: {
      en: {
        common: {
          upload: () => 'Upload',
          remove: () => 'Remove',
          selectImageFile: () => 'Please select a PNG, JPG, WebP, or GIF image',
        },
      },
    },
  })

  return mount(ImageUpload, {
    props: { modelValue: '', mode: 'image' },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('ImageUpload', () => {
  it('accepts only raster formats in image mode', () => {
    const wrapper = mountImageUpload()

    expect(wrapper.get('input[type="file"]').attributes('accept')).toBe(
      'image/png,image/jpeg,image/webp,image/gif',
    )
  })

  it('rejects SVG files in image mode', async () => {
    const wrapper = mountImageUpload()
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
