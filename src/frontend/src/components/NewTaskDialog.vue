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

    <!-- 文件重命名（选集模式且有媒体项时显示） -->
    <div v-if="selection?.items?.length" class="form-field">
      <div class="rename-header" @click="renameExpanded = !renameExpanded">
        <i class="pi" :class="renameExpanded ? 'pi-chevron-down' : 'pi-chevron-right'" />
        <span>文件重命名（可选）</span>
        <span v-if="renameCount" class="rename-count">{{ renameCount }} 项已自定义</span>
      </div>
      <div v-show="renameExpanded" class="rename-list">
        <div v-for="(item, idx) in selection.items" :key="item.messageId" class="rename-row">
          <span class="rename-idx">{{ idx + 1 }}</span>
          <span class="rename-orig" :title="item.name">{{ item.name }}</span>
          <div class="rename-input-group">
            <InputText
              class="rename-input"
              :model-value="renames.get(item.messageId) ?? ''"
              :placeholder="nameWithoutExt(item.name)"
              @update:model-value="(v: string | undefined) => setRename(item.messageId, v ?? '')"
            />
            <span class="rename-ext">{{ extOf(item.name) }}</span>
          </div>
        </div>
      </div>
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
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'

import DirSelect from './DirSelect.vue'
import { Download } from '../api'
import type { MediaItem, Selection } from '../types'
import { engine } from '../../wailsjs/go/models'
import { useScriptsStore } from '../stores/scripts'
import { useSettingsStore } from '../stores/settings'

const props = defineProps<{
  visible: boolean
  /** 选集模式：指定后隐藏链接输入，直接按对话+消息 ID 下载 */
  selection?: {
    dialogId: number
    dialogType: string
    title: string
    messageIds: number[]
    /** 选中媒体项的完整信息（用于重命名列表展示原始文件名） */
    items: MediaItem[]
  } | null
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

// ---- 文件重命名 ----
/** messageId → 自定义文件名（不含扩展名），空串/不存在 = 使用默认命名 */
const renames = ref(new Map<number, string>())
const renameExpanded = ref(false)

const renameCount = computed(() => {
  let n = 0
  for (const v of renames.value.values()) {
    if (v.trim()) n++
  }
  return n
})

/** 提取文件名主体（去扩展名） */
function nameWithoutExt(fileName: string): string {
  const dot = fileName.lastIndexOf('.')
  return dot > 0 ? fileName.slice(0, dot) : fileName
}

/** 提取扩展名（含点，如 .mp4） */
function extOf(fileName: string): string {
  const dot = fileName.lastIndexOf('.')
  return dot > 0 ? fileName.slice(dot) : ''
}

function setRename(messageId: number, value: string) {
  const next = new Map(renames.value)
  const trimmed = value.trim()
  if (trimmed) {
    next.set(messageId, trimmed)
  } else {
    next.delete(messageId)
  }
  renames.value = next
}

/** 将 renames Map 转为后端期望的 Record 格式 */
function renamesToRecord(): Record<number, string> | undefined {
  if (!renames.value.size) return undefined
  const obj: Record<number, string> = {}
  for (const [k, v] of renames.value) {
    if (v.trim()) obj[k] = v.trim()
  }
  return Object.keys(obj).length ? obj : undefined
}

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
        renames: renamesToRecord(),
      }),
    )
    urls.value = ''
    renames.value = new Map()
    renameExpanded.value = false
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

/* ---- 文件重命名列表 ---- */

.rename-header {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  user-select: none;
  font-size: 13px;
  font-weight: 500;
  color: var(--p-text-color);
  padding: 4px 0;
}

.rename-header .pi {
  font-size: 12px;
  color: var(--p-text-muted-color);
}

.rename-count {
  margin-left: auto;
  font-size: 12px;
  font-weight: 400;
  color: var(--p-primary-color);
}

.rename-list {
  max-height: 240px;
  overflow-y: auto;
  border: 1px solid var(--p-surface-200);
  border-radius: 6px;
  margin-top: 4px;
}

.app-dark .rename-list {
  border-color: var(--p-surface-700);
}

.rename-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  font-size: 13px;
  border-bottom: 1px solid var(--p-surface-100);
}

.app-dark .rename-row {
  border-bottom-color: var(--p-surface-800);
}

.rename-row:last-child {
  border-bottom: none;
}

.rename-idx {
  flex-shrink: 0;
  width: 20px;
  text-align: right;
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.rename-orig {
  flex-shrink: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--p-text-muted-color);
  font-size: 12px;
  max-width: 160px;
}

.rename-input-group {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 0;
  min-width: 0;
}

.rename-input {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  padding: 4px 8px;
}

.rename-ext {
  flex-shrink: 0;
  padding: 4px 6px;
  font-size: 12px;
  color: var(--p-text-muted-color);
  background: var(--p-surface-100);
  border-radius: 0 4px 4px 0;
  border: 1px solid var(--p-surface-200);
  border-left: none;
}

.app-dark .rename-ext {
  background: var(--p-surface-800);
  border-color: var(--p-surface-700);
}
</style>
