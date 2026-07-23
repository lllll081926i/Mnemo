<script lang="ts">
import { Transition, defineAsyncComponent, h } from 'vue'
import { useAppStore } from './store'
import './assets/global.css'
import './assets/design-tokens.css'
import './assets/fileitem.css'
import './assets/modal.css'
import './assets/antd.css'
import './assets/layout-refactor.css'

const PageMain = defineAsyncComponent(() => import('./layout/PageMain.vue'))
const PageCode = defineAsyncComponent(() => import('./layout/PageCode.vue'))
const PagePdf = defineAsyncComponent(() => import('./layout/PagePdf.vue'))
const PageImage = defineAsyncComponent(() => import('./layout/PageImage.vue'))
const PageVideo = defineAsyncComponent(() => import('./layout/PageVideo.vue'))
const PageMusic = defineAsyncComponent(() => import('./layout/PageMusic.vue'))

export default {
  setup() {
    const appStore = useAppStore()

    return () => {
      let page: any = null
      if (appStore.appPage == 'PageMain') page = PageMain
      else if (appStore.appPage == 'PagePdf') page = PagePdf
      else if (appStore.appPage == 'PageCode') page = PageCode
      else if (appStore.appPage == 'PageImage') page = PageImage
      else if (appStore.appPage == 'PageVideo') page = PageVideo
      else if (appStore.appPage == 'PageMusic') page = PageMusic
      return h(Transition, { name: 'app-page', mode: 'out-in' }, () => {
        if (!page) return h('div', { class: 'desktop-loading-empty' })
        return h(page, { key: appStore.appPage })
      })
    }
  }
}
</script>

<style>
.desktop-loading-empty {
  position: absolute;
  inset: 0;
  background: transparent;
}

.app-page-enter-active {
  transition:
    opacity 180ms ease-out,
    transform 180ms ease-out;
}

.app-page-leave-active {
  transition: opacity 120ms ease-in;
}

.app-page-enter-from {
  opacity: 0;
  transform: translateY(4px);
}

.app-page-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .app-page-enter-active,
  .app-page-leave-active {
    transition: none;
  }
}
</style>
