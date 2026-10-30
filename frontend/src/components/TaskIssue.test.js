import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TaskIssue from './TaskIssue.vue'

describe('任务处理建议', () => {
  it('没有结构化诊断时不猜测旧任务原因', () => {
    const wrapper = mount(TaskIssue)
    expect(wrapper.find('details').exists()).toBe(false)
  })
  it('只在统一通知中展示原因和网络入口，保留原账号身份', async () => {
    const wrapper = mount(TaskIssue, { props: { userId: 'original', issue: { code: 'NETWORK_UNAVAILABLE', message: '网络请求中断', action: 'network_settings' } } })
    expect(wrapper.text()).toBe('查看通知')
    expect(wrapper.find('details').exists()).toBe(false)
    let notice
    window.addEventListener('mnemo:drive-notice', event => { notice = event.detail }, { once: true })
    await wrapper.get('button').trigger('click')
    expect(notice.message).toContain('手动恢复任务')
    expect(notice.action).toMatchObject({ repair: 'network_settings', userId: 'original' })
    expect(wrapper.emitted('repair')).toBeUndefined()
    wrapper.unmount()
  })
  it('通知保留处理建议，冷却截止不承诺解除限制', async () => {
    const wrapper = mount(TaskIssue, { props: { writeOperation: true, issue: { code: 'PROVIDER_COOLDOWN', action: 'wait', message: '服务商限流', retryAt: Math.floor(Date.now() / 1000) + 60 } } })
    let notice
    window.addEventListener('mnemo:drive-notice', event => { notice = event.detail }, { once: true })
    await wrapper.get('button').trigger('click')
    expect(notice.message).toContain('到时不代表已解除限制')
    expect(notice.message).not.toContain('PROVIDER_COOLDOWN')
    wrapper.unmount()
  })
  it('未知操作或无效时间不会生成修复指令或错误日期', async () => {
    const wrapper = mount(TaskIssue, { props: { issue: { code: 'UNKNOWN', action: 'arbitrary_command', retryAt: 1e30 } } })
    let notice
    window.addEventListener('mnemo:drive-notice', event => { notice = event.detail }, { once: true })
    await wrapper.get('button').trigger('click')
    expect(notice.action.repair).toBeUndefined()
    expect(notice.message).not.toContain('Invalid Date')
    expect(wrapper.text()).not.toContain('UNKNOWN')
    wrapper.unmount()
  })
})
