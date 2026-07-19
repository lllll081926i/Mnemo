import { createPinia } from 'pinia'
import useAppStore from './appstore'
import type { KeyboardState } from './keyboardstore'
import useKeyboardStore from './keyboardstore'
import type { MouseState } from './mousestore'
import useMouseStore from './mousestore'
import type { ModalState } from './modalstore'
import useModalStore from './modalstore'
import type { WinState } from './winstore'
import useWinStore from './winstore'
import useSettingStore from '../setting/settingstore'
import type { ITokenInfo } from '../user/userstore'
import useUserStore from '../user/userstore'
import usePanTreeStore from '../pan/pantreestore'
import usePanFileStore from '../pan/panfilestore'

import type { IOtherShareLinkModel } from '../share/share/OtherShareStore'
import useOtherShareStore from '../share/share/OtherShareStore'
import useMyShareStore from '../share/share/MyShareStore'

import useUploadingStore from '../down/UploadingStore'
import useUploadedStore from '../down/UploadedStore'
import useDownedStore from '../down/DownedStore'
import useDowningStore from '../down/DowningStore'

import type { AsyncModel } from './footstore'
import useFootStore from './footstore'

const pinia = createPinia()
export {
  useAppStore,
  useSettingStore,
  useModalStore,
  ModalState,
  useWinStore,
  WinState,
  useMouseStore,
  useKeyboardStore,
  KeyboardState,
  MouseState,
  useUserStore,
  ITokenInfo,
  usePanTreeStore,
  usePanFileStore,
  IOtherShareLinkModel,
  useMyShareStore,
  useOtherShareStore,
  useFootStore,
  AsyncModel,
  useUploadingStore,
  useUploadedStore,
  useDowningStore,
  useDownedStore
}
export default pinia
