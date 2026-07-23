import { setActivePinia } from 'pinia'
import pinia from '../store'
import { WorkerPage } from './workercmd'

setActivePinia(pinia)

window.WinMsgToMain = function (event: any) {
  window.WebWorkerPort.send(event)
}

window.WebWorkerPort.onMessage((data: any) => {
  Promise.resolve().then(() => {
    try {
      window.WinMsg?.(data)
    } catch {}
  })
})

const startWorker = () => WorkerPage('upload')
if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', startWorker, { once: true })
else startWorker()
