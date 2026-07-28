<template>
  <div class="page">
    <div class="page-header">
      <h1>设置</h1>
      <p>代理、下载与界面主题</p>
    </div>

    <div class="panel-card">
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

      <div class="form-field">
        <label>数据目录</label>
        <span class="hint mono">{{ store.dataDir || '—' }}</span>
      </div>

      <div class="actions">
        <Button label="保存设置" icon="pi pi-save" :loading="saving" @click="save" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'

import { Download } from '../api'
import type { ThemeMode } from '../theme'
import { useSettingsStore } from '../stores/settings'

const store = useSettingsStore()
const toast = useToast()
const saving = ref(false)

const themeOptions = [
  { label: '亮色', value: 'light' },
  { label: '暗色', value: 'dark' },
  { label: '跟随系统', value: 'system' },
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
