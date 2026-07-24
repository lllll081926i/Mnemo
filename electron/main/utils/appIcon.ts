import { app, nativeImage, type NativeImage } from 'electron'
import { existsSync, readFileSync } from 'fs'
import path from 'path'
import { getStaticPath } from './mainfile'

/**
 * 32×32 Mnemo logo (from static/images/tray_32.png).
 * Embedded so dev / packaged tray never falls back to electron.exe icon
 * when file path resolution fails on Windows.
 */
const EMBEDDED_TRAY_PNG_32_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAgISURBVFhHpVdbb1TXGXUv74hWsedc9uWcMTyE+jZjz5wZzzkz4wtgEUVNik0wwdgxvuGAbWLAGGJoCiI0NK1Io6RRVOWhUl/zlB8QVQ4GFEFRSxJVxU4oFJtxK7Wq+pBoVd/eZ+yZsctDu6Wl4/FoZq397W9/a01V1ROW68a3cF639f/BVje+pfJ7n7hMs6HRtmPvWFbsD5bVtGKYTYV1xDSseMFg8YK5huaCQbAIcf1kzYWIQmLFZN4fTZ561+BerJKvbFlW44xtx75mvAWWHYNpN4WIK1h2MyzWApsnSpAsQQIWT8LkSRg8AYO3hM8kLJmGyb2vDebNVvKqZVkNp7giboJpNcIIYVoxWIo8DpvIWQI2S4bwNoLIlIgEDJZAhLWsQQlxfBgicbqM3LZjdbZNRKXkeueK2G4OiT0wAvfAeQqcp9fAQtg8DYunFIqViCghIUQSpkjDcpL1Jbtvep+xlg3kitimXSfXSAVPQ4hWSOGDmS1gVgukCCCkDy4yYKIVtmiFxQkkxIPBPURYEhESwxMwqQrc+3VI3/0d02z6gs58M3LGkmvEUhFn4IgALNKMWOolxFKHlRApsxCShATg0gcTGdhKRCsskYIhPEQ4iUjCoNfc+1NVVfa76qqZZlOBmk2d+Wbka8Q+XJGFZBlIlsbQhTs4ePIWBG+F4D4cmYOUOQiZBZcBmPBhiwwsqogSkQoFUEW8Vc4zW6vorhpm06rq9rDhNiN3RQBX5BCVeVhPNWJP71uYefffmPjxP9C15ypMIw7XaYcj82VCmFwXYeqdayHM+5tte98rCiiUNlwpuVNBLkwP8UQfTry1gpnLKzh+8hFmJ5aRaOgHZ+lQhBYinRy4QyKCEhFpGCKNGuatbi6AJVTDCaHPvJS8VrbBtX0MnPoYr179J+Ze+ytmJh7i0vi/MNnzO0gZIOp0wnU74LjtcJw2iFCELX1YIgNTtCJCAkSZgFiBhoy+57rbiVyfeUjutMO1WhFLjmHs4jLOX3qEC+cf4dyRZfx0oIDR6T+jzhuCywJEozu1CKcd0slDqCpkYQlfCTBkK2pEqlxAccjQPdcdH5Ze5tTOt9HOrBT2HPgths48xOyZ+7gyt4I3Bgs4NbKIH569jba978FlPmqju5QIR4mgKuTBZQ6W9GHKjLqGNSJdIsCKF9RYDe+7Pnvd8bT7bU4HakUOP9ixFwPTn2FsaglTx+7h6onHeGNwGd2v3MRzUwvoGfkYO57uRlR2oFZVoROO0wHptIE5OVhOAJNEOD4islQAWxegm6945aj8bdjmdkKaHtq7ruDo6RUcO3oP02P3cGnkAcYm7uLZ4/PoGZvHwPhttLVdgGSBqoLr7lQCBAlw87CdHEwZbBRAjqYF0Pnr8tOwcYU+ewJVoW/kE0xOPcCJI4s4O7SEucFFdE/dQPfL8zgwPI/Dw5/i4L6PsN3djai7E1F3lxbgtoO7bSUCAkRka6UAbS7rArJwZR5Rtx3CTiOVmsLk9CNMjy9idmQRr730JS4c+hK949exf3QeA4evYbh/AZP9t5GJT0DwrKqAdElAR4WAbKWA5jIBQtBUC9RddqmJ7DT2vfAhTk0t4/TIIs4PLuH1vr/g8oGv0DeygL6hTzDUv4AjLy7g5It3cHDXb8BFACe6EzLaGVageATZjRWg8FAUQI5GxkJ3miaa4BnU1+3H8aOLOD22hHOHl3Dx0H282fsAP993H4OD1zE4cA1jBxcwuf86TvbcxJnnb6Lh6V4wJw/pagHMbYP1346AEkypAHI1MhUSYJrNeKbrbcxNrGLu8CIu9n+FKwce4Jc9D/H23gcY7b+Bkb5rmOi9jhM9NzD7/E28/qPP0ZN+ExGRUkdA5WdOuYAa6ZdfQwoROtWklKUqV+MZbKvtwivDn+HikQIuDy7jF32P8U5vAR/0/B0fdK9i4tAtTB78FDMv3MKre3+Pnzx3Bz979nNc2HUN22ufUVXQ55+HReWXAQwloKICFB5IBHk4+TlZKlVie20X0s0vIx0bQ6ZpHNnGo8g3HENH/QTa6yeQbBxFsmEUXv0IUnUa6R3D8HeMrQlYL394/hsEUKBU2Y0yXUr5OPk5uZkQASw7CYt5sCjxCP2eRgApchAiBy5p3mdhUYllBga977St754EOFkYNANoEpYeQcRuXlUBkiVUjFoX4SsR5Go0Umm204ynCUdXLBrdDTe6W3W7E92lu56GVtnVqyCXmVBARtsx5XaKzzq76RRLMYqSDFko+Tm5GbkaGYsWQjOeHK8TTpQIiygS665fJydQ6RUxapwA1dJf3UKBhCJZhLV8oXMbpVc6Ck8lGPJvW5KIYE0EGQvNdhqvwmlXV4xI9V3XE28zctq9Ig8F1EhfRzJaEZZ8n3J7UQAFSFMURWRUJcjPyVLJ1chYaLYTWRH0ms68nHgjeTUdQbQNERkUQ2lVVbWVrDe5zmtKgEqvOjxSjNKZjoT4qsnI1WiqlYJI1zpddXtpw62TV6v/5fCUzDesCaBlsMRp/aOhGJ/DBMs9FaMoSJiUaMjPla9TtxcRkhZfh7umhqskN2o7UO0EZ8rIi8sQ3qwp099oITpK62daxShKMmSl69DGop9FYt1saufqGaCGzIfK7uS+qZbZs5W8ZYt+QJoi/SuTp+9GeOoxxWcKkJThKEZRkiEv12gtA91tGjAa9Le/Wi39xzVOcDfi5t4zovl4Jd+T1rcot5tmy/fpvv4voM/qq3bu25VfXlz/AZQ4qXmg54SqAAAAAElFTkSuQmCC'

