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
        <label class="proxy-toggle">
          <ToggleSwitch v-model="proxyEnabled" />
          使用代理
        </label>
        <span class="hint">关闭后所有网络请求直连，不经过任何代理</span>
      </div>
      <template v-if="proxyEnabled">
        <div class="form-field">
          <label>代理模式</label>
          <div class="proxy-mode-options">
            <label class="proxy-mode-option">
              <RadioButton v-model="store.settings.proxyMode" value="system" />
              使用系统代理
            </label>
            <label class="proxy-mode-option">
              <RadioButton v-model="store.settings.proxyMode" value="custom" />
              自定义代理
            </label>
          </div>
          <span class="hint">系统代理读取操作系统环境变量（HTTPS_PROXY / HTTP_PROXY / ALL_PROXY）</span>
        </div>
        <div v-if="store.settings.proxyMode === 'custom'" class="form-field">
          <label for="proxy">自定义代理地址</label>
          <InputText id="proxy" v-model="store.settings.proxy" placeholder="socks5://127.0.0.1:1080 或 http://127.0.0.1:8080" />
          <span class="hint">修改后对新创建的任务与登录流程生效</span>
        </div>
      </template>

      <h2 class="section-title">下载</h2>
      <div class="form-field">
        <label for="dl-dir">默认下载目录</label>
        <DirSelect id="dl-dir" ref="dlDirSelect" v-model="store.settings.downloadDir" kind="download" />
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
      <div class="num-row">
        <div class="form-field">
          <label for="log-font-size">日志字体大小</label>
          <InputNumber id="log-font-size" v-model="store.settings.ui.logFontSize" :min="10" :max="28" show-buttons fluid />
          <span class="hint">日志页字号（px），Ctrl + 滚轮亦可调整</span>
        </div>
        <div class="form-field">
          <label for="scrollbar-size">滚动条尺寸</label>
          <InputNumber id="scrollbar-size" v-model="store.settings.ui.scrollbarSize" :min="6" :max="24" show-buttons fluid />
          <span class="hint">日志页滚动条宽度（px）</span>
        </div>
        <div class="form-field">
          <label for="max-log-lines">最大滚动行数</label>
          <InputNumber id="max-log-lines" v-model="store.settings.ui.maxLogLines" :min="16" :max="1024" show-buttons fluid />
          <span class="hint">日志页保留的最大行数，超出后最老的数据被清除</span>
        </div>
      </div>

      <h2 class="section-title">存储</h2>
      <div class="form-field">
        <label>数据目录</label>
        <span class="hint mono">{{ store.dataDir || '—' }}</span>
      </div>
      <div class="form-field">
        <label for="cache-dir">缓存目录</label>
        <DirSelect id="cache-dir" ref="cacheDirSelect" v-model="store.settings.cacheDir" kind="cache" />
        <span class="hint">缩略图与预览缓存的存放位置，保存后立即生效</span>
      </div>
      <div class="form-field">
        <label for="temp-dir">视频临时目录</label>
        <DirSelect id="temp-dir" ref="tempDirSelect" v-model="store.settings.tempDir" kind="temp" />
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
        <DirSelect id="log-dir" ref="logDirSelect" v-model="store.settings.log.dir" kind="logDir" />
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
import RadioButton from 'primevue/radiobutton'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'

import DirSelect from '../components/DirSelect.vue'
import { LogApi, SettingsApi } from '../api'
import type { ThemeMode } from '../theme'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'

const store = useSettingsStore()
const auth = useAuthStore()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const saving = ref(false)

const dlDirSelect = ref<InstanceType<typeof DirSelect> | null>(null)
const cacheDirSelect = ref<InstanceType<typeof DirSelect> | null>(null)
const tempDirSelect = ref<InstanceType<typeof DirSelect> | null>(null)
const logDirSelect = ref<InstanceType<typeof DirSelect> | null>(null)

const accountHint = computed(() =>
  auth.loggedIn ? `已登录：${auth.username || auth.userId}` : '尚未登录 Telegram 账号',
)

// 代理总开关：mode !== 'off' 即开启；开启时回落系统代理，保证互斥二选一
const proxyEnabled = computed({
  get: () => store.settings.proxyMode !== 'off',
  set: (v: boolean) => {
    store.settings.proxyMode = v ? 'system' : 'off'
  },
})

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
    // 保存成功后把各目录字段 commit 进历史
    await dlDirSelect.value?.commit()
    await cacheDirSelect.value?.commit()
    await tempDirSelect.value?.commit()
    await logDirSelect.value?.commit()
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

.proxy-toggle {
  display: flex;
  align-items: center;
  gap: 10px;
}

.proxy-mode-options {
  display: flex;
  gap: 24px;
}

.proxy-mode-option {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 24px;
}
</style>
