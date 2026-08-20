import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import Modal from './Modal.vue'
import UiSelect from './UiSelect.vue'

const wrappers = []

function mountAttached(component, options = {}) {
  const wrapper = mount(component, { attachTo: document.body, ...options })
  wrappers.push(wrapper)
  return wrapper
}

afterEach(async () => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  await nextTick()
  document.body.replaceChildren()
})

describe('关键交互组件', () => {
  it('弹窗声明对话框语义，并由 Escape 请求关闭和恢复焦点', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()

    const wrapper = mountAttached(Modal, {
      props: { title: '删除文件' },
      slots: { default: '<button type="button">确认</button>' },
    })
    await nextTick()

    const dialog = document.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(dialog.getAttribute('aria-label')).toBe('删除文件')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    await nextTick()
    expect(document.activeElement).toBe(opener)
  })

  it('下拉框以键盘跳过禁用项并提交当前可选项', async () => {
    const wrapper = mountAttached(UiSelect, {
      props: {
        modelValue: 'alpha',
        placeholder: '选择供应商',
        options: [
          { value: 'alpha', label: 'Alpha' },
          { value: 'blocked', label: '不可用', disabled: true },
          { value: 'beta', label: 'Beta' },
        ],
      },
    })
    const trigger = wrapper.get('[role="combobox"]')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(document.querySelector('[role="listbox"]')).not.toBeNull()

    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([['beta']])
    expect(wrapper.emitted('change')).toEqual([['beta']])
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })
})
