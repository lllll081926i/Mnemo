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

it('菜单外部点击关闭后焦点回到触发按钮', async () => {
  wrapper = mount(DropdownBtn, {
    attachTo: document.body,
    props: { label: '选择', items: [{ label: '全选', action: 'all' }] },
  })
  const button = wrapper.get('button')
  button.element.focus()
  await button.trigger('click')
  await nextTick()
  // 菜单打开后焦点进入菜单项
  expect(document.activeElement).not.toBe(button.element)
  // 点击外部空白关闭，焦点应收回触发按钮而不是丢失到 body
  document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  await nextTick()
  await nextTick()
  expect(document.activeElement).toBe(button.element)
})
