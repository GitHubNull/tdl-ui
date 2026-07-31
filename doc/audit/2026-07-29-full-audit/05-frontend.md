# 05 前端（FE）

> 审计对象：`src/frontend/src/` 下全部源码（api.ts、App.vue、main.ts、router.ts、theme.ts、types.ts、style.css、pages/*、stores/*、components/*）及 vite.config.ts、tsconfig.json、package.json。
> 行号仅供参考，以符号名定位为准。

---

## FE-01 [High] ChatsPage 媒体分页无请求代际控制，切换对话乱序污染数据

- 类别：竞态 / 业务逻辑
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（`watch(selectedId)` L407-411、`loadMore`/`resetMedia` L457-484）

### 问题描述
媒体分页加载存在跨对话竞态。`watch(selectedId)` 触发 `resetMedia() + loadMore()`，但若上一个对话的 `Chat.listMedia` 请求仍在途：
1. 新 `loadMore` 因 `loading.value === true` 直接 return，新对话首屏不加载；
2. 旧请求 resolve 后把**旧对话的 items 推进新对话的空列表**，并用旧对话的 `nextOffset` 覆盖 `offset/hasMore`；
3. `loadMore` 内部 `for(;;)` 循环每轮通过 `buildQuery()` 实时读取 `selectedId.value`，循环中途切换对话会把两个对话的查询混在同一轮加载里。

`applyFilters`/`jumpMonth` 触发的重载同样受影响。附带后果：不同对话的 `messageId` 可能撞车，`:key="item.messageId"`（L201）出现重复 key。

### 根因分析
无请求代际（epoch）标识，也无取消机制，`loading` 标志只能防并发不能防乱序。

### 修复方案
引入代际计数器：`let epoch = 0`；`resetMedia()` 中 `epoch++`；`loadMore` 开头捕获 `const myEpoch = epoch` 与 `const dialogId = selectedId.value`，每次 `await` 返回后校验 `myEpoch === epoch`，不等则丢弃结果并退出循环。同时把 `buildQuery()` 的 `dialogId/dialogType` 固定为循环外捕获的快照。

> 与 FE-11（拆分 ChatsPage）合并处理性价比最高——抽 `useMediaPager(dialogId)` composable 时一并内置 epoch 逻辑。

### 验收标准
- 快速连续切换多个对话，右面板最终只显示当前对话数据，无旧对话内容混入。
- 用 chrome-devtools mcp 验证：切换时无重复 key 警告。

---

## FE-02 [High] 登录 submitCode/submitPassword 完全 fire-and-forget

- 类别：错误处理
- 涉及文件：`src/frontend/src/pages/LoginPage.vue`（`submitCode`/`submitPassword` L173-180）；`src/frontend/src/api.ts`（`svc()` L30-36）

### 问题描述
核心登录路径对 `Auth.submitCode`/`Auth.submitPassword` 完全 fire-and-forget：无 `await`、无 `catch`、无 loading 态。若后端同步返回错误（Promise reject），产生 unhandled rejection，用户无任何提示；且 `code.value = ''`（L175）在结果未知时就清空输入，验证码错误后用户需重新输入。更严重的是 `api.ts` 的 `svc()` 在绑定不可用时是**同步 throw**，fire-and-forget 调用会直接抛出未捕获的同步异常。

### 修复方案
改为 `async` + `try/catch` + toast 提示 + 按钮 loading 态；错误时保留已输入的验证码。

### 验收标准
- 输入错误验证码提交，界面出现错误提示且已输入内容不被清空。

---

## FE-03 [High] api.ts/types.ts 与 wailsjs 生成绑定双轨维护

- 类别：架构 / 类型安全
- 涉及文件：`src/frontend/src/api.ts`（L22-36、L124-128）、`src/frontend/src/types.ts`、`src/frontend/wailsjs/`

### 问题描述
项目已有 Wails 生成的强类型绑定（`wailsjs/go/services/*.d.ts` + `wailsjs/go/models.ts`、`wailsjs/runtime/runtime.d.ts`），但 `api.ts` 弃之不用，通过 `window.go?.services?.[name]` 的 `any` 通道手工重建了一整层 API，`types.ts` 又手工维护一份与 `models.ts` 平行的 DTO。后果：方法名/参数拼写错误编译期无法发现；后端结构体变更后 `types.ts` 需人肉同步，存在漂移风险。

### 修复方案
`api.ts` 改为 re-export `wailsjs/go/services/*` 的生成函数（其内部已有 `window['go']` 动态调用，非 Wails 环境同样可被 try/catch 兜底）；事件用 `wailsjs/runtime` 的 `EventsOn`；`types.ts` 仅保留 `models.ts` 没有的事件负载类型（`LoginUpdate`/`FileEvent`），其余 `import type` 自 `models.ts`。

### 验收标准
- 后端结构体字段变更后，前端引用处在 `vue-tsc` 检查时报错（配合 FE-09 门禁）。

---

## FE-04 [Medium] MediaPreview keydown/mousemove 监听未在卸载时清理

- 类别：内存泄漏
- 涉及文件：`src/frontend/src/components/MediaPreview.vue`（L221-231；`startPan` L173-174）

`keydown` 监听只在 `watch(visible)` 变 false 时移除，组件没有 `onBeforeUnmount` 清理。若用户在预览打开状态下直接切换路由（ChatsPage 整体卸载），`window` 上的 `keydown` 监听永久泄漏，且闭包持有整个组件作用域。`startPan` 的 `mousemove/mouseup` 监听在拖拽中途卸载时同样泄漏。建议补 `onBeforeUnmount(() => { window.removeEventListener('keydown', onKey); endPan() })`。

---

## FE-05 [Medium] fileStates/thumbFailed 集合只增不减

- 类别：内存泄漏 / 业务逻辑
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（L646、L719、L830-834）

两处只增不减的集合：① `fileStates` Map 收 `task:file` 事件，`done` 状态条目永不清理，长会话大批量下载后无限增长；② `thumbFailed` Set 切换对话/刷新后不清空，既积累内存，也导致缩略图一次加载失败后（如临时网络抖动）本次会话内永远显示占位图标、无法重试。建议 `resetMedia()` 时按当前 dialogId 清理两个集合；`fileStates` 对 `done` 条目设置延迟删除（如 5s 后移除角标并删除）。

---

## FE-06 [Medium] LogsPage watch(source) 读取无乱序保护

- 类别：竞态
- 涉及文件：`src/frontend/src/pages/LogsPage.vue`（L139-150）

`watch(source)` 中 `await LogApi.readLogFile(name, 5000)` 无乱序保护：快速切换两个日志文件时，先发慢回的请求会用旧文件内容覆盖 `historyLines`，界面显示与下拉框选中的文件不一致。建议 await 后校验 `source.value === name` 再赋值。

---

## FE-07 [Medium] 深链 /chats/:id 只在 onMounted 读一次

- 类别：业务逻辑
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（L838-841）

深链 `/chats/:id` 只在 `onMounted` 读一次 `route.params.id`。由于 `/chats` 和 `/chats/:id` 复用同一组件实例，组件挂载后再通过 URL/历史前进后退在 `/chats/5 → /chats/7` 间切换时 `selectedId` 不会更新，右面板停留在旧对话。建议 `watch(() => route.params.id, ...)` 同步 `selectedId`（注意与 `selectDialog` 的 `router.replace` 防循环）。

---

## FE-08 [Medium] 主题状态三处存储不同步

- 类别：状态一致性 / 主题规范
- 涉及文件：`src/frontend/src/App.vue`（L85-92）、`src/frontend/src/stores/settings.ts`（L47-56）、`src/frontend/src/theme.ts`（L20-22）

主题状态三处存储（theme.ts 模块变量 + settings store + App 的 `dark` ref）不同步：① 侧栏 `toggleTheme` 直接调 `setTheme`，不更新 `settings.settings.theme`，之后打开设置页 SelectButton 显示过期值，此时点"保存设置"会把**错误的主题**持久化到后端；② `theme.ts` 的系统 `prefers-color-scheme` 变化监听切换了 DOM class，但 App 的 `dark` ref 不感知，system 模式下系统切换深浅色后侧栏图标/tooltip 显示反了。建议把主题状态收敛为单一响应式源，App 与设置页均从该源派生。

---

## FE-09 [Medium] 主题切换控件不符合 ToggleSwitch 规范

- 类别：主题规范
- 涉及文件：`src/frontend/src/App.vue`（侧栏 L34-42）、`src/frontend/src/pages/SettingsPage.vue`（L64-71）

项目规范要求主题切换使用 ToggleSwitch 滑动开关、左侧标注当前模式名称。现状：App.vue 侧栏是图标按钮，SettingsPage 是三选 SelectButton，两处均非 ToggleSwitch。主题系统在"跟随系统 + localStorage 持久化（键 `tdlui-theme`）+ CSS 变量适配（`darkModeSelector: '.app-dark'`）"上达标，仅控件形式与状态同步不符合规范。

建议：若规范以三态（light/dark/system）为准，设置页保留三态选择、侧栏快捷切换改为 ToggleSwitch 并在 tooltip 说明"手动切换将脱离跟随系统"；无论选哪种，需先修复 FE-08 的三处状态源不同步。

---

## FE-10 [Medium] 任务操作与脚本页错误处理裸奔

- 类别：错误处理
- 涉及文件：`src/frontend/src/pages/TasksPage.vue`（L56/65/74/83）、`src/frontend/src/stores/tasks.ts`（L56-68）、`src/frontend/src/pages/ScriptsPage.vue`（`validate`/`testRun` L143-164）

模板中 `tasks.pause/resume/cancel/remove` 直接调用 async action，store 内部也不捕获，失败时静默 + unhandled rejection（例如恢复任务时后端报错，用户看到按钮点了没反应）。同页 `onOpenDir` 却有完整包装，风格不一致。ScriptsPage 的 `validate()`/`testRun()` 同样无 try/catch。建议仿照 `onOpenDir` 为这些操作补 try/catch + toast 包装函数；脚本校验错误写入 `result.value`。

---

## FE-11 [Medium] ChatsPage 1369 行上帝组件

- 类别：架构
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（全文 1369 行）

单文件承担 8 类职责：对话列表过滤/排序/正则、对话选中与路由同步、媒体分页 + 无限滚动哨兵、筛选表单、三种布局（瀑布流跨行计算、网格、时间流月份分组 + 锚点索引条）、多选下载、`task:file` 状态回填、十余个格式化工具。左右面板逻辑几乎零耦合，是典型可拆分边界。

修复建议：拆出 `DialogListPanel.vue`（左面板整体）、`MediaToolbar.vue`；分页/竞态逻辑抽 `useMediaPager(dialogId)` composable（顺带解决 FE-01 竞态）；瀑布流计算抽 `useWaterfall`。**一次拆分同时消解竞态（FE-01）、上帝组件、右面板状态缓存断裂（FE-16）三个问题。**

---

## FE-12 [Medium] 脚本删除无确认、切换/新建丢弃未保存修改

- 类别：UX / 危险操作
- 涉及文件：`src/frontend/src/pages/ScriptsPage.vue`（L130-141）

删除脚本无确认弹窗，单击"删除"立即执行且不可恢复——而 TasksPage/SettingsPage 的破坏性操作都用了 `ConfirmDialog`，规范不一致。另外切换脚本、点"新建"时不检查当前编辑器是否有未保存修改，直接覆盖 `source`，用户改动静默丢失。建议删除走 `confirm.require`；维护 `dirty` 标记，切换/新建前弹确认。

---

## FE-13 [Medium] build 脚本无 vue-tsc 类型检查、lint 体系缺失

- 类别：工程质量
- 涉及文件：`src/frontend/package.json`（L6-10）、`src/frontend/src/api.ts`（L22）、`src/frontend/src/pages/LogsPage.vue`（L89）

`build` 脚本只有 `vite build`，**没有 `vue-tsc --noEmit` 类型检查**——tsconfig 的 `strict: true` 形同虚设，模板与 TS 的类型错误不会阻断构建。同时 `api.ts` 与 LogsPage 存在 `eslint-disable` 注释，但 devDependencies 中**没有任何 ESLint 依赖与配置**，这些注释是死注释。建议 `"build": "vue-tsc --noEmit && vite build"`（加 `vue-tsc` 依赖）；引入 eslint + eslint-plugin-vue 或删除误导性注释。

---

## FE-14 [Medium] 硬编码颜色值违反 CSS 变量规范

- 类别：主题 / 代码质量
- 涉及文件：见下清单

项目规范明确"颜色一律使用 PrimeVue 语义 token，禁止硬编码 hex"。违规点：
- `ChatsPage.vue` L963 `.dialog-avatar { color: #fff }`、L1310 `.dl-state { color: #fff; background: rgb(0 0 0 / 55%) }`、L1268 `.play-badge { color: rgb(255 255 255 / 90%) }`
- `LoginPage.vue` L338 `.qr-wrap canvas { background: #fff }`（二维码白底属功能必需，可豁免但应注释说明）
- `MediaPreview.vue` L285-443 整个 lightbox 约 8 处 `rgb(0 0 0 / 88%)`、`#fff`、`rgb(255 255 255 / 65%)` 等（暗色遮罩场景可辩护，但按规范应至少定义为局部 CSS 变量如 `--mp-overlay-bg`）

建议：语义色替换为 `var(--p-surface-0)`/`var(--p-primary-contrast-color)` 等 token；遮罩类颜色集中为组件级自定义属性并注释豁免理由。

---

## FE-15 [Medium] fmtSize 等格式化函数 4 处重复 + 搜索框模板 3 处重复

- 类别：代码质量 / 重复
- 涉及文件：`ChatsPage.vue` L785、`TasksPage.vue` L342、`MediaPreview.vue` L258、`LogsPage.vue` L342

`fmtSize` 三处逐字节相同，LogsPage 又有第四个不同实现（同一数据两页显示格式可能不一致）；`kindIcon`、`fmtDate`、`pad` 同样重复；带清除图标的搜索框 HTML 在 ChatsPage×2 + LogsPage 重复三次。建议新建 `src/utils/format.ts` 统一导出；搜索框抽 `SearchBox.vue`。

---

## FE-16 [Low] 右面板媒体状态无缓存，切页即丢

- 类别：Store 边界 / UX
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（L419-440）

媒体列表（items/offset/筛选条件/多选）全部是组件本地状态，离开"对话"页再回来全部丢失、重新拉取（对话列表在 store 有缓存，右面板没有）。属设计取舍而非 bug，但与左面板缓存策略不一致，重型浏览场景体验断裂。若拆 `useMediaPager`，可顺带以 dialogId 为 key 做 LRU 缓存。

---

## FE-17 [Low] store 事件订阅返回的取消函数被丢弃（HMR 下双触发）

- 类别：事件订阅 / 生命周期
- 涉及文件：`src/frontend/src/stores/{auth,tasks,scripts,logs}.ts`（各自 `init()`）

四个 store 在 `init()` 中 `on(...)` 订阅后丢弃返回的取消函数。因 store 是应用级单例且有 `inited` 防重入，正常运行无泄漏；但 `wails dev` 下 Vite HMR 重建模块时旧回调无法注销，会出现事件双触发（热更后任务进度跳动/日志重复），需刷新页面恢复。建议保存取消函数，并在 `import.meta.hot?.dispose` 中注销。

---

## FE-18 [Low] LoginPage detect()/QRCode.toCanvas 缺错误处理

- 类别：错误处理
- 涉及文件：`src/frontend/src/pages/LoginPage.vue`（`detect` L210-218、`QRCode.toCanvas` L200；`startCode`/`startQR` catch）

`detect()` 无 try/catch（`Auth.detectDesktopPath` 抛错即未捕获）；`QRCode.toCanvas` 返回 Promise 未处理。另外 `startCode`/`startQR` 的 catch 里直接 `auth.stage = 'idle'` 从组件改 store state，绕过 action，建议在 store action 内部 catch 后复位。

---

## FE-19 [Low] auth store 的 pending 态无超时兜底

- 类别：业务逻辑
- 涉及文件：`src/frontend/src/stores/auth.ts`（L71-80）

`stage = 'pending'` 后若后端既不 reject 也不发 `login:update` 事件（如网络挂起），stage 永久停在 pending，发送按钮永久转圈，无超时兜底。建议加超时复位或允许 pending 态下取消。

---

## FE-20 [Low] jumpMonth 归零时 DatePicker 显示值不清

- 类别：业务逻辑
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（L562-567）

月份跳转后执行 `applyFilters`/切换对话会经 `resetMedia` 把 `jumpOffsetDate` 归零，但 `jumpMonth`（DatePicker 显示值）不清空，UI 显示已跳转到某月而实际查询从最新开始；且重选同一月份不会触发 watch。建议 `resetMedia` 同时清 `jumpMonth`。

---

## FE-21 [Low] caseSensitive 声明后无 UI 入口（死代码或功能缺失）

- 类别：无用代码
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（L333）

`const caseSensitive = ref(false)` 声明后无任何 UI 入口修改（对比 LogsPage 有 Aa 按钮），对话正则永远不区分大小写——要么漏了按钮（功能缺失），要么是死代码。建议补 ToggleButton 或删除。

---

## FE-22 [Low] LogsPage matcher computed 内产生副作用

- 类别：反模式
- 涉及文件：`src/frontend/src/pages/LogsPage.vue`（`matcher` L163-177）

`matcher` computed 内部写 `regexError.value`——computed 中产生副作用，依赖求值时机，Vue 官方不推荐。建议改为 `regexError` 也做成基于 `query/useRegex` 的 computed，或用 `watchEffect`。

---

## FE-23 [Low] LogsPage rows computed 全量正则解析无防抖

- 类别：性能
- 涉及文件：`src/frontend/src/pages/LogsPage.vue`（`rows` L195-230）

`rows` computed 对最多 5000 条日志逐条正则解析 + 拼 HTML 字符串，`query` 每敲一个字符全量重算（无防抖），实时日志每来一批也全量重建。数据量顶格时输入卡顿。建议搜索输入 200ms 防抖；html 渲染下沉到 `#item` 模板按需生成。

---

## FE-24 [Low] 瀑布流魔法数字与 CSS 强耦合

- 类别：魔法数字
- 涉及文件：`src/frontend/src/pages/ChatsPage.vue`（L627-641）

瀑布流计算中 `162`、`12`、`52`、`20`、`8` 与 CSS 的 `minmax(150px,1fr)`、`gap:12px`、`grid-auto-rows:8px` 强耦合，CSS 改动即静默错位。建议常量化并与 CSS 变量共享来源。其余魔法数字（`rootMargin: '600px'`、滚动底部阈值 `40`、`setTimeout 100`）建议命名常量。

---

## FE-25 [Low] TasksPage 展开态 record 在任务移除后不清理

- 类别：内存 / 状态残留
- 涉及文件：`src/frontend/src/pages/TasksPage.vue`（L183-186）

`expanded/loadingFiles/fileLists/selectedPaths` 四个 record 在任务被移除后不清理对应 key，页面生命周期内缓慢累积；且 `fileLists` 不随任务重跑刷新。影响很小，建议在删除路径统一清理。

---

## FE-26 [Low] MediaPreview 末项"下一个"与 items 清空的边界

- 类别：UX
- 涉及文件：`src/frontend/src/components/MediaPreview.vue`（L194-204）

在最后一项按"下一个"时只 `emit('load-more')` 不前进，数据到达后需再按一次；加载期间无 loading 提示，体感是"按了没反应"。另外预览打开时若外部 `items` 被清空（如登出触发 `resetMedia`），`item` 变 undefined，遮罩仍显示且顶部信息为空。建议 load-more 后待 items 增长自动前进；`items` 清空时自动关闭预览。

---

## 主题系统符合性核对表

| 规范要求 | 现状 | 结论 |
|---|---|---|
| 默认跟随系统 `prefers-color-scheme` | theme.ts 默认 `system` + `matchMedia` 监听 | 符合 |
| 手动切换持久化 localStorage | `setTheme` 写入 `tdlui-theme` | 符合 |
| CSS 变量适配 | 绝大多数用 `--p-*` token，`darkModeSelector` 配置正确；例外见 FE-14 | 局部违规 |
| ToggleSwitch 形式切换 | 侧栏图标按钮 + 设置页三选 SelectButton，均非 ToggleSwitch | 不符合（FE-09） |
| 状态同步 | 侧栏切换与 settings store 脱节 | 不符合（FE-08） |

---

## 正面结论（前端，勿误改）

- `LogsPage` 的 `v-html` 前分段 HTML 转义 + `<mark>` 高亮实现严谨，无注入风险；零宽匹配死循环也有防护（L256-259）。
- `logs.ts` 初始快照与事件流按 `seq` 去重合并，考虑了订阅/快照的时间窗重叠。
- ChatsPage 的 `IntersectionObserver`/`ResizeObserver`/事件退订在 `onBeforeUnmount` 成对清理；连续空页上限 `MAX_EMPTY_PAGES` 防筛选假死；正则编译错误以 `undefined` 哨兵值无副作用地传给模板提示。
- 大列表均用 `VirtualScroller`；缩略图走 HTTP 通道利用浏览器缓存与并发控制，内嵌模糊图作占位的渐进加载设计合理。
- 破坏性操作（删除文件/清缓存）在 Tasks/Settings 页有 ConfirmDialog（Scripts 页除外，见 FE-12）。
