<template>
  <div class="page">
    <div class="page-header">
      <h1>账号</h1>
      <p>登录 Telegram 账号后即可创建下载任务</p>
    </div>

    <!-- 已登录 -->
    <div v-if="auth.loggedIn" class="panel-card logged-in">
      <div class="user-row">
        <div class="avatar"><i class="pi pi-user" /></div>
        <div class="grow">
          <div class="user-name">{{ auth.name || auth.username || '已登录' }}</div>
          <div class="user-meta">
            <span v-if="auth.username">@{{ auth.username }} · </span>ID {{ auth.userId }}
          </div>
        </div>
        <Button label="退出登录" icon="pi pi-sign-out" severity="secondary" outlined @click="doLogout" />
      </div>
    </div>

    <!-- 未登录 -->
    <div v-else class="panel-card">
      <Message v-if="auth.error" severity="error" class="mb-16">{{ auth.error }}</Message>

      <Tabs value="code">
        <TabList>
          <Tab value="code"><i class="pi pi-mobile tab-icon" />验证码登录</Tab>
          <Tab value="qr"><i class="pi pi-qrcode tab-icon" />二维码登录</Tab>
          <Tab value="desktop"><i class="pi pi-desktop tab-icon" />Desktop 导入</Tab>
        </TabList>
        <TabPanels>
          <!-- 验证码登录 -->
          <TabPanel value="code">
            <div class="form-field">
              <label for="phone">手机号（含国际区号）</label>
              <InputText id="phone" v-model="phone" placeholder="+8613800000000" :disabled="busy" />
            </div>
            <Button
              v-if="auth.stage !== 'need_code' && auth.stage !== 'need_password'"
              label="发送验证码"
              icon="pi pi-send"
              :loading="auth.stage === 'pending'"
              @click="startCode"
            />

            <template v-if="auth.stage === 'need_code'">
              <div class="form-field">
                <label for="code">验证码</label>
                <InputText id="code" v-model="code" placeholder="Telegram 收到的验证码" autofocus />
              </div>
              <Button label="提交验证码" icon="pi pi-check" @click="submitCode" />
            </template>

            <PasswordStep v-if="auth.stage === 'need_password'" @submit="submitPassword" />
          </TabPanel>

          <!-- 二维码登录 -->
          <TabPanel value="qr">
            <div v-if="auth.stage === 'qr' && auth.qrUrl" class="qr-wrap">
              <canvas ref="qrCanvas" />
              <p class="hint">使用手机 Telegram「设置 → 设备 → 关联桌面设备」扫码</p>
              <Button label="取消" severity="secondary" text @click="auth.cancel()" />
            </div>
            <PasswordStep v-else-if="auth.stage === 'need_password'" @submit="submitPassword" />
            <div v-else>
              <p class="hint mb-16">生成二维码后用手机 Telegram 扫码即可登录，无需输入手机号。</p>
              <Button
                label="生成二维码"
                icon="pi pi-qrcode"
                :loading="auth.stage === 'pending'"
                @click="startQR"
              />
            </div>
          </TabPanel>

          <!-- Desktop 导入 -->
          <TabPanel value="desktop">
            <div class="form-field">
              <label for="tdpath">Telegram Desktop 数据目录</label>
              <div class="form-row">
                <InputText id="tdpath" v-model="desktopPath" class="grow" placeholder="自动探测或手动输入" />
                <Button label="自动探测" severity="secondary" outlined @click="detect" />
              </div>
            </div>
            <div class="form-field">
              <label for="passcode">本地密码（未设置留空）</label>
              <Password id="passcode" v-model="passcode" :feedback="false" toggle-mask fluid />
            </div>
            <Button label="读取账号" icon="pi pi-search" :loading="busy" @click="listAccounts" />

            <div v-if="accounts.length" class="accounts mt-16">
              <div v-for="acc in accounts" :key="acc.userId" class="form-row account-row">
                <i class="pi pi-user" />
                <span class="grow">用户 ID：{{ acc.userId }}</span>
                <Button label="导入该账号" size="small" :loading="busy" @click="importAccount(acc.userId)" />
              </div>
            </div>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { defineComponent, h, nextTick, ref, watch } from 'vue'
import QRCode from 'qrcode'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Message from 'primevue/message'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'