function candidateIconFiles(): string[] {
  const names = ['icon_256x256.png', 'tray_32.png', 'icon_256x256.ico', 'tray_16.png', 'icon_64x64.png']
  const roots: string[] = []
  try {
    roots.push(path.dirname(getStaticPath('icon_256x256.png')))
  } catch {
    /* ignore */
  }
  if (!app.isPackaged) {
    roots.push(path.resolve(process.cwd(), 'static', 'images'))
    roots.push(path.resolve(app.getAppPath(), 'static', 'images'))
  } else if (process.resourcesPath) {
    roots.push(path.join(process.resourcesPath, 'images'))
  }
  const out: string[] = []
  for (const root of roots) {
    for (const name of names) {
      out.push(path.join(root, name))
    }
  }
  return out
}

function tryLoadFromFile(filePath: string): NativeImage | null {
  if (!filePath || !existsSync(filePath)) return null
  try {
    let img = nativeImage.createFromPath(filePath)
    if (!img.isEmpty()) return img
    img = nativeImage.createFromBuffer(readFileSync(filePath))
    if (!img.isEmpty()) return img
  } catch (err) {
    console.error('[mnemo] load icon file failed', filePath, err)
  }
  return null
}

/** App / window icon: prefer large PNG/ICO from disk. */
export function loadAppNativeImage(): NativeImage {
  for (const file of candidateIconFiles()) {
    const img = tryLoadFromFile(file)
    if (img) {
      if (!app.isPackaged) console.log('[mnemo] app icon file', file, img.getSize())
      return img
    }
  }
  const embedded = nativeImage.createFromBuffer(Buffer.from(EMBEDDED_TRAY_PNG_32_BASE64, 'base64'))
  if (!app.isPackaged) console.log('[mnemo] app icon embedded fallback', embedded.getSize(), 'empty=', embedded.isEmpty())
  return embedded
}

/** Tray icon: 16–32px NativeImage (never empty if embed works). */
export function loadTrayNativeImage(): NativeImage {
  // Prefer dedicated small tray assets
  const trayFirst = candidateIconFiles().filter((p) => /tray_32|tray_16|icon_256x256\.png|icon_256x256\.ico/i.test(p))
  for (const file of trayFirst) {
    const img = tryLoadFromFile(file)
    if (!img) continue
    const { width } = img.getSize()
    // Windows tray: 16 or 32 works best; scale large sources down
    const target = width > 32 ? 32 : width
    const sized = width === target ? img : img.resize({ width: target, height: target, quality: 'best' })
    if (!sized.isEmpty()) {
      if (!app.isPackaged) console.log('[mnemo] tray icon file', file, '→', sized.getSize())
      return sized
    }
  }
  const embedded = nativeImage.createFromBuffer(Buffer.from(EMBEDDED_TRAY_PNG_32_BASE64, 'base64'))
  if (!app.isPackaged) console.log('[mnemo] tray icon embedded', embedded.getSize(), 'empty=', embedded.isEmpty())
  return embedded
}
