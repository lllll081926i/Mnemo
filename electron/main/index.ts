import { app } from 'electron'
import { getStaticPath } from './utils/mainfile'
import launch from './launch'

app.setAboutPanelOptions({
  applicationName: 'Mnemo',
  copyright: 'copyright ©2026 GaoZhangMin',
  website: 'https://github.com/gaozhangmin/mnemo',
  iconPath: getStaticPath('icon_64x64.png'),
  applicationVersion: '30'
})

const appLaunch = new launch()
