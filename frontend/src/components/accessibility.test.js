import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import UiSelect from './UiSelect.vue'
import SegTabs from './SegTabs.vue'
import Modal from './Modal.vue'

vi.mock('../../wailsjs/runtime/runtime', () => ({
  WindowMinimise: vi.fn(),
  WindowToggleMaximise: vi.fn(),
  WindowIsMaximised: vi.fn().mockResolvedValue(false),
}))

const wrappers = []
const options = [{ value: 'a', label: '甲' }, { value: 'disabled', label: '禁用', disabled: true }, { value: 'b', label: '乙' }]
function render(component, settings = {}) {
  const wrapper = mount(component, { attachTo: document.body, global: { components: { UiSelect, SegTabs, Modal }, stubs: { UiIcon: true } }, ...settings })
  wrappers.push(wrapper)
  return wrapper
}
function key(element, value, settings = {}) {
  const event = new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true, ...settings })
  element.dispatchEvent(event)
  return event
}

afterEach(() => {
  wrappers.splice(0).reverse().forEach(wrapper => wrapper.unmount())
  document.body.replaceChildren()
  vi.restoreAllMocks()
})

describe('键盘与焦点隔离', () => {
  it('同一页面的下拉框与分段按钮使用独立 ID', async () => {
    const wrapper = render({
      data: () => ({ options, tabs: [{ key: 'a', label: '甲' }, { key: 'b', label: '乙' }] }),
      template: '<div><UiSelect :options="options"/><UiSelect :options="options"/><SegTabs :options="tabs" model-value="a"/><SegTabs :options="tabs" model-value="a"/></div>',
    })
    const selects = wrapper.findAll('[role="combobox"]')
    expect(selects[0].attributes('aria-controls')).not.toBe(selects[1].attributes('aria-controls'))
    await selects[0].trigger('click')
    await selects[1].trigger('click')
    const ids = [...document.querySelectorAll('[id]')].map(element => element.id)
    expect(new Set(ids).size).toBe(ids.length)
    const tabs = wrapper.findAllComponents(SegTabs)
    await tabs[1].findAll('button')[0].trigger('keydown', { key: 'ArrowRight' })
    await nextTick()
    expect(document.activeElement).toBe(tabs[1].findAll('button')[1].element)
    expect(tabs[0].emitted('update:modelValue')).toBeUndefined()
  })

  it('未选中时从首项开始，跳过禁用项并保持组合框焦点', async () => {
    const wrapper = render(UiSelect, { props: { options } })
    const trigger = wrapper.get('[role="combobox"]')
    trigger.element.focus()
    key(trigger.element, 'ArrowDown')
    await nextTick()
    expect(document.getElementById(trigger.attributes('aria-activedescendant')).textContent).toContain('甲')
    key(trigger.element, 'ArrowDown')
    await nextTick()
    expect(document.getElementById(trigger.attributes('aria-activedescendant')).textContent).toContain('乙')
    expect(document.activeElement).toBe(trigger.element)
    expect([...document.querySelectorAll('[role="option"]')].every(element => element.tabIndex === -1)).toBe(true)
    key(trigger.element, 'Enter')
    await nextTick()
    expect(wrapper.emitted('update:modelValue')).toEqual([['b']])
  })

  it('方向键不被另一处展开的下拉框劫持', async () => {
    const wrapper = render({ data: () => ({ options }), template: '<div><UiSelect :options="options"/><input aria-label="另一个输入框"/></div>' })
    const select = wrapper.getComponent(UiSelect)
    await select.get('button').trigger('click')
    const input = wrapper.get('input')
    input.element.focus()
    key(input.element, 'ArrowDown')
    await nextTick()
    expect(select.get('[role="combobox"]').attributes('aria-expanded')).toBe('false')
    expect(select.emitted('update:modelValue')).toBeUndefined()
    expect(document.activeElement).toBe(input.element)
  })

  it('Escape 先关闭下拉框，再关闭外层弹窗', async () => {
    const wrapper = render({ data: () => ({ options }), template: '<Modal title="选择目录"><UiSelect :options="options"/></Modal>' })
    const modal = wrapper.getComponent(Modal)
    const trigger = wrapper.getComponent(UiSelect).get('[role="combobox"]')
    trigger.element.focus()
    key(trigger.element, 'Enter')
    await nextTick()
    key(trigger.element, 'Escape')
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(modal.emitted('close')).toBeUndefined()
    key(trigger.element, 'Escape')
    expect(modal.emitted('close')).toHaveLength(1)
  })

  it('Tab 关闭下拉框并保留外层弹窗的焦点循环', async () => {
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{ width: 100, height: 30 }])
    const wrapper = render({ data: () => ({ options }), template: '<Modal title="设置" :window-controls="false" :focus-dialog="true"><button hidden>隐藏按钮</button><UiSelect :options="options"/></Modal>' })
    await nextTick()
    const trigger = wrapper.getComponent(UiSelect).get('[role="combobox"]')
    trigger.element.focus()
    key(trigger.element, 'Enter')
    await nextTick()
    const event = key(trigger.element, 'Tab')
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(event.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(document.querySelector('.modal-window-close'))
    key(document.activeElement, 'Tab', { shiftKey: true })
    expect(document.activeElement).toBe(trigger.element)
  })

  it('对话框自身有焦点时，Shift+Tab 进入最后一个有效控件', async () => {
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{ width: 100, height: 30 }])
    render(Modal, { props: { title: '确认', focusDialog: true }, slots: { default: '<button id="last-valid">确认</button><fieldset disabled><button>不可用</button></fieldset><a tabindex="-1" href="#">不可聚焦</a>' } })
    await nextTick()
    const dialog = document.querySelector('[role="dialog"]')
    dialog.focus()
    key(dialog, 'Tab', { shiftKey: true })
    expect(document.activeElement.id).toBe('last-valid')
  })

  it('关闭底层弹窗不会把焦点抢出仍打开的顶层弹窗', async () => {
    const wrapper = render({
      data: () => ({ parent: true, child: false }),
      template: '<div><Modal v-if="parent" title="底层"><button id="open-child">子操作</button></Modal><Modal v-if="child" title="顶层" :focus-dialog="true"><button>确认</button></Modal></div>',
    })
    await nextTick()
    document.getElementById('open-child').focus()
    await wrapper.setData({ child: true })
    await flushPromises()
    const top = document.querySelector('[aria-label="顶层"]')
    top.focus()
    await wrapper.setData({ parent: false })
    await flushPromises()
    expect(top.contains(document.activeElement)).toBe(true)
  })

  it('选项刷新和控件禁用不会留下无效活动 ID 或可操作浮层', async () => {
    const wrapper = render(UiSelect, { props: { options, modelValue: 'a' } })
    await wrapper.get('[role="combobox"]').trigger('click')
    await wrapper.setProps({ options: [{ value: 'only-disabled', label: '不可用', disabled: true }] })
    expect(wrapper.get('[role="combobox"]').attributes('aria-activedescendant')).toBeUndefined()
    await wrapper.setProps({ disabled: true })
    expect(wrapper.get('[role="combobox"]').attributes('aria-expanded')).toBe('false')
  })
})
