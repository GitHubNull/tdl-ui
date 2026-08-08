// 性能监控工具：收集和分析应用性能指标
// 支持性能计时、指标统计、性能报告等功能

interface PerformanceMetric {
  label: string
  duration: number
  timestamp: number
  metadata?: Record<string, any>
}

interface PerformanceStats {
  count: number
  totalTime: number
  averageTime: number
  minTime: number
  maxTime: number
  recentTimes: number[]
}

export class PerformanceMonitor {
  private static metrics = new Map<string, PerformanceMetric[]>()
  private static maxMetricsPerLabel = 100 // 每个标签最多保留100条记录
  private static reportingEnabled = false
  private static reportingEndpoint = ''

  // 开始性能计时
  static startTiming(label: string, metadata?: Record<string, any>): () => void {
    const start = performance.now()
    
    return () => {
      const duration = performance.now() - start
      this.recordMetric(label, duration, metadata)
    }
  }

  // 记录性能指标
  static recordMetric(label: string, duration: number, metadata?: Record<string, any>) {
    if (!this.metrics.has(label)) {
      this.metrics.set(label, [])
    }

    const metrics = this.metrics.get(label)!
    const metric: PerformanceMetric = {
      label,
      duration,
      timestamp: Date.now(),
      metadata
    }

    metrics.push(metric)

    // 限制记录数量
    if (metrics.length > this.maxMetricsPerLabel) {
      metrics.splice(0, metrics.length - this.maxMetricsPerLabel)
    }

    // 上报性能数据（如果启用）
    if (this.reportingEnabled) {
      this.reportMetric(metric)
    }

    // 在开发模式下输出慢操作警告
    if (import.meta.env.DEV && duration > 1000) {
      console.warn(`Slow operation detected: ${label} took ${duration.toFixed(2)}ms`, metadata)
    }
  }

  // 获取性能统计
  static getStats(label: string): PerformanceStats | null {
    const metrics = this.metrics.get(label)
    if (!metrics || metrics.length === 0) {
      return null
    }

    const durations = metrics.map(m => m.duration)
    const totalTime = durations.reduce((sum, d) => sum + d, 0)
    const recentTimes = durations.slice(-10) // 最近10次

    return {
      count: metrics.length,
      totalTime,
      averageTime: totalTime / metrics.length,
      minTime: Math.min(...durations),
      maxTime: Math.max(...durations),
      recentTimes
    }
  }

  // 获取所有标签的统计
  static getAllStats(): Record<string, PerformanceStats> {
    const result: Record<string, PerformanceStats> = {}
    
    for (const [label] of this.metrics) {
      const stats = this.getStats(label)
      if (stats) {
        result[label] = stats
      }
    }
    
    return result
  }

  // 获取平均时间
  static getAverageTime(label: string): number {
    const stats = this.getStats(label)
    return stats ? stats.averageTime : 0
  }

  // 获取最近的性能趋势
  static getRecentTrend(label: string, count: number = 10): number[] {
    const metrics = this.metrics.get(label)
    if (!metrics) return []
    
    return metrics
      .slice(-count)
      .map(m => m.duration)
  }

  // 检查性能是否有回归
  static checkPerformanceRegression(label: string, threshold: number = 1.5): boolean {
    const trend = this.getRecentTrend(label, 20)
    if (trend.length < 10) return false

    const recent = trend.slice(-5)
    const baseline = trend.slice(0, -5)
    
    if (baseline.length === 0) return false

    const recentAvg = recent.reduce((a, b) => a + b, 0) / recent.length
    const baselineAvg = baseline.reduce((a, b) => a + b, 0) / baseline.length

    return recentAvg > baselineAvg * threshold
  }

  // 启用性能上报
  // 安全限制：仅允许同源或本地回环地址，防止数据外泄到第三方
  static enableReporting(endpoint: string) {
    if (!this.isAllowedEndpoint(endpoint)) {
      console.warn('Performance reporting endpoint not allowed:', endpoint)
      return
    }
    this.reportingEnabled = true
    this.reportingEndpoint = endpoint
  }

