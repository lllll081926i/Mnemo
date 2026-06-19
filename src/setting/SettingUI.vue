<script setup lang='ts'>
import { computed, onMounted, ref } from 'vue'
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import LimitReachedModal from './LimitReachedModal.vue'
import ServerHttp from '../aliapi/server'
import os from 'os'
import { getAppNewPath, getResourcesPath } from '../utils/electronhelper'
import { existsSync, readFileSync } from 'fs'
import { getPkgVersion } from '../utils/utils'
import { modalUpdateLog } from '../utils/modal'
import fs from 'node:fs'
import message from '../utils/message'
import { Sleep } from '../utils/format'

const platform = window.platform
const settingStore = useSettingStore()

const isPro = ref(localStorage.getItem('app_user_pro') === '1')
const isLoggedIn = ref(localStorage.getItem('app_user_authed') === '1')
const showUpgradeModal = ref(false)

onMounted(() => {
  if (localStorage.getItem('boxplayer_show_pricing') === '1') {
    localStorage.removeItem('boxplayer_show_pricing')
    if (!isPro.value) showUpgradeModal.value = true
  }
})

function handleLogout() {
  localStorage.removeItem('app_user_email')
  localStorage.removeItem('app_user_authed')
  isLoggedIn.value = false
  message.success('已退出登录')
}
const cb = (val: any) => {
  settingStore.updateStore(val)
}

const getAppVersion = computed(() => {
  const pkgVersion = getPkgVersion()
  if (os.platform() === 'linux') {
    return pkgVersion
  }
  return pkgVersion
  // let appVersion = ''
  // const localVersion = getResourcesPath('localVersion')
  // if (localVersion && existsSync(localVersion)) {
  //   appVersion = readFileSync(localVersion, 'utf-8')
  // } else {
  //   appVersion = pkgVersion
  // }
  // return appVersion
})

const verLoading = ref(false)
const handleCheckVer = () => {
  verLoading.value = true
  setTimeout(() => {
    ServerHttp.CheckUpgrade()
    verLoading.value = false
  }, 200)
}
const handleUpdateLog = () => {
  modalUpdateLog()
}

const handleImportAsar = () => {
  window.WebShowOpenDialogSync({
    title: '选择需要导入的Asar文件',
    buttonLabel: '导入更新文件',
    filters: [{ name: 'app.asar', extensions: ['asar'] }],
    properties: ['openFile', 'showHiddenFiles', 'noResolveAliases', 'treatPackageAsDirectory', 'dontAddToRecent']
  }, async (files: string[] | undefined) => {
    if (files && files.length > 0) {
      // 导入到app.new
      await fs.promises.cp(files[0], getAppNewPath())
      message.info('导入更新文件成功，重新打开应用...', 0)
      await Sleep(1000)
      window.WebToElectron({ cmd: 'relaunch' })
    }
  })
}
</script>

