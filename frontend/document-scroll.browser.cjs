// 使用本地生成的文档核验连续滚动，不连接真实网盘。
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright')
const assert = require('node:assert/strict')
const JSZip = require('jszip')
const XLSX = require('xlsx')
const fs = require('node:fs')
const path = require('node:path')
const output = process.env.QA_OUTPUT || path.join(require('node:os').tmpdir(), 'mnemo-document-scroll-qa')
const base = process.env.QA_URL || 'http://127.0.0.1:5173'
fs.mkdirSync(output, { recursive: true })

function pdfFixture(count) {
  const objects = ['<< /Type /Catalog /Pages 2 0 R >>', '', '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>']
  const kids = []
  for (let index = 1; index <= count; index++) {
    kids.push(`${objects.length + 1} 0 R`)
    const stream = `0.12 0.35 0.7 rg 40 580 530 100 re f BT /F1 28 Tf 40 730 Td (Continuous page ${index}) Tj ET`
    objects.push(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents ${objects.length + 2} 0 R >>`)
    objects.push(`<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`)
  }
  objects[1] = `<< /Type /Pages /Kids [${kids.join(' ')}] /Count ${count} >>`
  let source = '%PDF-1.4\n'
  const offsets = [0]
  objects.forEach((object, index) => { offsets.push(Buffer.byteLength(source)); source += `${index + 1} 0 obj\n${object}\nendobj\n` })
  const xref = Buffer.byteLength(source)
  source += `xref\n0 ${offsets.length}\n0000000000 65535 f \n` + offsets.slice(1).map(offset => `${String(offset).padStart(10, '0')} 00000 n \n`).join('')
  source += `trailer\n<< /Size ${offsets.length} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF`
  return Buffer.from(source)
}

async function officeFixture(kind, count) {
  const zip = new JSZip()
  const relationships = 'http://schemas.openxmlformats.org/package/2006/relationships'
  const office = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
  if (kind === 'pptx') {
    zip.file('[Content_Types].xml', `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>${Array.from({ length: count }, (_, index) => `<Override PartName="/ppt/slides/slide${index + 1}.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`).join('')}</Types>`)
    zip.file('_rels/.rels', `<Relationships xmlns="${relationships}"><Relationship Id="rId1" Type="${office}/officeDocument" Target="ppt/presentation.xml"/></Relationships>`)
    zip.file('ppt/presentation.xml', `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="${office}"><p:sldIdLst>${Array.from({ length: count }, (_, index) => `<p:sldId id="${256 + index}" r:id="rId${index + 1}"/>`).join('')}</p:sldIdLst><p:sldSz cx="9144000" cy="5143500"/></p:presentation>`)
    zip.file('ppt/_rels/presentation.xml.rels', `<Relationships xmlns="${relationships}">${Array.from({ length: count }, (_, index) => `<Relationship Id="rId${index + 1}" Type="${office}/slide" Target="slides/slide${index + 1}.xml"/>`).join('')}</Relationships>`)
    for (let index = 1; index <= count; index++) zip.file(`ppt/slides/slide${index}.xml`, `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/><p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="400000" y="400000"/><a:ext cx="8200000" cy="4000000"/></a:xfrm><a:prstGeom prst="rect"/><a:solidFill><a:srgbClr val="E7EFFB"/></a:solidFill></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr sz="3200"/><a:t>Continuous slide ${index}</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`)
  } else {
    zip.file('[Content_Types].xml', '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>')
    zip.file('_rels/.rels', `<Relationships xmlns="${relationships}"><Relationship Id="rId1" Type="${office}/officeDocument" Target="word/document.xml"/></Relationships>`)
    zip.file('word/document.xml', `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>${Array.from({ length: count }, (_, index) => `<w:p><w:r><w:t>Continuous Word page ${index + 1}</w:t>${index + 1 < count ? '<w:br w:type="page"/>' : ''}</w:r></w:p>`).join('')}<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`)
  }
  return zip.generateAsync({ type: 'nodebuffer' })
}

async function main() {
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  const page = await browser.newPage({ viewport: { width: 1100, height: 780 } })
  const errors = []
  page.on('pageerror', error => errors.push(String(error)))
  page.on('console', message => { if (message.type() === 'error') console.error(message.text()) })
  await page.route('**/document-scroll-check', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><html><head><link rel="stylesheet" href="/src/styles/design-tokens.css"><link rel="stylesheet" href="/src/styles/main.css"><style>html,body{margin:0;height:100%}#check{height:100vh;display:flex;overflow:hidden}</style></head><body><div id="check"></div></body></html>' }))
  await page.goto(`${base}/document-scroll-check`)
  async function mount(kind, bytes) {
    await page.evaluate(async ({ kind, bytes }) => {
      window.documentCheck?.unmount()
      const { createApp } = await import('/node_modules/.vite/deps/vue.js')
      const { default: component } = await import(`/src/components/${kind}Preview.vue`)
      const props = kind === 'Pdf' ? { url: URL.createObjectURL(new Blob([new Uint8Array(bytes)], { type: 'application/pdf' })) } : { bytes: new Uint8Array(bytes) }
      window.documentCheck = createApp(component, { ...props, file: { name: 'continuous-check' } })
      window.documentCheck.mount('#check')
    }, { kind, bytes: [...bytes] })
    await page.getByText(/正在解析|正在读取|正在排版/).waitFor({ state: 'hidden' })
    assert.equal(await page.getByRole('button', { name: /上一页|下一页|上一张|下一张/ }).count(), 0, `${kind} 不应再显示翻页按钮`)
    assert.equal(await page.getByRole('alert').count(), 0, `${kind} 解析失败：${await page.locator('#check').innerText()}`)
  }
  try {
    await mount('Pdf', pdfFixture(30))
    await page.waitForSelector('.pdfViewer .page canvas')
    assert.equal(await page.locator('.pdfViewer .page').count(), 30)
    await page.locator('.pdf-stage').hover()
    await page.mouse.wheel(0, 1500)
    await page.waitForFunction(() => document.querySelector('.pdf-stage').scrollTop > 500)
    await page.locator('.pdf-stage').evaluate(element => { element.scrollTop = element.scrollHeight })
    await page.waitForFunction(() => document.querySelector('.pdfViewer .page[data-page-number="30"] canvas'))
    await page.waitForFunction(() => document.querySelector('.document-controls').textContent.includes('30 / 30'))
    assert(await page.locator('.pdfViewer .page canvas').count() < 15, 'PDF 应按视口回收画布')
    await page.getByRole('button', { name: '＋', exact: true }).click()
    assert(await page.locator('.pdf-stage').evaluate(element => element.scrollTop > 1000), '缩放不能跳回第一页')
    await page.setViewportSize({ width: 850, height: 640 })
    await page.getByRole('button', { name: '适合宽度', exact: true }).click()
    await page.locator('.pdfViewer .page[data-page-number="30"]').evaluate(element => element.scrollIntoView({ block: 'start' }))
    await page.locator('.pdfViewer .page[data-page-number="30"] .textLayer span').first().waitFor()
    await page.screenshot({ path: path.join(output, 'pdf-continuous.png') })

    await mount('Pptx', await officeFixture('pptx', 12))
    await page.waitForSelector('[data-slide-index="0"]')
    assert.equal(await page.locator('[data-slide-index]').count(), 12)
    await page.locator('.pptx-stage').hover()
    await page.mouse.wheel(0, 1300)
    await page.waitForFunction(() => document.querySelector('.pptx-stage').scrollTop > 500)
    await page.locator('.pptx-stage').evaluate(element => { element.scrollTop = element.scrollHeight })
    await page.waitForFunction(() => document.querySelector('[data-slide-index="11"]').textContent.includes('Continuous slide 12'))
    await page.waitForFunction(() => document.querySelector('.pptx-controls').textContent.includes('12 / 12'))
    await page.getByRole('button', { name: '＋', exact: true }).click()
    assert(await page.locator('.pptx-stage').evaluate(element => element.scrollTop > 1000), 'PPT 缩放不能回到第一张')
    await page.setViewportSize({ width: 1100, height: 780 })
    await page.waitForFunction(() => document.querySelector('[data-slide-index="11"]')?.getBoundingClientRect().width > 900)
    assert(await page.locator('.pptx-stage').evaluate(element => element.scrollTop > 1000), '调整窗口不能回到第一张')
    await page.screenshot({ path: path.join(output, 'pptx-continuous.png') })

    const workbook = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(workbook, XLSX.utils.aoa_to_sheet(Array.from({ length: 4000 }, (_, index) => [`Continuous row ${index + 1}`, index + 1, `Value ${index + 1}`])), '长表格')
    XLSX.utils.book_append_sheet(workbook, XLSX.utils.aoa_to_sheet([['Second sheet']]), '第二张表')
    await mount('Spreadsheet', XLSX.write(workbook, { type: 'buffer', bookType: 'xlsx' }))
    await page.locator('.spreadsheet-stage').hover()
    await page.mouse.wheel(0, 4200)
    await page.waitForFunction(() => Number(document.querySelector('tbody tr[aria-rowindex] th').textContent) > 100)
    await page.locator('.spreadsheet-stage').evaluate(element => { element.scrollTop = element.scrollHeight })
    await page.getByRole('cell', { name: 'Continuous row 4000', exact: true }).waitFor()
    assert(await page.locator('tbody tr[aria-rowindex]').count() < 100, '长表格不能一次渲染全部行')
    assert(await page.locator('thead th').first().evaluate(element => Math.abs(element.getBoundingClientRect().top - document.querySelector('.spreadsheet-stage').getBoundingClientRect().top) < 2), '表头应保持可见')
    await page.screenshot({ path: path.join(output, 'xlsx-continuous.png') })
    await page.getByRole('combobox').click()
    await page.getByRole('option', { name: '第二张表', exact: true }).click()
    await page.getByRole('cell', { name: 'Second sheet', exact: true }).waitFor()
    assert.equal(await page.locator('.spreadsheet-stage').evaluate(element => element.scrollTop), 0)

    await mount('Docx', await officeFixture('docx', 3))
    assert.equal(await page.locator('section.docx').count(), 3)
    await page.locator('.docx-stage').evaluate(element => { element.scrollTop = element.scrollHeight })
    assert(await page.locator('.docx-stage').evaluate(element => element.scrollTop > 1000))
    await page.screenshot({ path: path.join(output, 'docx-continuous.png') })
    await page.evaluate(() => window.documentCheck.unmount())
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    assert.deepEqual(errors, [], '连续阅读与关闭预览不能产生未处理异常')
    console.log(`连续滚动验收通过：PDF 30 页、PPT 12 张、表格 4000 行、Word 3 页。截图：${output}`)
  } catch (error) {
    console.error('浏览器状态：', await page.locator('#check').innerText(), errors)
    await page.screenshot({ path: path.join(output, 'failure.png') })
    throw error
  } finally { await browser.close() }
}

main().catch(error => { console.error(error); process.exitCode = 1 })
