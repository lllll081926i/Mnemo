<script lang="ts">
import { defineAsyncComponent, h } from 'vue'
import { useAppStore } from './store'
import PageMain from './layout/PageMain.vue'
import './assets/global.css'
import './assets/design-tokens.css'
import './assets/fileitem.css'
import './assets/antd.css'
import './assets/layout-refactor.css'

const PageVideoXBTVue = defineAsyncComponent(() => import('./layout/PageVideoXBT.vue'))
const PageCode = defineAsyncComponent(() => import('./layout/PageCode.vue'))
const PageOffice = defineAsyncComponent(() => import('./layout/PageOffice.vue'))
const PagePdf = defineAsyncComponent(() => import('./layout/PagePdf.vue'))
const PageDocx = defineAsyncComponent(() => import('./layout/PageDocx.vue'))
const PageSheet = defineAsyncComponent(() => import('./layout/PageSheet.vue'))
const PageImage = defineAsyncComponent(() => import('./layout/PageImage.vue'))
const PageVideo = defineAsyncComponent(() => import('./layout/PageVideo.vue'))
const PageMusic = defineAsyncComponent(() => import('./layout/PageMusic.vue'))
const PageWorker = defineAsyncComponent(() => import('./layout/PageWorker.vue'))

export default {
  setup() {
    const appStore = useAppStore()

    return () => {
      if (appStore.appPage == 'PageMain') return h(PageMain)
      if (appStore.appPage == 'PageOffice') return h(PageOffice)
      if (appStore.appPage == 'PagePdf') return h(PagePdf)
      if (appStore.appPage == 'PageDocx') return h(PageDocx)
      if (appStore.appPage == 'PageSheet') return h(PageSheet)
      if (appStore.appPage == 'PageVideoXBT') return h(PageVideoXBTVue)
      if (appStore.appPage == 'PageCode') return h(PageCode)
      if (appStore.appPage == 'PageImage') return h(PageImage)
      if (appStore.appPage == 'PageVideo') return h(PageVideo)
      if (appStore.appPage == 'PageMusic') return h(PageMusic)
      if (appStore.appPage == 'PageWorker') return h(PageWorker)
      return h('div', { class: 'desktop-loading-empty' })
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
</style>
