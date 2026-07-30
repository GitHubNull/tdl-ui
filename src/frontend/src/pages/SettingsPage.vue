<template>
  <div class="page">
    <div class="page-header">
      <h1>设置</h1>
      <p>代理、下载、界面主题与日志</p>
    </div>

    <div class="panel-card">
      <h2 class="section-title">账号</h2>
      <div class="form-field">
        <label>Telegram 账号</label>
        <div class="form-row">
          <span class="hint grow">{{ accountHint }}</span>
          <Button
            v-if="auth.loggedIn"
            label="账号管理"
            icon="pi pi-user"
            severity="secondary"
            outlined
            @click="router.push('/login')"
          />
          <Button v-else label="前往登录" icon="pi pi-sign-in" @click="router.push('/login')" />
        </div>
      </div>

      <h2 class="section-title">网络</h2>
      <div class="form-field">
        <label for="proxy">代理地址</label>
        <InputText id="proxy" v-model="store.settings.proxy" placeholder="socks5://127.0.0.1:1080 或 http://127.0.0.1:8080，留空直连" />
        <span class="hint">修改后对新创建的任务与登录流程生效</span>
      </div>

      <h2 class="section-title">下载</h2>
      <div class="form-field">
        <label for="dl-dir">默认下载目录</label>
        <div class="form-row">
          <InputText id="dl-dir" v-model="store.settings.downloadDir" class="grow" />
          <Button icon="pi pi-folder-open" severity="secondary" outlined v-tooltip.top="'浏览…'" @click="browse" />
        </div>
      </div>
      <div class="form-field">
        <label for="tpl">默认命名模板（Go text/template，与 tdl 兼容）</label>
        <InputText id="tpl" v-model="store.settings.template" spellcheck="false" />
        <span class="hint">可用变量：DialogID、MessageID、MessageDate、FileName、FileCaption、FileSize；函数：filenamify 等</span>
      </div>
      <div class="num-row">
        <div class="form-field">
          <label for="threads">单文件线程数</label>
          <InputNumber id="threads" v-model="store.settings.threads" :min="1" :max="16" show-buttons fluid />
        </div>
        <div class="form-field">
          <label for="limit">并发文件数</label>
          <InputNumber id="limit" v-model="store.settings.limit" :min="1" :max="8" show-buttons fluid />
        </div>
        <div class="form-field">
          <label for="pool">DC 连接池大小</label>
          <InputNumber id="pool" v-model="store.settings.poolSize" :min="1" :max="32" show-buttons fluid />
        </div>
      </div>

      <h2 class="section-title">界面</h2>
      <div class="form-field">
        <label>主题</label>
        <SelectButton
          :model-value="store.settings.theme"
          :options="themeOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
          @update:model-value="onTheme"
        />
      </div>

      <h2 class="section-title">存储</h2>
      <div class="form-field">
        <label>数据目录</label>
        <span class="hint mono">{{ store.dataDir || '—' }}</span>
      </div>
      <div class="form-field">
        <label for="cache-dir">缓存目录</label>
        <div class="form-row">
          <InputText id="cache-dir" v-model="store.settings.cacheDir" class="grow" placeholder="留空使用 <数据目录>\cache" />
          <Button icon="pi pi-folder-open" severity="secondary" outlined v-tooltip.top="'浏览…'" @click="browseCacheDir" />
        </div>
        <span class="hint">缩略图与预览缓存的存放位置，保存后立即生效</span>
      </div>
      <div class="form-field">
        <label for="temp-dir">视频临时目录</label>
        <div class="form-row">
          <InputText id="temp-dir" v-model="store.settings.tempDir" class="grow" placeholder="留空使用 <数据目录>\tmp" />
          <Button icon="pi pi-folder-open" severity="secondary" outlined v-tooltip.top="'浏览…'" @click="browseTempDir" />
        </div>
        <span class="hint">在线视频边下边播的分段暂存位置，上限 2GB 自动清理，保存后立即生效</span>
      </div>
      <div class="form-field">
        <label>缓存管理</label>
        <div class="form-row">
          <Button label="清空缓存" icon="pi pi-trash" severity="secondary" outlined @click="onClearCache" />
        </div>
        <span class="hint">删除缓存目录下的缩略图、预览文件与视频临时分段，不影响已下载的内容</span>
      </div>

      <h2 class="section-title">日志</h2>
      <div class="form-field">
        <label>输出目标</label>
        <SelectButton
          v-model="store.settings.log.targets"
          :options="logTargetOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
        />
      </div>
      <div class="form-field">
        <label>日志级别</label>
        <SelectButton
          v-model="store.settings.log.level"
          :options="logLevelOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
        />
        <span class="hint">保存后立即生效，低于该级别的日志将被丢弃</span>
      </div>
      <div class="form-field">
        <label for="log-dir">日志目录</label>
        <div class="form-row">
          <InputText id="log-dir" v-model="store.settings.log.dir" class="grow" placeholder="留空使用默认目录" />
          <Button icon="pi pi-folder-open" severity="secondary" outlined v-tooltip.top="'浏览…'" @click="browseLogDir" />
        </div>
        <span class="hint">默认为程序目录下的 logs 子目录</span>
      </div>
      <div class="form-field">
        <label for="log-fmt">格式模板</label>
        <InputText id="log-fmt" v-model="store.settings.log.format" class="mono" spellcheck="false" placeholder="留空使用默认格式" />
        <span class="hint">可用占位符：{datetime} {level} {module} {file} {func} {line} {msg}</span>
      </div>
      <div class="num-row">
        <div class="form-field">
          <label for="log-size">单文件上限（MB）</label>
          <InputNumber id="log-size" v-model="store.settings.log.maxSizeMb" :min="1" :max="1024" show-buttons fluid />
        </div>
        <div class="form-field">
          <label for="log-age">保留天数</label>
          <InputNumber id="log-age" v-model="store.settings.log.maxAgeDays" :min="1" :max="365" show-buttons fluid />
        </div>
        <div class="form-field">
          <label for="log-backups">保留文件数</label>
          <InputNumber id="log-backups" v-model="store.settings.log.maxBackups" :min="1" :max="100" show-buttons fluid />
        </div>
      </div>
      <div class="form-field">
        <label>YAML 配置</label>
        <div class="form-row">
          <Button label="导入 YAML 配置" icon="pi pi-file-import" severity="secondary" outlined @click="importLogYaml" />
          <Button label="导出当前配置" icon="pi pi-file-export" severity="secondary" outlined @click="exportLogYaml" />
        </div>
        <span class="hint">数据目录下的 logging.yaml 会在启动时自动加载；导入后立即生效无需保存</span>
      </div>

      <div class="actions">
        <Button label="保存设置" icon="pi pi-save" :loading="saving" @click="save" />
      </div>
    </div>
    <ConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import ConfirmDialog from 'primevue/confirmdialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'

