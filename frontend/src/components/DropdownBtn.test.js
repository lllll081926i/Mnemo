import { afterEach, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DropdownBtn from './DropdownBtn.vue'

let wrapper

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

it('重复点击触发按钮会关闭菜单，点击外部也会关闭', async () => {
  wrapper = mount(DropdownBtn, {
    attachTo: document.body,
    props: { label: '选择', items: [{ label: '全选', action: 'all' }] },
  })
  const button = wrapper.get('button')
  await button.trigger('click')
  await nextTick()
  expect(button.attributes('aria-expanded')).toBe('true')

  button.element.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  await button.trigger('click')
  await nextTick()
  expect(button.attributes('aria-expanded')).toBe('false')

  await button.trigger('click')
  await nextTick()
  document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  await nextTick()
  expect(button.attributes('aria-expanded')).toBe('false')
})
