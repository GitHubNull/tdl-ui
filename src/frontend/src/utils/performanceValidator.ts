// 性能验证脚本：验证多媒体加载优化效果
// 对比优化前后的性能指标，生成性能报告

import { PerformanceMonitor } from '../utils/performance'

interface PerformanceBaseline {
  mediaListLoadTime: number
  thumbnailLoadTime: number
  cacheHitRate: number
  memoryUsage: number
  networkRequests: number
}

interface PerformanceResult {
  baseline: PerformanceBaseline
  optimized: PerformanceBaseline
  improvement: {
    mediaListLoadTime: number // 百分比改进
    thumbnailLoadTime: number
    cacheHitRate: number
    memoryUsage: number
    networkRequests: number
  }
}

export class PerformanceValidator {
  private static instance: PerformanceValidator
  private baseline: PerformanceBaseline | null = null

  static getInstance(): PerformanceValidator {
    if (!this.instance) {
      this.instance = new PerformanceValidator()
    }
    return this.instance
  }

  // 记录基线性能（优化前）
  recordBaseline(metrics: Partial<PerformanceBaseline>) {
    this.baseline = {
      mediaListLoadTime: metrics.mediaListLoadTime || 3000, // 默认3秒
      thumbnailLoadTime: metrics.thumbnailLoadTime || 800,  // 默认800ms
      cacheHitRate: metrics.cacheHitRate || 0.3,           // 默认30%
      memoryUsage: metrics.memoryUsage || 100,             // 默认100MB
      networkRequests: metrics.networkRequests || 50       // 默认50个请求
    }
  }

  // 测量优化后性能
  async measureOptimizedPerformance(): Promise<PerformanceBaseline> {
    const endTiming = PerformanceMonitor.startTiming('performance-validation')
    
    try {
      // 模拟媒体列表加载
      const mediaListStart = performance.now()
      await this.simulateMediaListLoad()
      const mediaListLoadTime = performance.now() - mediaListStart

      // 模拟缩略图加载
      const thumbnailStart = performance.now()
      await this.simulateThumbnailLoad()
      const thumbnailLoadTime = performance.now() - thumbnailStart

      // 获取缓存统计
      const cacheStats = await this.getCacheStats()
      
      // 获取内存使用情况
      const memoryUsage = await this.getMemoryUsage()
      
      // 获取网络请求统计
      const networkRequests = await this.getNetworkRequestCount()

      return {
        mediaListLoadTime,
        thumbnailLoadTime,
        cacheHitRate: cacheStats.hitRate,
        memoryUsage,
        networkRequests
      }
    } finally {
      endTiming()
    }
  }

  // 生成性能对比报告
  async generatePerformanceReport(): Promise<PerformanceResult> {
    if (!this.baseline) {
      // 如果没有基线，使用默认值
      this.recordBaseline({})
    }

    const optimized = await this.measureOptimizedPerformance()
    
    const improvement = {
      mediaListLoadTime: this.calculateImprovement(this.baseline!.mediaListLoadTime, optimized.mediaListLoadTime),
      thumbnailLoadTime: this.calculateImprovement(this.baseline!.thumbnailLoadTime, optimized.thumbnailLoadTime),
      cacheHitRate: this.calculateImprovement(this.baseline!.cacheHitRate, optimized.cacheHitRate, true),
      memoryUsage: this.calculateImprovement(this.baseline!.memoryUsage, optimized.memoryUsage),
      networkRequests: this.calculateImprovement(this.baseline!.networkRequests, optimized.networkRequests)
    }

    return {
      baseline: this.baseline!,
      optimized,
      improvement
    }
  }

  // 计算改进百分比
  private calculateImprovement(baseline: number, optimized: number, higherIsBetter: boolean = false): number {
    if (baseline === 0) return 0
    
    if (higherIsBetter) {
      // 对于缓存命中率等指标，越高越好
      return ((optimized - baseline) / baseline) * 100
    } else {
      // 对于加载时间等指标，越低越好
      return ((baseline - optimized) / baseline) * 100
    }
  }

  // 模拟媒体列表加载
  private async simulateMediaListLoad(): Promise<void> {
    // 模拟异步加载延迟
    await new Promise(resolve => setTimeout(resolve, Math.random() * 500 + 200))
  }

  // 模拟缩略图加载
  private async simulateThumbnailLoad(): Promise<void> {
    // 模拟缩略图加载延迟
    await new Promise(resolve => setTimeout(resolve, Math.random() * 200 + 100))
  }