  // 校验上报端点是否安全
  private static isAllowedEndpoint(endpoint: string): boolean {
    if (!endpoint) return false
    try {
      const url = new URL(endpoint, window.location.origin)
      // 允许同源请求
      if (url.origin === window.location.origin) return true
      // 允许本地回环地址（开发调试）
      if (url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '::1') return true
      // 其他情况一律拒绝
      return false
    } catch {
      return false
    }
  }

  // 禁用性能上报
  static disableReporting() {
    this.reportingEnabled = false
    this.reportingEndpoint = ''
  }

  // 上报性能指标
  private static async reportMetric(metric: PerformanceMetric) {
    if (!this.reportingEndpoint) return

    try {
      // 在 Wails 环境中调用后端方法
      const runtime = (window as any).runtime
      if (runtime?.ReportPerformance) {
        await runtime.ReportPerformance(metric.label, metric.duration)
      } else if (typeof fetch !== 'undefined') {
        // 在 Web 环境中发送到 HTTP 端点
        await fetch(this.reportingEndpoint, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(metric)
        })
      }
    } catch (error) {
      // 静默处理上报失败，不影响应用功能
      console.debug('Performance reporting failed:', error)
    }
  }

  // 清理过期指标
  static cleanup(maxAge: number = 60 * 60 * 1000) { // 默认1小时
    const cutoff = Date.now() - maxAge
    
    for (const [label, metrics] of this.metrics.entries()) {
      const filtered = metrics.filter(m => m.timestamp > cutoff)
      if (filtered.length === 0) {
        this.metrics.delete(label)
      } else {
        this.metrics.set(label, filtered)
      }
    }
  }

  // 清空所有指标
  static clear() {
    this.metrics.clear()
  }

  // 导出性能数据
  static exportData(): Record<string, PerformanceMetric[]> {
    const result: Record<string, PerformanceMetric[]> = {}
    
    for (const [label, metrics] of this.metrics.entries()) {
      result[label] = [...metrics]
    }
    
    return result
  }

  // 生成性能报告
  static generateReport(): string {
    const stats = this.getAllStats()
    const lines = ['Performance Report', '==================', '']
    
    for (const [label, stat] of Object.entries(stats)) {
      lines.push(`${label}:`)
      lines.push(`  Count: ${stat.count}`)
      lines.push(`  Average: ${stat.averageTime.toFixed(2)}ms`)
      lines.push(`  Min: ${stat.minTime.toFixed(2)}ms`)
      lines.push(`  Max: ${stat.maxTime.toFixed(2)}ms`)
      lines.push(`  Total: ${stat.totalTime.toFixed(2)}ms`)
      lines.push('')
    }
    
    return lines.join('\n')
  }
}

// 便捷的装饰器函数（用于 Vue 组件方法）
export function measurePerformance(label: string) {
  return function (target: any, propertyKey: string, descriptor: PropertyDescriptor) {
    const originalMethod = descriptor.value

    descriptor.value = function (...args: any[]) {
      const endTiming = PerformanceMonitor.startTiming(label)
      try {
        const result = originalMethod.apply(this, args)
        
        // 处理异步方法
        if (result instanceof Promise) {
          return result.finally(() => endTiming())
        } else {
          endTiming()
          return result
        }
      } catch (error) {
        endTiming()
        throw error
      }
    }

    return descriptor
  }
}

// Vue 3 Composition API 钩子
export function usePerformanceMonitor() {
  const startTiming = (label: string, metadata?: Record<string, any>) => {
    return PerformanceMonitor.startTiming(label, metadata)
  }

  const recordMetric = (label: string, duration: number, metadata?: Record<string, any>) => {
    PerformanceMonitor.recordMetric(label, duration, metadata)
  }

  const getStats = (label: string) => {
    return PerformanceMonitor.getStats(label)
  }

  const getAllStats = () => {
    return PerformanceMonitor.getAllStats()
  }

  return {
    startTiming,
    recordMetric,
    getStats,
    getAllStats
  }
}
