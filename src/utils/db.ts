import Dexie from 'dexie'
import { ITokenInfo } from '../user/userstore'

/** Legacy table shape retained for local DB migration compatibility. */
type IOtherShareLinkModel = {
  share_id: string
  share_name: string
  description: string
  share_pwd: string
  expiration: string
  expired: boolean
  share_msg: string
  created_at: string
  updated_at: string
  saved_at: string
  saved_time: number
}

const TOKEN_SECRET_FIELDS: Array<keyof ITokenInfo> = [
  'access_token',
  'refresh_token',
  'open_api_access_token',
  'open_api_refresh_token',
  'signature'
]

type PersistedToken = ITokenInfo & {
  __mnemo_secret?: string
  __mnemo_secret_version?: number
}

async function protectToken(token: ITokenInfo): Promise<PersistedToken> {
  if (!window.WebSafeStorageEncrypt) throw new Error('系统安全存储接口不可用')
  const secrets: Partial<ITokenInfo> = {}
  const stored = { ...token } as PersistedToken
  for (const field of TOKEN_SECRET_FIELDS) {
    ;(secrets as any)[field] = token[field]
    ;(stored as any)[field] = ''
  }
  stored.__mnemo_secret = await window.WebSafeStorageEncrypt(JSON.stringify(secrets))
  stored.__mnemo_secret_version = 1
  return stored
}

async function revealToken(stored: PersistedToken): Promise<ITokenInfo> {
  if (!stored.__mnemo_secret) return stored
  if (!window.WebSafeStorageDecrypt) throw new Error('系统安全存储接口不可用')
  const secrets = JSON.parse(await window.WebSafeStorageDecrypt(stored.__mnemo_secret)) as Partial<ITokenInfo>
  const token = { ...stored, ...secrets } as PersistedToken
  delete token.__mnemo_secret
  delete token.__mnemo_secret_version
  return token
}

class MNEMODB3 extends Dexie {
  iobject: Dexie.Table<object, string>
  istring: Dexie.Table<string, string>
  inumber: Dexie.Table<number, string>
  ibool: Dexie.Table<boolean, string>
  icache: Dexie.Table<Blob, string>

  itoken: Dexie.Table<PersistedToken, string>
  iothershare: Dexie.Table<IOtherShareLinkModel, string>

