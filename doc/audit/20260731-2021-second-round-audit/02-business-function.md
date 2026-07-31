# 02 业务功能类发现（FUNC）

> 读者为 AI 编程代理。定位以符号名为准，行号仅供参考。禁止时间估算，优先级仅用 P0-P3。
> 本轮 ID 独立于 2026-07-29 轮次；"关联旧 {ID}" 指上一轮文档中的条目。

本类共 2 项：Medium 1、Low 1。

---

## FUNC-001 [Medium] ChatsPage 单/双击延时定时器跨对话未清理，切换对话后可误开旧对话预览

- 类别：业务功能 / 交互流程、边界条件
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue` — `onThumbClick`、`thumbClickTimer`、`thumbClickMsgId`、`watch(selectedId)` 切换对话处理块、`onBeforeUnmount`

### 问题描述（含影响分析）

缩略图区域用 250ms 延时（`DBLCLICK_DELAY_MS`）区分单击（开预览）与双击（打开已下载文件）。`thumbClickTimer` / `thumbClickMsgId` 仅在 `onBeforeUnmount` 中清理，**切换对话的 `watch(selectedId)` 分支不清理**。触发序列：

1. 用户在对话 A 单击一张已下载媒体的缩略图 → `thumbClickTimer` 启动，250ms 后将调用 `openPreview(itA)`。
2. 250ms 内用户点击左侧对话列表切到对话 B → `watch(selectedId)` 重置媒体列表并加载 B。
3. 定时器到期，`openPreview(itA)` 仍执行——以对话 A 的 `MediaItem` 打开预览遮罩，而当前页面上下文已是对话 B。

影响：预览遮罩显示与当前对话不符的媒体；若预览组件按 `props.items`（已被 B 的数据替换）解析索引，可能出现预览项与 items 不一致的错乱状态。另有次级问题：`thumbClickMsgId` 不清零，若 A、B 两对话恰有相同 `messageId`（不同 dialog 的 messageId 空间独立，完全可能），切换后首击会被误判为"双击第二击"直接触发 `openDownloaded`。

### 根因分析

单双击区分的可变状态（timer + msgId）生命周期挂在组件卸载上，而实际语义边界是"当前对话"——对话切换是与卸载等价的上下文失效事件，但未挂接清理。

### 修复方案

1. 抽一个 `resetThumbClick()` 函数：`clearTimeout(thumbClickTimer)`（非空时）、`thumbClickTimer = null`、`thumbClickMsgId = 0`。
2. 在 `watch(selectedId)` 的处理块开头调用 `resetThumbClick()`。
3. `onBeforeUnmount` 中现有的清理改为调用同一函数。
4. 登出重置分支（"登出后清空选中与媒体"处）同样调用。

### 实施步骤与优先级（P1）

1. 修改 `ChatsPage.vue`，三个调用点接入 `resetThumbClick()`。
2. `cd src/frontend && pnpm build` 确认 vue-tsc 通过。
3. 用 chrome-devtools mcp 做交互验证（见验收标准）。

### 验收标准（测试验证方法）

- 交互验证：对话 A 单击已下载媒体缩略图后 250ms 内切到对话 B → 不出现任何预览遮罩；再在 B 中单击任意缩略图 → 正常打开 B 的预览（首击不触发"打开文件"）。
- `pnpm build` 通过。

---

## FUNC-002 [Low] MediaPreview pendingAdvance 在 items 整体重置时未复位，理论上可误前进

- 类别：业务功能 / 边界条件（关联旧 FE-26：该修复主体正确，遗留一个窄边界）
- 涉及文件：`src/frontend/src/components/MediaPreview.vue` — `pendingAdvance`、`go`、`close` 及监听 `props.items.length` 的 watch

### 问题描述（含影响分析）

`pendingAdvance` 在"末项点下一张且 hasMore"时置 true，等待续拉数据到达后自动前进（FE-26）。`close()` 与 `go()` 正常路径均会复位；items 清空时 watch 会走 `close()` 分支也会复位。剩余的窄边界：预览打开期间外部把 `items` **整体替换为另一批非空数据**（长度仍增加），watch 的 `pendingAdvance && props.index + 1 < len` 条件满足时会向"新数据集"前进一格——前进目标与用户当初的续拉意图不再对应。

当前实际调用方（ChatsPage + useMediaPager）在预览打开时只会追加分页数据、不会整体换源（切对话前会先关预览），故该问题**当前不可达**，定级 Low，属防御性加固。

### 根因分析

`pendingAdvance` 表达的是"对旧数据集尾部的续拉预期"，但复位条件只覆盖了长度减小/清空，未覆盖"数据集代际更换"。

### 修复方案

1. `MediaPreview.vue` 新增 `watch(() => props.items, () => { pendingAdvance = false }, ...)` 之外更精确的方案：在现有长度 watch 中，当 `len < oldLen`（收缩，含清空）或首元素身份变化（`props.items[0]?.messageId` 与旧值不同）时复位 `pendingAdvance = false`。
2. 或者由父组件在数据源重置时通过 `:key` 强制重建 MediaPreview（更粗但更简单）。推荐方案 1，避免遮罩闪烁。

### 实施步骤与优先级（P2）

1. 修改 watch 逻辑加入代际判断。
2. `cd src/frontend && pnpm build`。

### 验收标准（测试验证方法）

- 交互验证：打开预览浏览至末项点"下一张"触发续拉，数据到达后自动前进一格（FE-26 行为保持）；关闭预览重开后不发生无操作的自动前进。
- `pnpm build` 通过。

---

## 正面结论（勿误改）

以下业务功能实现经本轮验证**正确**，修复上述发现时不得破坏：

1. **FE-26 末项续拉自动前进主路径**（`MediaPreview.vue` `go`/watch）：hasMore 时置 pendingAdvance、数据到达后前进、items 清空自动关闭的主逻辑正确。
2. **FE-01 epoch 代际防乱序**（`src/frontend/src/composables/useMediaPager.ts`）：切换对话/重置后在途响应一律丢弃，验证仍然有效。
3. **FE-20 月份跳转重置**（`ChatsPage.vue`）：重新查询从最新开始并同步清空月份显示值，重选同月可再次触发。
4. **FE-25 任务终态清理**（`src/frontend/src/pages/TasksPage.vue`）：任务移除/重跑时清理展开与文件列表状态。
5. **FE-12 未保存修改守卫**（`src/frontend/src/pages/ScriptsPage.vue`）：切换/新建/删除前确认，防静默丢失。
6. **视频本地回放直通**（`main.go` MediaHandler 装配 + `taskManager.DownloadedFile`）：已下载视频由磁盘直接回放，零 API 消耗，功能与降级路径（未下载走流式预览）均正常。
7. **双击打开已下载文件**（`ChatsPage.vue` `onThumbClick` 的双击分支）：同一 msgId 的第二击正确取消定时器并调用 `openDownloaded`。
