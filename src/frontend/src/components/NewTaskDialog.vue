<template>
  <Dialog
    :visible="visible"
    modal
    header="添加下载"
    :style="{ width: '560px' }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <div v-if="selection" class="selection-summary">
      <i class="pi pi-images" />
      <span>将从「<b>{{ selection.title }}</b>」下载 <b>{{ selection.messageIds.length }}</b> 个文件</span>
    </div>

    <div v-else class="form-field">
      <label for="nt-urls">消息链接（每行一条）</label>
      <Textarea
        id="nt-urls"
        v-model="urls"
        rows="6"
        auto-resize
        placeholder="https://t.me/channel_name/123&#10;https://t.me/c/1234567890/456"
      />
      <span class="hint">支持公开频道与私有频道（t.me/c/...）消息链接</span>
    </div>

    <div class="form-field">
      <label for="nt-dir">保存目录</label>
      <DirSelect id="nt-dir" v-model="dir" kind="download" />
    </div>

    <div class="form-field">
      <label for="nt-script">过滤 / 命名脚本（可选）</label>
      <Select
        id="nt-script"
        v-model="scriptName"
        :options="scriptOptions"
        option-label="label"
        option-value="value"
        :placeholder="scriptPlaceholder"
        show-clear
        fluid
      />
      <span class="hint">脚本可实现按条件跳过文件、自定义文件名与任务钩子，到「脚本」页编写</span>
    </div>

    <div class="options-row">
      <div class="opt">
        <Checkbox v-model="group" input-id="nt-opt-group" binary />
        <label for="nt-opt-group">下载整组相册</label>
      </div>
      <div class="opt">
        <Checkbox v-model="skipSame" input-id="nt-opt-skip" binary />
        <label for="nt-opt-skip">跳过同名同大小文件</label>
      </div>
      <div class="opt">
        <Checkbox v-model="rewriteExt" input-id="nt-opt-ext" binary />
        <label for="nt-opt-ext">按 MIME 修正扩展名</label>
      </div>
    </div>

    <template #footer>
      <Button label="取消" severity="secondary" text @click="close" />
      <Button
        label="创建下载任务"
        icon="pi pi-download"
        :disabled="selection ? !selection.messageIds.length : !urlList.length"
        :loading="creating"
        @click="create"
      />
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'

import DirSelect from './DirSelect.vue'
import { Download } from '../api'
import type { Selection } from '../types'
import { engine } from '../../wailsjs/go/models'
import { useScriptsStore } from '../stores/scripts'
import { useSettingsStore } from '../stores/settings'

const props = defineProps<{
  visible: boolean
  /** 选集模式：指定后隐藏链接输入，直接按对话+消息 ID 下载 */
  selection?: { dialogId: number; dialogType: string; title: string; messageIds: number[] } | null
}>()
const emit = defineEmits<{
  'update:visible': [boolean]
  created: []
}>()

const scripts = useScriptsStore()
const settings = useSettingsStore()
const toast = useToast()

const urls = ref('')
const dir = ref('')
const scriptName = ref<string | null>(null)
const group = ref(true)
const skipSame = ref(true)
const rewriteExt = ref(false)
const creating = ref(false)
const dirSelect = ref<InstanceType<typeof DirSelect> | null>(null)

const urlList = computed(() =>
  urls.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean),
)

const scriptOptions = computed(() =>
  scripts.enabled.map((s) => ({ label: s.name, value: s.name })),
)

const scriptPlaceholder = computed(() =>
  scriptOptions.value.length ? '不使用脚本' : '无已启用脚本（到脚本页启用）',
)

function close() {
  emit('update:visible', false)
}

async function create() {
  creating.value = true
  try {
    const sel = props.selection
    const selections: Selection[] = sel
      ? [{ dialogId: sel.dialogId, dialogType: sel.dialogType, messageIds: sel.messageIds }]
      : []
    await Download.createTask(
      engine.TaskOptions.createFrom({
        urls: sel ? [] : urlList.value,
        selections,
        label: sel ? `${sel.title} × ${sel.messageIds.length} 个文件` : '',
        dir: dir.value,
        scriptName: scriptName.value ?? '',
        template: '',
        rewriteExt: rewriteExt.value,
        skipSame: skipSame.value,
        group: group.value,
        restart: false,
      }),
    )
    urls.value = ''
    dirSelect.value?.commit()
    emit('created')
    close()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '创建失败', detail: String(e), life: 6000 })
  } finally {
    creating.value = false
  }
}
</script>

<style scoped>
.selection-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
  border-radius: 8px;
  background: var(--p-surface-100);
  font-size: 13px;
}

.app-dark .selection-summary {
  background: var(--p-surface-800);
}

.selection-summary .pi {
  color: var(--p-primary-color);
}

.options-row {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
}

.opt {
  display: flex;
  align-items: center;
  gap: 8px;
}

.opt label {
  cursor: pointer;
  font-size: 13px;
}

.form-row > .grow {
  flex: 1;
}
</style>