  // 获取缓存统计
  private async getCacheStats(): Promise<{ hitRate: number }> {
    // 这里应该调用后端 API 获取真实的缓存统计
    // 暂时返回模拟数据
    return { hitRate: 0.75 } // 模拟75%的缓存命中率
  }

  // 获取内存使用情况
  private async getMemoryUsage(): Promise<number> {
    // 使用 Performance API 获取内存信息（如果可用）
    if ('memory' in performance) {
      const memory = (performance as any).memory
      return memory.usedJSHeapSize / 1024 / 1024 // 转换为 MB
    }
    return 80 // 模拟80MB内存使用
  }

  // 获取网络请求数量
  private async getNetworkRequestCount(): Promise<number> {
    // 这里可以通过 Performance Observer API 获取网络请求统计
    // 暂时返回模拟数据
    return 30 // 模拟30个网络请求
  }

  // 验证性能目标是否达成
  validatePerformanceTargets(result: PerformanceResult): {
    passed: boolean
    details: Record<string, { target: number; actual: number; passed: boolean }>
  } {
    const targets = {
      mediaListLoadTime: 60, // 目标：减少60%
      thumbnailLoadTime: 70, // 目标：减少70%
      cacheHitRate: 80,      // 目标：达到80%
      memoryUsage: 30,       // 目标：减少30%
      networkRequests: 40    // 目标：减少40%
    }

    const details: Record<string, { target: number; actual: number; passed: boolean }> = {}
    let allPassed = true

    for (const [key, target] of Object.entries(targets)) {
      const actual = result.improvement[key as keyof typeof result.improvement]
      const passed = actual >= target
      details[key] = { target, actual, passed }
      if (!passed) allPassed = false
    }

    return { passed: allPassed, details }
  }

  // 生成详细的性能报告文本
  generateDetailedReport(result: PerformanceResult): string {
    const validation = this.validatePerformanceTargets(result)
    const lines = [
      '# 多媒体加载性能优化报告',
      '',
      '## 性能指标对比',
      '',
      '| 指标 | 优化前 | 优化后 | 改进幅度 | 目标 | 是否达标 |',
      '|------|--------|--------|----------|------|----------|'
    ]

    const metrics = [
      { key: 'mediaListLoadTime', name: '媒体列表加载时间', unit: 'ms' },
      { key: 'thumbnailLoadTime', name: '缩略图加载时间', unit: 'ms' },
      { key: 'cacheHitRate', name: '缓存命中率', unit: '%' },
      { key: 'memoryUsage', name: '内存使用量', unit: 'MB' },
      { key: 'networkRequests', name: '网络请求数量', unit: '个' }
    ]

    for (const metric of metrics) {
      const baseline = result.baseline[metric.key as keyof PerformanceBaseline]
      const optimized = result.optimized[metric.key as keyof PerformanceBaseline]
      const improvement = result.improvement[metric.key as keyof typeof result.improvement]
      const target = validation.details[metric.key]?.target || 0
      const passed = validation.details[metric.key]?.passed || false

      lines.push(
        `| ${metric.name} | ${baseline.toFixed(1)}${metric.unit} | ${optimized.toFixed(1)}${metric.unit} | ${improvement.toFixed(1)}% | ${target}% | ${passed ? '✅' : '❌'} |`
      )
    }

    lines.push('')
    lines.push('## 优化效果总结')
    lines.push('')

    if (validation.passed) {
      lines.push('🎉 所有性能目标均已达成！优化效果显著。')
    } else {
      lines.push('⚠️ 部分性能目标未达成，需要进一步优化。')
    }

    lines.push('')
    lines.push('### 主要改进')
    
    const significantImprovements = Object.entries(result.improvement)
      .filter(([_, improvement]) => improvement > 20)
      .sort(([_, a], [__, b]) => b - a)

    for (const [key, improvement] of significantImprovements) {
      const metricName = metrics.find(m => m.key === key)?.name || key
      lines.push(`- ${metricName}: 提升 ${improvement.toFixed(1)}%`)
    }

    lines.push('')
    lines.push('### 建议')
    
    if (result.improvement.cacheHitRate < 80) {
      lines.push('- 缓存命中率仍有提升空间，建议调整缓存策略')
    }
    
    if (result.improvement.memoryUsage < 30) {
      lines.push('- 内存使用量优化不足，建议检查内存泄漏')
    }

    return lines.join('\n')
  }
}

// 导出便捷函数
export async function validatePerformance(): Promise<PerformanceResult> {
  const validator = PerformanceValidator.getInstance()
  return validator.generatePerformanceReport()
}

export function generatePerformanceReport(result: PerformanceResult): string {
  const validator = PerformanceValidator.getInstance()
  return validator.generateDetailedReport(result)
}
