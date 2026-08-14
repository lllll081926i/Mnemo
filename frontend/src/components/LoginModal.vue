<script setup>
import { ref } from 'vue'
import { login, saveMounted } from '../api'

const props = defineProps({ providers: Array })
const emit = defineEmits(['close', 'toast'])

const provider = ref('pikpak')
const form = ref({})
const mountedForm = ref({ name: '', endpoint: '', username: '', password: '', bucket: '', region: '', basePath: '' })
const busy = ref(false)

const loginFields = () => {
  const p = props.providers.find((x) => x.ID === provider.value)
  return (p && p.Login && p.Login.Fields) || []
}

const isMounted = () => provider.value === 'webdav' || provider.value === 's3'

async function submit() {
  busy.value = true
  try {
    if (isMounted()) {
      await saveMounted(provider.value, mountedForm.value)
    } else {
      await login(provider.value, { ...form.value })
    }
    emit('toast', '登录成功', 'success')
    emit('close')
  } catch (e) {
    emit('toast', String(e), 'error')
  }
  busy.value = false
}
</script>

<template>
  <div class="modal-mask" @click.self="emit('close')">
    <div class="modal">
      <h3>添加账号</h3>
      <div class="field">
        <label>网盘</label>
        <select class="select" v-model="provider" style="width:100%">
          <option v-for="p in providers" :key="p.ID" :value="p.ID">{{ p.Meta.Label }}</option>
        </select>
      </div>

      <template v-if="isMounted()">
        <div class="field"><label>名称</label><input class="input" v-model="mountedForm.name" /></div>
        <div class="field"><label>地址 / Endpoint</label><input class="input" v-model="mountedForm.endpoint" placeholder="https://dav.example.com 或 s3.example.com" /></div>
        <div class="field"><label>用户名 / AccessKey</label><input class="input" v-model="mountedForm.username" /></div>
        <div class="field"><label>密码 / SecretKey</label><input class="input" type="password" v-model="mountedForm.password" /></div>
        <template v-if="provider === 's3'">
          <div class="field"><label>Bucket</label><input class="input" v-model="mountedForm.bucket" /></div>
          <div class="field"><label>Region（可选）</label><input class="input" v-model="mountedForm.region" /></div>
          <div class="field"><label>基础路径（可选）</label><input class="input" v-model="mountedForm.basePath" /></div>
        </template>
      </template>

      <template v-else>
        <div v-for="f in loginFields()" :key="f.Key" class="field">
          <label>{{ f.Label }}</label>
          <input
            class="input"
            :type="f.Type === 'password' ? 'password' : 'text'"
            v-model="form[f.Key]"
            :placeholder="f.Placeholder || ''"
          />
        </div>
        <div v-if="provider === 'onedrive' || provider === 'dropbox'" class="field" style="color:var(--text-tertiary);font-size:12px">
          点击登录后将在浏览器完成 OAuth 授权，自动回调。
        </div>
      </template>

      <div class="modal-actions">
        <button class="btn" @click="emit('close')">取消</button>
        <button class="btn primary" :disabled="busy" @click="submit">{{ busy ? '登录中…' : '登录' }}</button>
      </div>
    </div>
  </div>
</template>