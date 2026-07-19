import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const root = process.cwd()
const read = (relativePath: string) => fs.readFileSync(path.join(root, relativePath), 'utf8')
const methodSource = (source: string, start: string, end: string) => source.slice(source.indexOf(start), source.indexOf(end))

describe('multi-account provider boundaries', () => {
  it('uses the shared provider-aware preparation path when switching or falling back accounts', () => {
    const source = read('src/user/userdal.ts')
    const ensure = methodSource(source, 'private static async ensureTokenReady', 'static async aLoadFromDB')
    const logoff = methodSource(source, 'static async UserLogOff', 'static async UserClearFromDB')
    const change = methodSource(source, 'static async UserChange', 'static async UserRefreshByUserFace')

    expect(logoff).toContain('ensureTokenReady(token)')
    expect(change).toContain('ensureTokenReady(token)')
    expect(change).not.toContain('AliUser.ApiTokenRefreshAccount')
    expect(ensure).toContain('if (isNonAliyunProvider(token))')
  })

  it('keeps non-Aliyun accounts out of Aliyun refresh and sign-in APIs', () => {
    const source = read('src/user/userdal.ts')
    const login = methodSource(source, 'static async UserLogin', 'static async LoadPanData')
    const refresh = methodSource(source, 'static async UserRefreshByUserFace', 'static async UserAutoSign')
    const autoSign = source.slice(source.indexOf('static async UserAutoSign'))
    const cloud123Login = methodSource(login, 'if (isCloud123User(token))', '} else if (isBaiduUser(token))')

    expect(cloud123Login).not.toContain('OpenApiTokenRefreshAccount')
    expect(refresh).toContain('refreshCloud123AccessToken')
    expect(refresh).toContain('!isNonAliyunProvider(token)')
    expect(autoSign).toContain('isNonAliyunProvider(token)')
  })

  it('migrates Baidu and 115 fallback ids to stable remote account ids', () => {
    const baidu = read('src/utils/baidu.ts')
    const user = read('src/aliapi/user.ts')
    const userDal = read('src/user/userdal.ts')

    expect(baidu).toContain("buildDriveProviderUserId('baidu', uk)")
    expect(baidu).not.toContain('if (!token.user_id && uk)')
    expect(user).toContain("buildDriveProviderUserId('115', accountId)")
    expect(user).toContain('SaveUserToken(token, previousUserId)')
    expect(userDal).toContain('SaveUserToken(refreshed, previousUserId)')
    expect(userDal).not.toContain('refreshed.user_id = token.user_id || refreshed.user_id')
  })

  it('persists inline-key user records without passing a conflicting Dexie key', () => {
    const db = read('src/utils/db.ts')
    const userDal = read('src/user/userdal.ts')

    expect(db).toContain('return this.itoken.put(token)')
    expect(db).not.toContain('this.itoken.put(token, token.user_id)')
    expect(userDal).toContain('static async SaveUserToken')
    expect(userDal).toContain('await UserDAL.SaveUserToken(token)')
  })

  it('commits the active account only after its drive has loaded and restores failures', () => {
    const source = read('src/user/userdal.ts')
    const login = methodSource(source, 'static async UserLogin', 'static async LoadPanData')

    expect(login.indexOf('await UserDAL.LoadPanData(token)')).toBeLessThan(login.indexOf("await DB.saveValueString('uiDefaultUser', token.user_id)"))
    expect(login.indexOf('await UserDAL.LoadPanData(token)')).toBeLessThan(login.indexOf('useUserStore().userLogin(token.user_id)'))
    expect(login).toContain('previousActiveUserId')
    expect(login).toContain("message.error('加载账号失败")
  })

  it('disambiguates same-provider accounts and rejects duplicate mounted-storage names', () => {
    const account = read('src/utils/driveAccount.ts')
    const pan = read('src/pan/PanLeft.vue')
    const webdav = read('src/utils/webdavClient.ts')

    expect(account).toContain('detail:')
    expect(account).toContain('getDriveProviderAccountId')
    expect(pan).toContain('account.detail')
    expect(pan).toContain('handleRemoveDriveAccount')
    expect(pan).toContain('UserDAL.UserClearFromDB(account.user_id)')
    expect(pan).toContain('removeWebDavConnection')
    expect(pan).toContain('removeS3Connection')
    expect(webdav).toContain('WebDAV 连接名称“${name}”已存在')
  })
})
