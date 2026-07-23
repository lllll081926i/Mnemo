import { MouseMessage } from '../store/mousestore'

export function isMouseButton(button: number, event: MouseMessage, fun: () => void): boolean {
  if (event.button === button && !event.Ctrl && !event.Shift && !event.Alt) {
    fun()
    return true
  }
  return false
}

export function isLeftClick(event: MouseMessage, fun: () => void): boolean {
  return isMouseButton(0, event, fun)
}

export const TestButton = isMouseButton

export function TestButtonAlt(button: number, event: MouseMessage, fun: () => void): boolean {
  if (event.button === button && event.Alt) {
    fun()
    return true
  }
  return false
}
