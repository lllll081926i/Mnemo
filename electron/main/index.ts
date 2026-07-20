import { app } from 'electron'
import { getStaticPath } from './utils/mainfile'
import launch from './launch'

app.setAboutPanelOptions({
  applicationName: 'Mnemo',
  copyright: 'copyright ©2026 Mnemo',
  website: 'https://github.com/lllll081926i/Mnemo',
  iconPath: getStaticPath('icon_64x64.png'),
  applicationVersion: app.getVersion()
})

const runtime = globalThis as typeof globalThis & { __mnemoLaunchInstance?: launch }
runtime.__mnemoLaunchInstance ??= new launch()
