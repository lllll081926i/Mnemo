<script setup lang="ts">
import { computed, ref } from 'vue'
import message from '../utils/message'
import UserDAL from '../user/userdal'
import useUserStore from '../user/userstore'
import { createWebDavConnection, createWebDavUserToken, getWebDavConnections, removeWebDavConnection, saveWebDavConnection, testWebDavConnection, type WebDavConnectionConfig } from '../utils/webdavClient'

type WebDavForm = Pick<WebDavConnectionConfig, 'name' | 'url' | 'username' | 'password' | 'rootPath'>

const connections = ref<WebDavConnectionConfig[]>(getWebDavConnections())
const editingId = ref('')
const formVisible = ref(connections.value.length === 0)
const saving = ref(false)
const testingId = ref('')
const form = ref<WebDavForm>({ name: '', url: '', username: '', password: '', rootPath: '/' })

const isEditing = computed(() => !!editingId.value)
const useUser = useUserStore()

const refreshConnections = () => {
  connections.value = getWebDavConnections()
}

const resetForm = () => {
  editingId.value = ''
  formVisible.value = connections.value.length === 0
  form.value = { name: '', url: '', username: '', password: '', rootPath: '/' }
}

const startCreate = () => {
  editingId.value = ''
  formVisible.value = true
  form.value = { name: '', url: '', username: '', password: '', rootPath: '/' }
}

const startEdit = (connection: WebDavConnectionConfig) => {
  editingId.value = connection.id
  formVisible.value = true
  form.value = {
    name: connection.name,
    url: connection.url,
    username: connection.username,
    password: connection.password,
    rootPath: connection.rootPath
  }
}

const saveConnection = async () => {
  if (saving.value) return
  if (!form.value.name.trim() || !form.value.url.trim() || !form.value.username.trim() || !form.value.password.trim()) {
    message.error('请填写 WebDAV 名称、地址、用户名和密码')
    return
  }

  saving.value = true
  try {
    const current = connections.value.find((item) => item.id === editingId.value)
    const created = createWebDavConnection(form.value)
    const connection = current ? { ...created, id: current.id, createdAt: current.createdAt } : created
    await testWebDavConnection(connection)
    saveWebDavConnection(connection)
    refreshConnections()
    if (useUser.user_id === `webdav:${connection.id}`) {
      await UserDAL.UserLogin(createWebDavUserToken(connection))
    }
    message.success(current ? 'WebDAV 已更新' : 'WebDAV 已添加')
    resetForm()
  } catch (error: any) {
    message.error(`连接 WebDAV 服务器失败: ${error?.message || '未知错误'}`)
  } finally {
    saving.value = false
  }
}

const testConnection = async (connection: WebDavConnectionConfig) => {
  if (testingId.value) return
  testingId.value = connection.id
  try {
    await testWebDavConnection(connection)
    message.success('WebDAV 连接正常')
  } catch (error: any) {
    message.error(`WebDAV 连接失败: ${error?.message || '未知错误'}`)
  } finally {
    testingId.value = ''
  }
}

const openConnection = async (connection: WebDavConnectionConfig) => {
  try {
    await UserDAL.UserLogin(createWebDavUserToken(connection), true)
  } catch (error: any) {
    message.error(`打开 WebDAV 失败: ${error?.message || '未知错误'}`)
  }
}

const deleteConnection = async (connection: WebDavConnectionConfig) => {
  removeWebDavConnection(connection.id)
  if (useUser.user_id === `webdav:${connection.id}`) {
    await UserDAL.UserLogOff(`webdav:${connection.id}`)
  } else {
    await UserDAL.UserClearFromDB(`webdav:${connection.id}`)
  }
  refreshConnections()
  if (editingId.value === connection.id) resetForm()
  message.success('WebDAV 已删除')
}
</script>

<template>
  <div class="ui-plain-list webdav-settings">
    <div class="ui-plain-row">
      <span class="ui-plain-label">WebDAV 连接</span>
      <div class="ui-plain-control">
        <a-button size="small" type="outline" @click="startCreate">添加 WebDAV</a-button>
      </div>
    </div>

    <template v-if="formVisible">
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="webdav-name">连接名称</label>
        <div class="ui-plain-control"><a-input id="webdav-name" v-model="form.name" class="ui-control-md" placeholder="必填，用于区分连接" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="webdav-url">WebDAV 地址</label>
        <div class="ui-plain-control"><a-input id="webdav-url" v-model="form.url" class="ui-control-lg" placeholder="https://example.com/dav" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="webdav-username">用户名</label>
        <div class="ui-plain-control"><a-input id="webdav-username" v-model="form.username" class="ui-control-md" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="webdav-password">密码</label>
        <div class="ui-plain-control"><a-input-password id="webdav-password" v-model="form.password" class="ui-control-md" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="webdav-root-path">挂载路径</label>
        <div class="ui-plain-control"><a-input id="webdav-root-path" v-model="form.rootPath" class="ui-control-sm" placeholder="/" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <span class="ui-plain-label"></span>
        <div class="ui-plain-control">
          <a-button size="small" type="primary" :loading="saving" @click="saveConnection">{{ isEditing ? '保存' : '连接' }}</a-button>
          <a-button size="small" type="outline" @click="resetForm">取消</a-button>
        </div>
      </div>
    </template>

    <template v-for="connection in connections" :key="connection.id">
      <div v-if="editingId !== connection.id" class="ui-plain-row webdav-connection-row">
        <span class="ui-plain-label">{{ connection.name }}</span>
        <div class="ui-plain-control">
          <span class="webdav-connection-url" :title="connection.url">{{ connection.url }}</span>
          <a-button size="small" type="text" @click="openConnection(connection)">打开</a-button>
          <a-button size="small" type="text" @click="testConnection(connection)">{{ testingId === connection.id ? '测试中' : '测试' }}</a-button>
          <a-button size="small" type="text" @click="startEdit(connection)">编辑</a-button>
          <a-button size="small" type="text" status="danger" @click="deleteConnection(connection)">删除</a-button>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.webdav-connection-url {
  display: inline-block;
  max-width: min(32vw, 260px);
  overflow: hidden;
  color: var(--text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
