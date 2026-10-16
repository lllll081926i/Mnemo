let rendererPromise

export function loadASSRenderer() {
  if (!rendererPromise) {
    rendererPromise = Promise.all([
      import('jassub'),
      import('jassub/dist/jassub-worker.js?url'),
      import('jassub/dist/jassub-worker.wasm?url'),
      import('jassub/dist/jassub-worker-modern.wasm?url'),
      import('jassub/dist/default.woff2?url'),
    ]).then(([module, worker, wasm, modern, font]) => ({
      Renderer: module.default,
      assets: { workerUrl: worker.default, wasmUrl: wasm.default, modernWasmUrl: modern.default, fonts: [font.default], availableFonts: { 'liberation sans': font.default }, fallbackFont: 'liberation sans', useLocalFonts: false },
    })).catch(error => {
      rendererPromise = null
      throw error
    })
  }
  return rendererPromise
}
