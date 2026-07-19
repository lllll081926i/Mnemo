import { EventEmitter } from 'node:events'
import createSsdp, { type SSDP, type Service } from '@achingbrain/ssdp'
import Player from './player'

export interface DeviceInfo {
  name: string;
  host: string;
  xml: string;
  info: any;
  emitted?: boolean;
}

class Dlna extends EventEmitter {
  private ssdpClient?: SSDP
  private ssdpClientPromise?: Promise<SSDP>
  private searchController?: AbortController
  private casts: { [name: string]: DeviceInfo } = {}

  private getClient() {
    if (this.ssdpClient) return Promise.resolve(this.ssdpClient)
    this.ssdpClientPromise ||= createSsdp().then((client) => {
      this.ssdpClient = client
      client.on('error', (error) => this.emit('error', error))
      return client
    })
    return this.ssdpClientPromise
  }

  search() {
    this.searchController?.abort()
    const controller = new AbortController()
    this.searchController = controller
    void this.discover(controller)
  }

  private async discover(controller: AbortController) {
    try {
      const client = await this.getClient()
      if (controller.signal.aborted) return
      for await (const service of client.discover({ serviceType: 'urn:schemas-upnp-org:device:MediaRenderer:1', signal: controller.signal })) {
        this.addService(service)
      }
    } catch (error: any) {
      if (error?.name !== 'AbortError' && !controller.signal.aborted) this.emit('error', error)
    }
  }

  private addService(service: Service<any>) {
    const device = service.details?.device
    const name = String(device?.friendlyName || '').trim()
    if (!name) return
    const xml = service.location.toString()
    const host = service.location.hostname
    if (!this.casts[name]) {
      this.casts[name] = { name, host, xml, info: device }
      this.listen(this.casts[name])
    } else if (!this.casts[name].host) {
      this.casts[name].host = host
      this.casts[name].xml = xml
      this.listen(this.casts[name])
    }
  }

  listen(device: DeviceInfo) {
    if (!device || !device.host || device.emitted) return
    let player = new Player(device)
    this.emit('update', player)
    device.emitted = true
  }

  destroy() {
    this.casts = {}
    this.searchController?.abort()
    this.searchController = undefined
    const client = this.ssdpClient
    this.ssdpClient = undefined
    this.ssdpClientPromise = undefined
    if (client) void client.stop().catch((error) => this.emit('error', error))
  }
}

export default new Dlna()