  constructor() {
    super('MNEMO3Database')

    this.version(10)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',

        itoken: 'user_id',
        iothershare: 'share_id'
      })
      .upgrade((tx: any) => {
        console.log('upgrade', tx)
      })

    this.version(11)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',

        itoken: 'user_id',
        iothershare: 'share_id',
        imusic_track: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, artist, album'
      })
      .upgrade((tx: any) => {
        console.log('upgrade to v11 (music_track)', tx)
      })

    this.version(12)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',

        itoken: 'user_id',
        iothershare: 'share_id',
        imusic_track: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, artist, album',
        ibook_item: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, author, ext'
      })
      .upgrade((tx: any) => {
        console.log('upgrade to v12 (book_item)', tx)
      })

    this.version(13)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',

        itoken: 'user_id',
        iothershare: 'share_id',
        imusic_track: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, artist, album',
        ibook_item: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, author, ext',
        ibook_note: '&id, book_id, [user_id+drive_id], user_id, drive_id, file_id, kind, created_at, updated_at'
      })
      .upgrade((tx: any) => {
        console.log('upgrade to v13 (book_note)', tx)
      })

    this.version(14)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',

        itoken: 'user_id',
        iothershare: 'share_id',
        imusic_track: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, artist, album',
        ibook_item: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, author, ext',
        ibook_note: '&id, book_id, [user_id+drive_id], user_id, drive_id, file_id, kind, created_at, updated_at',
        ibook_bookmark: '&id, book_id, [user_id+drive_id], user_id, drive_id, file_id, percentage, created_at, updated_at'
      })
      .upgrade((tx: any) => {
        console.log('upgrade to v14 (book_bookmark)', tx)
      })

    this.version(15)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',

        itoken: 'user_id',
        iothershare: 'share_id',
        imusic_track: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, artist, album',
        ibook_item: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, author, ext',
        ibook_note: '&id, book_id, [user_id+drive_id], user_id, drive_id, file_id, kind, created_at, updated_at',
        ibook_bookmark: '&id, book_id, [user_id+drive_id], user_id, drive_id, file_id, percentage, created_at, updated_at',
        ibook_ai_chunk: '&id, bookId, [bookId+sourceHash], [bookId+settingsHash], sourceHash, settingsHash, sectionIndex, pageNumber',
        ibook_ai_meta: '&id, bookId, [bookId+sourceHash], [bookId+settingsHash], sourceHash, settingsHash, lastUpdated',
        ibook_ai_bm25: '&id, bookId, [bookId+sourceHash], [bookId+settingsHash], sourceHash, settingsHash, updatedAt',
        ibook_ai_conversation: '&id, bookId, [bookId+mode], updatedAt',
        ibook_ai_message: '&id, conversationId, createdAt'
      })
      .upgrade((tx: any) => {
        console.log('upgrade to v15 (book_ai)', tx)
      })

    this.version(16)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',
        itoken: 'user_id',
        iothershare: 'share_id',
        imusic_track: '&id, [user_id+drive_id], user_id, drive_id, parent_file_id, scanned_at, updated_at, artist, album',
        ibook_item: null,
        ibook_note: null,
        ibook_bookmark: null,
        ibook_ai_chunk: null,
        ibook_ai_meta: null,
        ibook_ai_bm25: null,
        ibook_ai_conversation: null,
        ibook_ai_message: null
      })

    this.version(17)
      .stores({
        iobject: '',
        istring: '',
        inumber: '',
        ibool: '',
        icache: '',
        itoken: 'user_id',
        iothershare: 'share_id',
        imusic_track: null
      })

    this.iobject = this.table('iobject')
    this.istring = this.table('istring')
    this.inumber = this.table('inumber')
    this.ibool = this.table('ibool')
    this.icache = this.table('icache')

    this.itoken = this.table('itoken')
    this.iothershare = this.table('iothershare')
  }

  async getValueString(key: string): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const val = await this.istring.get(key)
    if (val) return val
    else return ''
  }

  async saveValueString(key: string, value: string): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.istring.put(value || '', key)
  }

  async saveValueStringBatch(keys: string[], values: string[]): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.istring.bulkPut(values, keys)
  }

  async getValueNumber(key: string): Promise<number> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const val = await this.inumber.get(key)
    if (val) return val
    return 0
  }

  async saveValueNumber(key: string, value: number): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.inumber.put(value, key)
  }

  async getValueBool(key: string): Promise<boolean> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const val = await this.ibool.get(key)
    if (val) return true
    return false
  }

  async saveValueBool(key: string, value: boolean): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.ibool.put(value || false, key)
  }

  async getValueObject(key: string): Promise<object | undefined> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const val = await this.iobject.get(key)
    if (val) return val
    else return undefined
  }

  async saveValueObject(key: string, value: object): Promise<string | void> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.iobject.put(value, key).catch(() => {})
  }

  async saveValueObjectBatch(keys: string[], values: object[]): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.iobject.bulkPut(values, keys)
  }

  async deleteValueObject(key: string): Promise<void> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.iobject.delete(key)
  }

  async getUser(user_id: string): Promise<ITokenInfo | undefined> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const stored = await this.transaction('r', this.itoken, () => {
      return this.itoken.get(user_id)
    })
    if (!stored) return undefined
    const token = await revealToken(stored)
    if (!stored.__mnemo_secret) void this.saveUser(token).catch(() => undefined)
    return token
  }

  async getUserAll(): Promise<ITokenInfo[]> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const storedList = await this.transaction('r', this.itoken, () => {
      return this.itoken.toArray()
    })
    const list = await Promise.all(storedList.map(revealToken))
    const legacyTokens = storedList.filter((token) => !token.__mnemo_secret)
    if (legacyTokens.length) void this.saveUserBatch(legacyTokens).catch(() => undefined)
    return list.sort((a: ITokenInfo, b: ITokenInfo) => Number(b.used_size || 0) - Number(a.used_size || 0))
  }

  async deleteUser(user_id: string): Promise<void> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.itoken.delete(user_id)
  }

  async saveUser(token: ITokenInfo): Promise<string> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.itoken.put(await protectToken(token))
  }

  async saveUserBatch(tokens: ITokenInfo[]): Promise<boolean | string> {
    if (tokens.length == 0) return false
    if (!this.isOpen()) await this.open().catch()
    const protectedTokens = await Promise.all(tokens.map(protectToken))
    return this.itoken.bulkPut(protectedTokens).catch()
  }

  async getCache(key: string): Promise<Blob | undefined> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const val = await this.icache.get(key)
    return val
  }

  async saveCache(key: string, data: Blob) {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.icache.put(data, key)
  }

  async getOtherShare(share_id: string): Promise<IOtherShareLinkModel | undefined> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.iothershare.get(share_id)
  }

  async getOtherShareAll(): Promise<IOtherShareLinkModel[]> {
    if (!this.isOpen()) await this.open().catch(() => {})
    const list = await this.iothershare.toArray()
    return list.sort((a: IOtherShareLinkModel, b: IOtherShareLinkModel) => b.saved_time - a.saved_time)
  }

  async deleteOtherShareBatch(share_id_list: string[]): Promise<void> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.iothershare.bulkDelete(share_id_list)
  }

  async saveOtherShare(share: IOtherShareLinkModel): Promise<string | void> {
    if (!this.isOpen()) await this.open().catch(() => {})
    return this.iothershare.put(share, share.share_id).catch(() => {})
  }

}

const DB = new MNEMODB3()
export default DB
