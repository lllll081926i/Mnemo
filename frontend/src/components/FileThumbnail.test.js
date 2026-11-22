import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { expect, it } from 'vitest'
import FileThumbnail from './FileThumbnail.vue'

it('保留加载占位图标，坏地址只回退当前图片，换地址后能正常显示', async () => {
  const failures = reactive(new Set())
  const wrapper = mount(FileThumbnail, { props: { source: '/old.png', icon: 'image', size: 20, failures, onFailed: source => failures.add(source) } })
  expect(wrapper.get('.file-thumbnail-placeholder').classes()).not.toContain('ready')
  expect(wrapper.get('img').attributes('decoding')).toBe('async')
  await wrapper.get('img').trigger('error')
  expect(wrapper.find('img').exists()).toBe(false)
  expect(wrapper.get('.file-thumbnail-placeholder').classes()).not.toContain('ready')
  await wrapper.setProps({ source: '/new.png' })
  await wrapper.get('img').trigger('load')
  expect(wrapper.get('img').classes()).toContain('ready')
  expect(wrapper.get('.file-thumbnail-placeholder').classes()).toContain('ready')
  await wrapper.setProps({ source: '/old.png' })
  expect(wrapper.find('img').exists()).toBe(false)
  wrapper.unmount()
})
