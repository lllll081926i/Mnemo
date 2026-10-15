import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TaskIssue from './TaskIssue.vue'

describe('任务处理建议', () => {
  it('没有结构化诊断时不猜测旧任务原因', () => {
    const wrapper = mount(TaskIssue)
    expect(wrapper.find('details').exists()).toBe(false)
  })
  it('展示原因和网络入口，但不会自动恢复任务', async () => {
    const wrapper = mount(TaskIssue, { props: { issue: { code: 'NETWORK_UNAVAILABLE', message: '网络请求中断', action: 'network_settings' } } })
    expect(wrapper.text()).toContain('手动恢复任务')
    expect(wrapper.emitted('repair')).toBeUndefined()
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('repair')).toEqual([['network_settings']])
  })
  it('写入结果提示保留核对语义，冷却截止不承诺解除限制', () => {
    const wrapper = mount(TaskIssue, { props: { writeOperation: true, issue: { code: 'PROVIDER_COOLDOWN', action: 'wait', message: '服务商限流', retryAt: Math.floor(Date.now() / 1000) + 60 } } })
    expect(wrapper.text()).toContain('重试前确认目标目录')
    expect(wrapper.text()).toContain('到时不代表已解除限制')
    expect(wrapper.find('button').exists()).toBe(false)
  })
  it('未知操作或无效时间不会生成可执行入口或错误日期', () => {
    const wrapper = mount(TaskIssue, { props: { issue: { code: 'UNKNOWN', action: 'arbitrary_command', retryAt: 1e30 } } })
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Invalid Date')
    expect(wrapper.text()).toContain('核对错误详情')
  })
})
