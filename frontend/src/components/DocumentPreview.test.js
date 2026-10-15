import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const api = vi.hoisted(() => ({ PinFileSnapshot: vi.fn().mockResolvedValue(undefined), PreviewURL: vi.fn(), cachedPreviewDocumentURL: vi.fn() }))
vi.mock('../api', () => api)
import DocumentPreview from './DocumentPreview.vue'

afterEach(() => vi.restoreAllMocks())

it('文档预览关闭后释放迟到地址，不启动阅读器', async () => {
  let resolvePreview
  api.PreviewURL.mockImplementationOnce(() => new Promise(resolve => { resolvePreview = resolve }))
  const fetch = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true })
  const wrapper = mount(DocumentPreview, { props: { account: { user_id: 'account', drive_id: 'drive' }, file: { file_id: 'pdf', size: 10 }, kind: 'pdf' } })
  await flushPromises()
  wrapper.unmount()
  const address = 'http://127.0.0.1:45678/stream/document?t=local-token'
  resolvePreview(address)
  await flushPromises()
  expect(fetch).toHaveBeenCalledExactlyOnceWith(address, expect.objectContaining({ method: 'DELETE', keepalive: true }))
})
