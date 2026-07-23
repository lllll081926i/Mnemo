import { app } from 'electron'
import { getStaticPath } from './utils/mainfile'
import launch from './launch'

// Set the product identity before any window, tray or single-instance handle is created.
app.setName('Mnemo')
if (process.platform === 'win32') app.setAppUserModelId('com.mnemo.app')
process.title = 'Mnemo'

app.setAboutPanelOptions({
  applicationName: 'Mnemo',
  copyright: 'copyright ©2026 Mnemo',
  website: 'https://github.com/lllll081926i/Mnemo',
  iconPath: getStaticPath('icon_64x64.png'),
  applicationVersion: app.getVersion()
})

const runtime = globalThis as typeof globalThis & { __mnemoLaunchInstance?: launch }
runtime.__mnemoLaunchInstance ??= new launch()
