<template>
  <div class="page page-table">
    <div class="page-header">
      <h1>对话</h1>
      <p>你的私聊、群组与频道，点击进入对话查看媒体文件</p>
    </div>

    <Message v-if="!auth.loggedIn" severity="warn" class="mb-16">
      <span>尚未登录账号，请先登录后再查看对话。</span>
      <Button label="去登录" size="small" class="ml-8" @click="router.push('/login')" />
    </Message>

    <div class="table-card panel-card">
      <!-- 工具栏：搜索 + 正则/大小写开关 + 刷新 -->
      <div class="table-toolbar">
        <div class="search-box">
          <InputText v-model="search" placeholder="搜索对话标题或用户名" class="search-input" />
          <i
            class="search-icon"
            :class="search ? 'pi pi-times clearable' : 'pi pi-search'"
            @click="search && (search = '')"
          />
        </div>
        <Button
          label=".*"
          size="small"
          :severity="useRegex ? 'primary' : 'secondary'"
          :outlined="!useRegex"
          v-tooltip.top="'正则表达式'"
          @click="useRegex = !useRegex"
        />
        <Button
          label="Aa"
          size="small"
          :severity="caseSensitive ? 'primary' : 'secondary'"
          :outlined="!caseSensitive"
          v-tooltip.top="'区分大小写'"
          @click="caseSensitive = !caseSensitive"
        />
        <span v-if="regexInvalid" class="regex-error">正则表达式无效</span>
        <span class="spacer" />
        <span class="count-hint">{{ filtered.length }} / {{ chats.dialogs.length }} 个对话</span>
        <Button
          icon="pi pi-refresh"
          size="small"
          severity="secondary"
          text
          rounded
          :loading="chats.loading"
          v-tooltip.top="'刷新对话列表'"
          @click="refresh"
        />
      </div>

      <DataTable
        :value="filtered"
        data-key="id"
        scrollable
        scroll-height="flex"
        :virtual-scroller-options="{ itemSize: 52 }"
        :loading="chats.loading"
        selection-mode="single"
        class="row-clickable"
        @row-click="onRowClick"
      >
        <Column field="type" header="类型" style="width: 90px">
          <template #body="{ data }">
            <Tag :value="typeLabel(data.type)" :severity="typeSeverity(data.type)" />
          </template>
        </Column>
        <Column field="title" header="标题" sortable>
          <template #body="{ data }">
            <span class="cell-title">{{ data.title }}</span>
          </template>
        </Column>
        <Column field="username" header="用户名" style="width: 180px">
          <template #body="{ data }">
            <span class="cell-muted">{{ data.username ? '@' + data.username : '-' }}</span>
          </template>
        </Column>
        <Column field="unreadCount" header="未读" style="width: 80px">
          <template #body="{ data }">
            <Badge v-if="data.unreadCount" :value="data.unreadCount" />
            <span v-else class="cell-muted">-</span>
          </template>
        </Column>
        <Column field="lastMessageAt" header="最近消息" sortable style="width: 170px">
          <template #body="{ data }">
            <span class="cell-muted">{{ fmtTime(data.lastMessageAt) }}</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty-state small">
            <i class="pi pi-comments" />
            <p>{{ chats.loaded ? '没有匹配的对话' : '暂无对话' }}</p>
          </div>
        </template>
      </DataTable>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Badge from 'primevue/badge'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'

import type { DialogView } from '../types'
import { useAuthStore } from '../stores/auth'
import { useChatsStore } from '../stores/chats'

const auth = useAuthStore()
const chats = useChatsStore()
const router = useRouter()
const toast = useToast()

const search = ref('')
const useRegex = ref(false)
const caseSensitive = ref(false)

// 编译正则：undefined 表示非法（无副作用，供模板提示）
const regex = computed(() => {
  if (!useRegex.value || !search.value.trim()) return null
  try {
    return new RegExp(search.value.trim(), caseSensitive.value ? '' : 'i')
  } catch {
    return undefined
  }
})
const regexInvalid = computed(() => useRegex.value && search.value.trim() !== '' && regex.value === undefined)

const filtered = computed(() => {
  const q = search.value.trim()
  if (!q) return chats.dialogs

  if (useRegex.value) {
    if (!regex.value) return chats.dialogs // 非法正则回退为不过滤
    const re = regex.value
    return chats.dialogs.filter((d) => re.test(d.title) || re.test(d.username ?? ''))
  }

  if (caseSensitive.value) {
    return chats.dialogs.filter((d) => d.title.includes(q) || (d.username ?? '').includes(q))
  }
  const needle = q.toLowerCase()
  return chats.dialogs.filter(
    (d) => d.title.toLowerCase().includes(needle) || (d.username ?? '').toLowerCase().includes(needle),
  )
})

function typeLabel(t: string) {
  switch (t) {
    case 'private':
      return '私聊'
    case 'group':
      return '群组'
    case 'channel':
      return '频道'
    default:
      return t
  }
}

function typeSeverity(t: string) {
  switch (t) {
    case 'private':
      return 'success'
    case 'group':
      return 'info'
    case 'channel':
      return 'warn'
    default:
      return 'secondary'
  }
}

function fmtTime(unix?: number) {
  if (!unix) return '-'
  const d = new Date(unix * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function onRowClick(e: { data: DialogView }) {
  router.push({
    path: `/chats/${e.data.id}`,
    query: { type: e.data.type, title: e.data.title },
  })
}

async function refresh() {
  try {
    await chats.refresh()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '加载对话失败', detail: String(e?.message ?? e), life: 5000 })
  }
}

onMounted(() => {
  if (auth.loggedIn) refresh()
})

// 登录状态变化：登录后自动加载，登出后清空
watch(
  () => auth.loggedIn,
  (v) => {
    if (v) {
      refresh()
    } else {
      chats.reset()
    }
  },
)
</script>

<style scoped>
.mb-16 {
  margin-bottom: 16px;
}

.ml-8 {
  margin-left: 8px;
}

.search-input {
  width: 320px;
}

.count-hint {
  color: var(--p-text-muted-color);
  font-size: 12px;
  margin-right: 4px;
}

.regex-error {
  color: var(--p-red-500);
  font-size: 12px;
}

.cell-title {
  font-weight: 500;
}

.cell-muted {
  color: var(--p-text-muted-color);
  font-size: 13px;
}

.empty-state.small {
  padding: 48px 24px;
}

.empty-state.small .pi {
  font-size: 2rem;
  margin-bottom: 12px;
}
</style>
