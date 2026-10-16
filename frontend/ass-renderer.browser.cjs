const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const assert = require('node:assert/strict')

async function main() {
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  try {
    const page = await browser.newPage()
    const failures = []
    page.on('pageerror', error => failures.push(error.message))
    const url = new URL('/ass-engine-qa', process.env.QA_URL || 'http://127.0.0.1:5186').href
    await page.route(url, route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body></body></html>' }))
    await page.goto(url)
    const result = await page.evaluate(async () => {
      const { loadASSRenderer } = await import('/src/player/ass-renderer.js')
      const { Renderer, assets } = await loadASSRenderer()
      const canvas = document.createElement('canvas')
      canvas.width = 640
      canvas.height = 360
      document.body.append(canvas)
      const renderer = new Renderer({ ...assets, canvas, onDemandRender: false, asyncRender: false, subContent: `[Script Info]
ScriptType: v4.00+
PlayResX: 640
PlayResY: 360
[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Liberation Sans,36,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1
[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.00,0:00:10.00,Default,,0,0,0,,{\\pos(320,200)\\c&H00FFFF&}Mnemo ASS` })
      try {
        await new Promise((resolve, reject) => {
          const timeout = setTimeout(() => reject(new Error('字幕 worker 初始化超时')), 15000)
          renderer.addEventListener('ready', () => { clearTimeout(timeout); resolve() }, { once: true })
          renderer.addEventListener('error', () => { clearTimeout(timeout); reject(new Error('字幕 worker 加载失败')) }, { once: true })
        })
        renderer.setCurrentTime(true, 1)
        const deadline = Date.now() + 5000
        let painted = false
        while (Date.now() < deadline && !painted) {
          painted = canvas.getContext('2d').getImageData(0, 0, 640, 360).data.some((value, index) => index % 4 === 3 && value > 0)
          if (!painted) await new Promise(resolve => setTimeout(resolve, 100))
        }
        renderer.timeOffset = 2
        return { painted, timeOffset: renderer.timeOffset, localFonts: assets.useLocalFonts }
      } finally {
        renderer.destroy()
        canvas.remove()
      }
    })
    assert(result.painted, 'ASS 字幕必须实际绘制到画布')
    assert.equal(result.timeOffset, 2)
    assert.equal(result.localFonts, false)
    assert.deepEqual(failures, [])
    console.log('ASS worker、WASM、内置字体、特效绘制与释放通过')
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