<template>
  <div class='settingcard'>
    <div class='settings-app-hero'>
      <div class='settings-app-badge'>Application</div>
      <div class='appver'>BoxPlayer {{ getAppVersion }} <span class="appver-badge" :class="{ pro: isPro }">{{ isPro ? 'PRO' : '开源版' }}</span></div>
      <div class="appver-actions">
        <span v-if="isLoggedIn" class="appver-email">{{ localStorage.getItem('app_user_email') || '' }}</span>
        <button v-if="!isLoggedIn" class="appver-login" @click="document.getElementById('SettingAppAccount')?.scrollIntoView({behavior:'smooth'})">登录</button>
        <button v-if="isLoggedIn" class="appver-logout" @click="handleLogout">退出</button>
        <button v-if="!isPro" class="appver-upgrade" @click="showUpgradeModal = true">升级专业版</button>
      </div>
      <div class='settings-app-subtitle'>统一配置桌面外观、启动行为、更新策略与系统集成体验</div>
    </div>
    <div class='settings-app-actions'>
      <a-button type='outline' status='success' size='small' tabindex='-1' @click='handleUpdateLog'>
        更新日志
      </a-button>
      <a-button style='margin-left: 10px' type='outline' size='small' tabindex='-1' :loading='verLoading'
                @click='handleCheckVer'>
        检查更新
      </a-button>
      <a-button style='margin-left: 10px'
                v-if='platform !== "linux"'
                status='warning' type='outline' size='small' tabindex='-1'
                @click='handleImportAsar'>
        手动导入
      </a-button>
    </div>
    <div class='settingspace'></div>
    <div class='settingspace'></div>
    <div class='settinghead'>界面颜色</div>
    <div class='settingrow'>
      <a-radio-group type='button' tabindex='-1' :model-value='settingStore.uiTheme'
                     @update:model-value='cb({ uiTheme: $event })'>
        <a-radio tabindex='-1' value='system'>跟随系统</a-radio>
        <a-radio tabindex='-1' value='light'>浅色模式</a-radio>
        <a-radio tabindex='-1' value='dark'>深色模式</a-radio>
      </a-radio-group>
    </div>
    <div class='settingspace'></div>
    <div class='settinghead'>默认启动 Tab</div>
    <div class='settingrow'>
      <a-radio-group
        type='button'
        tabindex='-1'
        :model-value='settingStore.uiDefaultTab'
        @update:model-value='cb({ uiDefaultTab: $event })'
      >
        <a-radio tabindex='-1' value='pan'>网盘</a-radio>
        <a-radio tabindex='-1' value='media-server'>媒体服务器</a-radio>
        <a-radio tabindex='-1' value='media'>媒体库</a-radio>
      </a-radio-group>
    </div>
    <template v-if="['win32', 'darwin'].includes(platform)">
      <div class='settingspace'></div>
      <div class='settinghead'>开机自启设置</div>
      <div class='settingrow'>
        <MySwitch :value='settingStore.uiLaunchStart' @update:value='cb({ uiLaunchStart: $event })'>
          开机时自动启动
        </MySwitch>
      </div>
      <div class='settingrow' v-if="settingStore.uiLaunchStart">
        <MySwitch :value='settingStore.uiLaunchStartShow'
                  @update:value='cb({ uiLaunchStartShow: $event })'>
          自动启动后显示主窗口
        </MySwitch>
      </div>
    </template>
    <div class='settingspace'></div>
    <div class='settinghead'>检查更新设置</div>
    <div class='settingrow'>
      <MySwitch :value='settingStore.uiLaunchAutoCheckUpdate'
                @update:value='cb({ uiLaunchAutoCheckUpdate: $event })'>
        启动时检查更新
      </MySwitch>
    </div>
    <div class='settingspace'></div>
    <div class='settinghead'>自动签到设置</div>
    <div class='settingrow'>
      <MySwitch :value='settingStore.uiLaunchAutoSign' @update:value='cb({ uiLaunchAutoSign: $event })'>
        启动时自动签到
      </MySwitch>
    </div>
    <div class='settingspace'></div>
    <div class='settinghead'>关闭时彻底退出</div>
    <div class='settingrow'>
      <MySwitch :value='settingStore.uiExitOnClose' @update:value='cb({ uiExitOnClose: $event })'>
        关闭窗口时彻底退出小白羊
      </MySwitch>
      <a-popover position='right'>
        <IconFont name="iconbulb" />
        <template #content>
          <div>
            默认：<span class='opred'>关闭</span>
            <hr />
            默认是点击窗口上的关闭按钮时<br />最小化到托盘，继续上传/下载<br /><br />开启此设置后直接彻底退出小白羊程序
          </div>
        </template>
      </a-popover>
    </div>
    <div class='settingspace'></div>
    <div class='settinghead'>软件更新代理</div>
    <div class='settingrow'>
      <MySwitch :value='settingStore.uiUpdateProxyEnable' @update:value='cb({ uiUpdateProxyEnable: $event })'>
        开启软件更新代理
      </MySwitch>
      <div class='settingrow' v-if="settingStore.uiUpdateProxyEnable">
        <a-input v-model.trim='settingStore.uiUpdateProxyUrl'
                 allow-clear
                 :style="{ width: '280px' }"
                 placeholder='软件更新代理'
                 @update:model-value='cb({ uiUpdateProxyUrl: $event })' />
      </div>
    </div>
  </div>
  <LimitReachedModal :visible="showUpgradeModal" @update:visible="showUpgradeModal = $event" />
</template>

<style scoped>
.settings-app-hero {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 14px;
}

.settings-app-badge {
  display: inline-flex;
  align-self: flex-start;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(88, 130, 255, 0.12);
  color: var(--color-primary-6);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.appver {
  font-weight: 600;
  font-size: 28px;
  line-height: 1.4;
}

.settings-app-subtitle {
  max-width: 520px;
  color: var(--color-text-2);
  font-size: 14px;
  line-height: 1.7;
}

.settings-app-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.settings-app-actions :deep(.arco-btn) {
  margin-left: 0 !important;
}

:global(html.dark) .settings-app-badge {
  background: rgba(120, 160, 255, 0.2);
  color: #dbe6ff;
}

@media (max-width: 900px) {
  .appver {
    font-size: 24px;
  }
}

.appver-badge{display:inline-block;margin-left:8px;padding:1px 8px;font-size:10px;font-weight:700;color:var(--color-text-3);background:var(--color-fill-2);border:1px solid var(--color-border);border-radius:5px;vertical-align:middle}
.appver-badge.pro{color:#b45309;background:rgba(245,158,11,.15);border-color:rgba(245,158,11,.35)}
.appver-actions{display:flex;align-items:center;gap:8px;margin-top:8px}
.appver-email{font-size:12px;color:var(--color-text-3)}
.appver-login{padding:3px 10px;font-size:11px;color:rgb(var(--primary-6));background:transparent;border:1px solid rgb(var(--primary-6));border-radius:5px;cursor:pointer;font-family:inherit}
.appver-login:hover{background:rgba(var(--primary-6),.08)}
.appver-logout{padding:3px 10px;font-size:11px;color:var(--color-text-4);background:transparent;border:1px solid var(--color-border);border-radius:5px;cursor:pointer;font-family:inherit}
.appver-logout:hover{color:rgb(var(--danger-6));border-color:rgb(var(--danger-6))}
.appver-upgrade{padding:3px 12px;font-size:11px;font-weight:600;color:#fff;background:linear-gradient(135deg,#f59e0b,#eab308);border:0;border-radius:6px;cursor:pointer;font-family:inherit}
.appver-upgrade:hover{opacity:.9}
</style>