import { Auth } from '../api'
import type { DesktopAccount } from '../types'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const toast = useToast()

const phone = ref('')
const code = ref('')
const busy = ref(false)

// 二步验证密码输入子组件（三处复用）
const PasswordStep = defineComponent({
  emits: ['submit'],
  setup(_, { emit }) {
    const pwd = ref('')
    return () =>
      h('div', [
        h('div', { class: 'form-field' }, [
          h('label', '二步验证密码'),
          h(Password, {
            modelValue: pwd.value,
            'onUpdate:modelValue': (v: string) => (pwd.value = v),
            feedback: false,
            toggleMask: true,
            fluid: true,
          }),
        ]),
        h(Button, {
          label: '提交密码',
          icon: 'pi pi-lock',
          onClick: () => emit('submit', pwd.value),
        }),
      ])
  },
})

async function startCode() {
  try {
    await auth.startCodeLogin(phone.value)
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '登录失败', detail: String(e), life: 5000 })
    auth.stage = 'idle'
  }
}

function submitCode() {
  Auth.submitCode(code.value)
  code.value = ''
}

function submitPassword(pwd: string) {
  Auth.submitPassword(pwd)
}

// ---- 二维码 ----
const qrCanvas = ref<HTMLCanvasElement>()

async function startQR() {
  try {
    await auth.startQRLogin()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '登录失败', detail: String(e), life: 5000 })
    auth.stage = 'idle'
  }
}

watch(
  () => auth.qrUrl,
  async (url) => {
    if (!url) return
    await nextTick()
    if (qrCanvas.value) {
      QRCode.toCanvas(qrCanvas.value, url, { width: 220, margin: 2 })
    }
  },
)

// ---- Desktop 导入 ----
const desktopPath = ref('')
const passcode = ref('')
const accounts = ref<DesktopAccount[]>([])

async function detect() {
  const p = await Auth.detectDesktopPath()
  if (p) {
    desktopPath.value = p
    toast.add({ severity: 'success', summary: '已找到数据目录', detail: p, life: 3000 })
  } else {
    toast.add({ severity: 'warn', summary: '未找到', detail: '请手动指定 Telegram Desktop 目录', life: 4000 })
  }
}

async function listAccounts() {
  busy.value = true
  try {
    accounts.value = await Auth.listDesktopAccounts(desktopPath.value, passcode.value)
    if (!accounts.value.length) {
      toast.add({ severity: 'warn', summary: '没有可导入的账号', life: 3000 })
    }
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '读取失败', detail: String(e), life: 5000 })
  } finally {
    busy.value = false
  }
}

async function importAccount(userId: string) {
  busy.value = true
  try {
    await Auth.importDesktopSession(desktopPath.value, passcode.value, userId)
    toast.add({
      severity: 'success',
      summary: '导入成功',
      detail: '若 Telegram Desktop 仍在运行，双端共用会话可能触发冲突导致双双掉线，建议退出 Desktop 端登录。',
      life: 8000,
    })
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '导入失败', detail: String(e), life: 5000 })
  } finally {
    busy.value = false
  }
}

async function doLogout() {
  try {
    await auth.logout()
    toast.add({ severity: 'info', summary: '已退出登录', life: 3000 })
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '退出失败', detail: String(e), life: 5000 })
  }
}
</script>

<style scoped>
.mb-16 {
  margin-bottom: 16px;
}

.mt-16 {
  margin-top: 16px;
}

.tab-icon {
  margin-right: 8px;
}

.hint {
  color: var(--p-text-muted-color);
  font-size: 13px;
}

.user-row {
  display: flex;
  align-items: center;
  gap: 16px;
}

.user-row .grow {
  flex: 1;
}

.avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--p-primary-50);
  color: var(--p-primary-color);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.25rem;
}

.user-name {
  font-weight: 600;
  font-size: 1.05rem;
}

.user-meta {
  color: var(--p-text-muted-color);
  font-size: 13px;
  margin-top: 4px;
}

.qr-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 16px 0;
}

.qr-wrap canvas {
  border-radius: 8px;
  background: #fff;
  padding: 8px;
}

.account-row {
  padding: 8px 0;
  border-bottom: 1px solid var(--p-surface-200);
}

.account-row:last-child {
  border-bottom: none;
}
</style>
