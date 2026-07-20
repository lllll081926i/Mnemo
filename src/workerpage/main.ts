import { setActivePinia } from 'pinia'
import pinia from '../store'
import { WorkerPage } from './workercmd'

setActivePinia(pinia)

window.WinMsgToMain = function (event: any) {
  if (window.MainPort) window.MainPort.postMessage(event)
}

window.Electron.ipcRenderer.on('setPort', (_event: any) => {
  const [port] = _event.ports
  window.MainPort = port
  port.onmessage = (event: any) => {
    Promise.resolve().then(() => {
      try {
        window.WinMsg?.(event.data)
      } catch {}
    })
  }
})

const startWorker = () => WorkerPage('upload')
if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', startWorker, { once: true })
else startWorker()
