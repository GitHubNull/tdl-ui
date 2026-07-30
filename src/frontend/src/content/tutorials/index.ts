// 教程章节清单：Markdown 正文经 Vite ?raw 内联打包，离线可用
import basicInstall from './basic-install.md?raw'
import basicLogin from './basic-login.md?raw'
import intermediateDownload from './intermediate-download.md?raw'
import intermediateSettings from './intermediate-settings.md?raw'
import advancedScriptBasics from './advanced-script-basics.md?raw'
import advancedScriptExamples from './advanced-script-examples.md?raw'
import advancedScriptApi from './advanced-script-api.md?raw'
import advancedScriptDebug from './advanced-script-debug.md?raw'

export type TutorialLevel = 'basic' | 'intermediate' | 'advanced'

export interface Chapter {
  /** 路由参数用的章节 id */
  id: string
  level: TutorialLevel
  title: string
  summary: string
  source: string
}

export const LEVEL_LABELS: Record<TutorialLevel, string> = {
  basic: '基础',
  intermediate: '中级',
  advanced: '高级',
}

/** 全部章节，按 基础 → 中级 → 高级 顺序排列 */
export const chapters: Chapter[] = [
  {
    id: 'install',
    level: 'basic',
    title: '安装与启动',
    summary: '下载运行、界面总览、数据目录',
    source: basicInstall,
  },
  {
    id: 'login',
    level: 'basic',
    title: '账号登录',
    summary: '二维码 / 验证码 / Desktop 导入',
    source: basicLogin,
  },
  {
    id: 'download',
    level: 'intermediate',
    title: '创建下载任务',
    summary: '选集下载、消息链接、断点续传',
    source: intermediateDownload,
  },
  {
    id: 'settings',
    level: 'intermediate',
    title: '代理与设置',
    summary: '代理、命名模板、性能参数、主题',
    source: intermediateSettings,
  },
  {
    id: 'script-basics',
    level: 'advanced',
    title: '脚本语法基础与编写规范',
    summary: '脚本结构、契约函数、沙箱白名单',
    source: advancedScriptBasics,
  },
  {
    id: 'script-examples',
    level: 'advanced',
    title: '脚本实战示例',
    summary: '过滤 / 归档命名 / 生命周期统计三个完整案例',
    source: advancedScriptExamples,
  },
  {
    id: 'script-api',
    level: 'advanced',
    title: '内置函数与类型参考',
    summary: 'api.Log/Logf、契约函数、FileInfo/TaskInfo',
    source: advancedScriptApi,
  },
  {
    id: 'script-debug',
    level: 'advanced',
    title: '脚本调试与错误处理',
    summary: '校验试运行、超时熔断、常见错误对照',
    source: advancedScriptDebug,
  },
]

export function findChapter(id: string | undefined): Chapter | undefined {
  return chapters.find((c) => c.id === id)
}
