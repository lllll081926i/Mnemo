import Dexie from 'dexie'

export interface IStateFileHash {
  size: number
  mtime: number
  
  presha1: string
  sha1: string
  name: string
}
class XBYDB3Cache extends Dexie {
  ifilehash: Dexie.Table<IStateFileHash>
  iobject: Dexie.Table<object, string>

  constructor() {
    super('XBYDB3Cache')

    this.version(10)
      .stores({
        ilog: '&logid',
        ifilehash: '++id,[size+mtime]',
        iobject: ''
      })
      .upgrade((tx: any) => {
        console.log('upgrade', tx)
      })
    this.ifilehash = this.table('ifilehash')
    this.iobject = this.table('iobject')
  }

  async getFileHashList(size: number, mtime: number): Promise<IStateFileHash[]> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.ifilehash.where({ size, mtime }).toArray()
  }

  async getFileHash(size: number, mtime: number, prehash: string, name: string): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const hashList = await this.ifilehash.where({ size, mtime }).toArray()
    for (let i = 0, maxi = hashList.length; i < maxi; i++) {
      if (hashList[i].presha1 == prehash && hashList[i].name == name) {
        return hashList[i].sha1
      }
    }
    return ''
  }

  async saveFileHash(value: IStateFileHash) {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.ifilehash.put(value).catch(() => {})
  }
}

const DBCache = new XBYDB3Cache()
export default DBCache
