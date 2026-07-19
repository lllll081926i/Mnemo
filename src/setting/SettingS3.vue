<script setup lang="ts">
import { computed, ref } from 'vue'
import message from '../utils/message'
import UserDAL from '../user/userdal'
import useUserStore from '../user/userstore'
import { createS3Connection, createS3UserToken, getS3Connections, removeS3Connection, saveS3Connection, testS3Connection, type S3ConnectionConfig, type S3ConnectionInput } from '../utils/s3Client'

const connections = ref<S3ConnectionConfig[]>(getS3Connections())
const editingId = ref('')
const formVisible = ref(connections.value.length === 0)
const saving = ref(false)
const testingId = ref('')
const useUser = useUserStore()
const emptyForm = (): S3ConnectionInput => ({ name: '', endpoint: '', region: 'us-east-1', accessKeyId: '', secretAccessKey: '', sessionToken: '', bucket: '', rootPrefix: '', forcePathStyle: true })
const form = ref<S3ConnectionInput>(emptyForm())
const isEditing = computed(() => !!editingId.value)

const refreshConnections = () => {
  connections.value = getS3Connections()
}

const resetForm = () => {
  editingId.value = ''
  formVisible.value = connections.value.length === 0
  form.value = emptyForm()
}

const startCreate = () => {
  editingId.value = ''
  formVisible.value = true
  form.value = emptyForm()
}

const startEdit = (connection: S3ConnectionConfig) => {
  editingId.value = connection.id
  formVisible.value = true
  form.value = {
    name: connection.name,
    endpoint: connection.endpoint,
    region: connection.region,
    accessKeyId: connection.accessKeyId,
    secretAccessKey: connection.secretAccessKey,
    sessionToken: connection.sessionToken,
    bucket: connection.bucket,
    rootPrefix: connection.rootPrefix,
    forcePathStyle: connection.forcePathStyle
  }
}

const saveConnection = async () => {
  if (saving.value) return
  if (!form.value.name.trim() || !form.value.bucket.trim() || !form.value.accessKeyId.trim() || !form.value.secretAccessKey.trim()) {
    message.error('请填写 S3 名称、Bucket、Access Key 和 Secret Key')
    return
  }
  saving.value = true
  try {
    const current = connections.value.find((item) => item.id === editingId.value)
    const created = createS3Connection(form.value)
    const connection = current ? { ...created, id: current.id, createdAt: current.createdAt } : created
    await testS3Connection(connection)
    saveS3Connection(connection)
    refreshConnections()
    if (useUser.user_id === `s3:${connection.id}`) await UserDAL.UserLogin(createS3UserToken(connection))
    message.success(current ? 'S3 已更新' : 'S3 已添加')
    resetForm()
  } catch (error: any) {
    message.error(`连接 S3 失败: ${error?.message || '未知错误'}`)
  } finally {
    saving.value = false
  }
}

const testConnection = async (connection: S3ConnectionConfig) => {
  if (testingId.value) return
  testingId.value = connection.id
  try {
    await testS3Connection(connection)
    message.success('S3 连接正常')
  } catch (error: any) {
    message.error(`S3 连接失败: ${error?.message || '未知错误'}`)
  } finally {
    testingId.value = ''
  }
}

const openConnection = async (connection: S3ConnectionConfig) => {
  try {
    await UserDAL.UserLogin(createS3UserToken(connection), true)
  } catch (error: any) {
    message.error(`打开 S3 失败: ${error?.message || '未知错误'}`)
  }
}

const deleteConnection = async (connection: S3ConnectionConfig) => {
  removeS3Connection(connection.id)
  if (useUser.user_id === `s3:${connection.id}`) await UserDAL.UserLogOff(`s3:${connection.id}`)
  else await UserDAL.UserClearFromDB(`s3:${connection.id}`)
  refreshConnections()
  if (editingId.value === connection.id) resetForm()
  message.success('S3 已删除')
}
</script>

<template>
  <div class="ui-plain-list s3-settings">
    <div class="ui-plain-row">
      <span class="ui-plain-label">S3 连接</span>
      <div class="ui-plain-control"><a-button size="small" type="outline" @click="startCreate">添加 S3</a-button></div>
    </div>

    <template v-if="formVisible">
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="s3-name">连接名称</label>
        <div class="ui-plain-control"><a-input id="s3-name" v-model="form.name" class="ui-control-md" placeholder="必填且不可重复" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="s3-endpoint">Endpoint</label>
        <div class="ui-plain-control"><a-input id="s3-endpoint" v-model="form.endpoint" class="ui-control-lg" placeholder="AWS 官方可留空" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <span class="ui-plain-label">区域与路径样式</span>
        <div class="ui-plain-control">
          <a-input v-model="form.region" class="ui-control-sm" placeholder="Region" allow-clear />
          <a-switch v-model="form.forcePathStyle" size="small" />
          <span>路径样式</span>
        </div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="s3-bucket">Bucket</label>
        <div class="ui-plain-control"><a-input id="s3-bucket" v-model="form.bucket" class="ui-control-md" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="s3-access-key">Access Key ID</label>
        <div class="ui-plain-control"><a-input id="s3-access-key" v-model="form.accessKeyId" class="ui-control-md" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="s3-secret-key">Secret Access Key</label>
        <div class="ui-plain-control"><a-input-password id="s3-secret-key" v-model="form.secretAccessKey" class="ui-control-md" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="s3-session-token">Session Token</label>
        <div class="ui-plain-control"><a-input-password id="s3-session-token" v-model="form.sessionToken" class="ui-control-lg" placeholder="可选" allow-clear /></div>
      </div>
      <div class="ui-plain-row">
        <label class="ui-plain-label" for="s3-root-prefix">根前缀</label>
        <div class="ui-plain-control"><a-input id="s3-root-prefix" v-model="form.rootPrefix" class="ui-control-md" placeholder="可选" allow-clear /></div>
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
      <div v-if="editingId !== connection.id" class="ui-plain-row">
        <span class="ui-plain-label">{{ connection.name }}</span>
        <div class="ui-plain-control">
          <span class="s3-target" :title="`${connection.endpoint || 'AWS'} / ${connection.bucket}`">{{ connection.endpoint || 'AWS' }} / {{ connection.bucket }}</span>
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
.s3-target {
  display: inline-block;
  max-width: min(32vw, 280px);
  overflow: hidden;
  color: var(--text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
