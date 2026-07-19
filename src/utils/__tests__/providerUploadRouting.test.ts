import { readFileSync } from 'fs'
import path from 'path'
import { describe, expect, it } from 'vitest'

const read = (file: string) => readFileSync(path.resolve(process.cwd(), file), 'utf8').replace(/\r\n/g, '\n')

describe('provider upload routing', () => {
  it('routes every queue-enabled aggregate provider to a concrete handler', () => {
    const uploader = read('src/workerpage/uploader.ts')
    for (const provider of ['pikpak', 'quark', '139', '189']) {
      expect(uploader).toContain(`if (provider === '${provider}') return runUploadDisk`)
    }
    for (const handler of ['PikPakUploadDisk', 'QuarkUploadDisk', 'Cloud139UploadDisk', 'Cloud189UploadDisk']) {
      expect(uploader).toContain(handler)
    }
  })

  it('hides and rejects upload actions outside real drive folders', () => {
    const toolbar = read('src/pan/menus/PanTopbtn.vue')
    const actions = read('src/pan/topbtns/topbtn.ts')
    expect(toolbar).toContain('v-if="isShowBtn && !dirtype.includes(\'pic\') && capabilities.upload"')
    expect(actions).toContain("directoryId.startsWith('color') || directoryId.startsWith('search')")
    expect(actions).toContain("!['favorite', 'recover', 'trash'].includes(directoryId)")
  })
})