import { Download, LogApi, SettingsApi } from '../api'
import type { ThemeMode } from '../theme'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'

const store = useSettingsStore()
const auth = useAuthStore()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const saving = ref(false)

const accountHint = computed(() =>
  auth.loggedIn ? `已登录：${auth.username || auth.userId}` : '尚未登录 Telegram 账号',
)

const themeOptions = [
  { label: '亮色', value: 'light' },
  { label: '暗色', value: 'dark' },
  { label: '跟随系统', value: 'system' },
]

const logTargetOptions = [
  { label: '仅文件', value: 'file' },
  { label: '仅界面', value: 'ui' },
  { label: '两者', value: 'both' },
]

const logLevelOptions = [
  { label: 'DEBUG', value: 'debug' },
  { label: 'INFO', value: 'info' },
  { label: 'WARN', value: 'warn' },
  { label: 'ERROR', value: 'error' },
]

function onTheme(mode: ThemeMode) {
  store.applyTheme(mode)
}

async function browse() {
  try {
    const picked = await Download.selectDirectory()
    if (picked) store.settings.downloadDir = picked
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '选择目录失败', detail: String(e), life: 4000 })
  }
}

async function browseLogDir() {
  try {
    const picked = await Download.selectDirectory()
    if (picked) store.settings.log.dir = picked
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '选择目录失败', detail: String(e), life: 4000 })
  }
}

async function browseCacheDir() {
  try {
    const picked = await Download.selectDirectory()
    if (picked) store.settings.cacheDir = picked
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '选择目录失败', detail: String(e), life: 4000 })
  }
}

async function browseTempDir() {
  try {
    const picked = await Download.selectDirectory()
    if (picked) store.settings.tempDir = picked
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '选择目录失败', detail: String(e), life: 4000 })
  }
}

function onClearCache() {
  confirm.require({
    header: '清空缓存',
    message: '将删除缓存目录下的缩略图、预览文件与视频临时分段，不影响已下载的内容。确定继续？',
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: '清空', severity: 'danger' },
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await SettingsApi.clearCache()
        toast.add({ severity: 'success', summary: '缓存已清空', life: 3000 })
      } catch (e: any) {
        toast.add({ severity: 'error', summary: '清空缓存失败', detail: String(e), life: 5000 })
      }
    },
  })
}

async function importLogYaml() {
  try {
    const imported = await LogApi.importYAMLConfig()
    store.settings.log = { ...imported }
    toast.add({ severity: 'success', summary: '日志配置已导入并生效', life: 2500 })
  } catch (e: any) {
    const msg = String(e)
    if (msg.includes('已取消')) return
    toast.add({ severity: 'error', summary: '导入失败', detail: msg, life: 5000 })
  }
}

async function exportLogYaml() {
  try {
    await LogApi.exportYAMLConfig()
    toast.add({ severity: 'success', summary: '日志配置已导出', life: 2500 })
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '导出失败', detail: String(e), life: 5000 })
  }
}

async function save() {
  saving.value = true
  try {
    await store.save()
    toast.add({ severity: 'success', summary: '设置已保存', life: 2500 })
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '保存失败', detail: String(e), life: 5000 })
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.section-title {
  font-size: 13px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--p-text-muted-color);
  margin: 0 0 16px;
}

.section-title:not(:first-child) {
  margin-top: 32px;
}

.num-row {
  display: flex;
  gap: 16px;
}

.num-row .form-field {
  flex: 1;
}

.mono {
  font-family: Consolas, monospace;
}

.actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 24px;
}
</style>
