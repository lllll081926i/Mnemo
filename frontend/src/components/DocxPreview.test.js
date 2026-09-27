import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import DocxPreview from './DocxPreview.vue'

const { renderAsync } = vi.hoisted(() => ({ renderAsync: vi.fn() }))
renderAsync.mockImplementation(async (_bytes, body, _styles, options) => {
  const iframe = options.h({ tagName: 'iframe' })
  body.appendChild(iframe)
  const page = document.createElement('section')
  page.className = 'docx'
  body.appendChild(page)
})

vi.mock('docx-preview', () => ({
  renderAsync,
  defaultOptions: { h: element => document.createElement(element.tagName) },
}))

describe('DOCX 预览', () => {
  afterEach(() => { renderAsync.mockClear() })

  it('启用完整排版并隔离嵌入的 HTML', async () => {
    const wrapper = mount(DocxPreview, { props: { bytes: new Uint8Array([1]), file: { name: 'test.docx' } } })
    await flushPromises()
    expect(renderAsync).toHaveBeenCalledOnce()
    expect(renderAsync.mock.calls[0][3]).toMatchObject({ renderAltChunks: true, experimental: true, useBase64URL: true })
    expect(wrapper.find('iframe').attributes('sandbox')).toBe('')
    expect(wrapper.text()).toContain('1 页')
    wrapper.unmount()
  })

  it('实验排版失败时切换兼容模式', async () => {
    renderAsync.mockImplementationOnce(async () => { throw new Error('实验排版失败') })
    const wrapper = mount(DocxPreview, { props: { bytes: new Uint8Array([1]), file: { name: 'test.docx' } } })
    await flushPromises()
    expect(renderAsync).toHaveBeenCalledTimes(2)
    expect(renderAsync.mock.calls[1][3]).toMatchObject({ experimental: false, renderAltChunks: false })
    expect(wrapper.text()).toContain('兼容模式')
    wrapper.unmount()
  })

  it('两种排版均失败后可以原位重试', async () => {
    renderAsync.mockRejectedValueOnce(new Error('实验排版失败')).mockRejectedValueOnce(new Error('兼容排版失败'))
    const wrapper = mount(DocxPreview, { props: { bytes: new Uint8Array([1]), file: { name: 'test.docx' } } })
    await flushPromises()
    expect(wrapper.text()).toContain('兼容排版失败')
    await wrapper.findAll('button').find(button => button.text() === '重试').trigger('click')
    await flushPromises()
    expect(renderAsync).toHaveBeenCalledTimes(3)
    expect(wrapper.text()).toContain('1 页')
    wrapper.unmount()
  })
})
