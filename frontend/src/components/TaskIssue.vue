<script setup>
import { taskFailureNotice } from '../task-notices'
import { dispatchDriveNotice } from '../workspace'

const props = defineProps({
  issue: { type: Object, default: null },
  reason: { type: String, default: '' },
  action: { type: String, default: '处理任务' },
  userId: { type: String, default: '' },
  writeOperation: { type: Boolean, default: false },
})
function showNotice() {
  dispatchDriveNotice(taskFailureNotice(props.reason, props.action, props.issue, { userId: props.userId }))
}
</script>

<template>
  <button v-if="issue?.code || reason" type="button" class="task-notice btn sm" title="在通知中查看失败原因和处理建议" @click.stop="showNotice">查看通知</button>
</template>

<style scoped>
.task-notice { margin-top: 4px; }
</style>
