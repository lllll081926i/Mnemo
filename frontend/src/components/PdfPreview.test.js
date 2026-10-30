import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const pdf = vi.hoisted(() => ({ getDocument: vi.fn(), viewers: [] }))
vi.mock('pdfjs-dist', () => ({ getDocument: pdf.getDocument, GlobalWorkerOptions: {}, PasswordResponses: { INCORRECT_PASSWORD: 2 } }))
vi.mock('pdfjs-dist/web/pdf_viewer.mjs', () => ({
  ScrollMode: { VERTICAL: 0 },
  EventBus: class {
    callbacks = new Map()
    on(name, callback) { this.callbacks.set(name, callback) }
    dispatch(name, event = {}) { this.callbacks.get(name)?.(event) }
  },
  PDFViewer: class {
    constructor(options) { this.options = options; this.pagesPromise = Promise.resolve(); this.currentScale = 1; pdf.viewers.push(this) }
    setDocument = vi.fn(document => {
      if (document?.initializationError) this.pagesPromise = Promise.reject(document.initializationError)
      else if (document) this.options.eventBus.dispatch('pagesinit')
    })
  },
}))
import PdfPreview from './PdfPreview.vue'

const wrappers = []
function open() {
  const wrapper = mount(PdfPreview, { props: { url: '/local-test.pdf' } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => { pdf.getDocument.mockReset(); pdf.viewers.length = 0 })
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

it('连续阅读保留密码解锁，并在关闭后取消监听和销毁文档', async () => {
  let unlock
  const document = { numPages: 4 }
  const task = { promise: new Promise(resolve => { unlock = resolve }), destroy: vi.fn().mockResolvedValue(undefined) }
  pdf.getDocument.mockReturnValue(task)
  const wrapper = open()
  await flushPromises()
  task.onPassword(() => unlock(document), 2)
  await flushPromises()
  expect(wrapper.text()).toContain('密码不正确')
  await wrapper.get('input').setValue('password')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.text()).toContain('1 / 4')
  expect(wrapper.text()).not.toMatch(/上一页|下一页/)
  expect(pdf.viewers[0].scrollMode).toBe(0)
  wrapper.unmount()
  expect(task.destroy).toHaveBeenCalledOnce()
  expect(pdf.viewers[0].options.abortSignal.aborted).toBe(true)
  expect(pdf.viewers[0].setDocument).toHaveBeenLastCalledWith(null)
})

it('加载失败后销毁旧任务并在当前窗口重试', async () => {
  const failed = { destroy: vi.fn().mockResolvedValue(undefined) }
  const retried = { promise: Promise.resolve({ numPages: 3 }), destroy: vi.fn().mockResolvedValue(undefined) }
  pdf.getDocument.mockImplementationOnce(() => ({ ...failed, promise: Promise.reject(new Error('读取失败')) })).mockReturnValueOnce(retried)
  const wrapper = open()
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('读取失败')
  await wrapper.get('[role="alert"] button').trigger('click')
  await flushPromises()
  expect(failed.destroy).toHaveBeenCalledOnce()
  expect(pdf.viewers[0].options.abortSignal.aborted).toBe(true)
  expect(wrapper.text()).toContain('1 / 3')
  expect(wrapper.find('[role="alert"]').exists()).toBe(false)
})

it('关闭后不再装入迟到文档或处理迟到渲染错误', async () => {
  let resolve
  const task = { promise: new Promise(done => { resolve = done }), destroy: vi.fn().mockResolvedValue(undefined) }
  pdf.getDocument.mockReturnValue(task)
  const wrapper = open()
  await flushPromises()
  const viewer = pdf.viewers[0]
  wrapper.unmount()
  resolve({ numPages: 8 })
  viewer.options.eventBus.dispatch('pagerendered', { error: new Error('迟到错误') })
  await flushPromises()
  expect(task.destroy).toHaveBeenCalledOnce()
  expect(viewer.setDocument.mock.calls).toEqual([[null]])
})

it('文档打开后第一页初始化失败时退出加载并允许重试', async () => {
  pdf.getDocument.mockReturnValue({ promise: Promise.resolve({ numPages: 2, initializationError: new Error('页面数据损坏') }), destroy: vi.fn().mockResolvedValue(undefined) })
  const wrapper = open()
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('页面数据损坏')
  expect(wrapper.text()).not.toContain('正在解析')
})
