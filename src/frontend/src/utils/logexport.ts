/** 导出用的结构化日志记录；未能结构化解析的行仅填 raw 与 message。 */
export interface LogRecord {
  time: string
  level: string
  module: string
  /** 文件 · 函数:行号 */
  source: string
  message: string
  /** 原始整行文本 */
  raw: string
}

/** .log / .txt：逐行原文。 */
export function toPlainText(records: LogRecord[]): string {
  return records.map((r) => r.raw).join('\r\n') + (records.length ? '\r\n' : '')
}

/** RFC4180：含逗号、引号、换行时加引号并把引号翻倍。 */
function csvField(s: string): string {
  if (/[",\r\n]/.test(s)) return `"${s.replace(/"/g, '""')}"`
  return s
}

/** .csv：固定表头 time,level,module,source,message。 */
export function toCsv(records: LogRecord[]): string {
  const head = 'time,level,module,source,message'
  const body = records.map((r) =>
    [r.time, r.level, r.module, r.source, r.message].map(csvField).join(','),
  )
  return [head, ...body].join('\r\n') + '\r\n'
}

/** 按扩展名组装导出内容。 */
export function buildContent(records: LogRecord[], ext: string): string {
  return ext === '.csv' ? toCsv(records) : toPlainText(records)
}
