<script lang='ts'>
import { defineComponent } from 'vue'
import { useModalStore } from '../store'
import RenameModal from '../pan/topbtns/RenameModal.vue'
import RenameMultiModal from '../pan/topbtns/RenameMultiModal.vue'
import CreatNewDirModal from '../pan/topbtns/CreatNewDirModal.vue'
import SelectPanDirModal from '../pan/topbtns/SelectPanDirModal.vue'
import CreatNewShareLinkModal from '../share/share/CreatNewShareLinkModal.vue'
import ShuXingModal from '../pan/topbtns/ShuXingModal.vue'
import UploadModal from '../pan/topbtns/UploadModal.vue'
import DownloadModal from '../pan/topbtns/DownloadModal.vue'
import SelectVideoQualityModal from '../pan/topbtns/SelectVideoQualityModal.vue'
import CloudOfflineDownloadModal from '../down/CloudOfflineDownloadModal.vue'
import PasswordModal from '../pan/topbtns/PasswordModal.vue'

export default defineComponent({
  components: {
    SelectVideoQualityModal,
    CloudOfflineDownloadModal,
    PasswordModal,
    RenameModal,
    RenameMultiModal,
    CreatNewDirModal,
    CreatNewShareLinkModal,
    SelectPanDirModal,
    ShuXingModal,
    UploadModal,
    DownloadModal
  },
  setup() {
    const modalStore = useModalStore()
    return { modalStore }
  }
})
</script>

<template>
  <CreatNewDirModal :visible="modalStore.modalName == 'creatdir'"
                    :dirtype="modalStore.modalData.dirtype || ''"
                    :encType="modalStore.modalData.encType || ''"
                    :parentdirid="modalStore.modalData.parentdirid || ''"
                    :callback='modalStore.modalData.callback' />
  <CreatNewShareLinkModal :visible="modalStore.modalName == 'creatshare'"
                           :sharetype="modalStore.modalData.sharetype || ''"
                           :filelist='modalStore.modalData.filelist || []' />

  <RenameModal :visible="modalStore.modalName == 'rename'"
               :istree='modalStore.modalData.istree || false'
               :ispic='modalStore.modalData.ispic || false' />
  <RenameMultiModal :visible="modalStore.modalName == 'renamemulti'" :istree='modalStore.modalData.istree || false' />
  <ShuXingModal :visible="modalStore.modalName == 'shuxing'"
                :istree='modalStore.modalData.istree || false'
                :inputselectType="modalStore.modalData.inputselectType || ''"
                :ispic='modalStore.modalData.ispic || false' />

  <UploadModal :visible="modalStore.modalName == 'upload'"
               :file_id="modalStore.modalData.file_id || ''"
               :filelist='modalStore.modalData.filelist || []'
               :ispic='modalStore.modalData.ispic || false'
               :encType="modalStore.modalData.encType || ''" />
  <DownloadModal :visible="modalStore.modalName == 'download'" :istree='modalStore.modalData.istree || false' />

  <SelectPanDirModal :visible="modalStore.modalName == 'selectpandir'"
                     :selecttype="modalStore.modalData.selecttype || ''"
                     :selectid="modalStore.modalData.selectid || ''"
                     :category='modalStore.modalData.category'
                     :extFilter='modalStore.modalData.extFilter'
                     :callback='modalStore.modalData.callback' />

  <SelectVideoQualityModal :visible="modalStore.modalName == 'selectvideoquality'"
                           :file-info="modalStore.modalData.fileInfo || {}"
                           :quality-data="modalStore.modalData.qualityData || {}"
                           :callback="modalStore.modalData.callback" />

  <CloudOfflineDownloadModal :visible="modalStore.modalName == 'cloudoffline'"
                             :offline-form="modalStore.modalData.offlineForm" />
  <PasswordModal :visible="modalStore.modalName == 'showpassword'"
                 :opt-type="modalStore.modalData.optType || ''"
                 :callback="modalStore.modalData.callback" />
</template>
