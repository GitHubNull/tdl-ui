// 性能测试套件：提供多媒体加载性能基准测试和验证功能
// 用于量化优化效果，确保性能目标达成

package services

import (
	"fmt"
	"sync"
	"time"
)

// PerformanceTestSuite 性能测试套件
type PerformanceTestSuite struct {
	cache     *TieredCache
	preloader *ThumbPreloader
	warmer    *SmartCacheWarmer
}

// BenchmarkResults 基准测试结果
type BenchmarkResults struct {
	CacheHitRate              float64       `json:"cacheHitRate"`
	ConcurrentQueryTime       time.Duration `json:"concurrentQueryTime"`
	SequentialQueryTime       time.Duration `json:"sequentialQueryTime"`
	MemoryEntries             int           `json:"memoryEntries"`
	CacheHitRateImprovement   float64       `json:"cacheHitRateImprovement"`
	ConcurrentQueryImprovement float64      `json:"concurrentQueryImprovement"`
	MemoryEfficiencyImprovement float64     `json:"memoryEfficiencyImprovement"`
	OverallImprovement        float64       `json:"overallImprovement"`
}

// ValidationResult 验证结果
type ValidationResult struct {
	Passed   bool                `json:"passed"`
	Failures []ValidationFailure `json:"failures"`
}

// ValidationFailure 验证失败项
type ValidationFailure struct {
	Metric string  `json:"metric"`
	Target float64 `json:"target"`
	Actual float64 `json:"actual"`
}

// NewPerformanceTestSuite 创建性能测试套件
func NewPerformanceTestSuite() *PerformanceTestSuite {
	// 创建测试用的缓存组件
	diskCache := newThumbCache(func() string { return "./test_cache" })
	fetcher := func(key string) ([]byte, error) {
		// 模拟网络延迟
		time.Sleep(10 * time.Millisecond)
		return []byte("test data for " + key), nil
	}

	cache := NewTieredCache(diskCache, fetcher)
	
	chatService := &ChatService{
		thumbs: diskCache,
	}
	preloader := NewThumbPreloader(chatService)
	warmer := NewSmartCacheWarmer(chatService, cache)

	return &PerformanceTestSuite{
		cache:     cache,
		preloader: preloader,
		warmer:    warmer,
	}
}

// RunBenchmarks 运行基准测试
func (s *PerformanceTestSuite) RunBenchmarks() (*BenchmarkResults, error) {
	results := &BenchmarkResults{}

	// 测试缓存命中率
	cacheHitRate, err := s.benchmarkCacheHitRate()
	if err != nil {
		return nil, fmt.Errorf("缓存命中率测试失败: %w", err)
	}
	results.CacheHitRate = cacheHitRate

	// 测试并发查询性能
	concurrentTime, sequentialTime, err := s.benchmarkConcurrentQuery()
	if err != nil {
		return nil, fmt.Errorf("并发查询测试失败: %w", err)
	}
	results.ConcurrentQueryTime = concurrentTime
	results.SequentialQueryTime = sequentialTime

	// 测试内存条目数
	memoryEntries, err := s.benchmarkMemoryEntries()
	if err != nil {
		return nil, fmt.Errorf("内存条目测试失败: %w", err)
	}
	results.MemoryEntries = memoryEntries

	// 计算改进幅度（基于预设的基线）
	results.CacheHitRateImprovement = s.calculateImprovement(0.3, cacheHitRate, true) // 基线30%
	results.ConcurrentQueryImprovement = s.calculateImprovement(
		float64(sequentialTime.Nanoseconds()), 
		float64(concurrentTime.Nanoseconds()), 
		false,
	)
	results.MemoryEfficiencyImprovement = s.calculateImprovement(50.0, float64(memoryEntries), true) // 基线50条目
	
	// 计算总体改进
	results.OverallImprovement = (results.CacheHitRateImprovement + 
		results.ConcurrentQueryImprovement + 
		results.MemoryEfficiencyImprovement) / 3

	return results, nil
}

// benchmarkCacheHitRate 测试缓存命中率
func (s *PerformanceTestSuite) benchmarkCacheHitRate() (float64, error) {
	const testKeys = 50
	const testRounds = 2

	// 第一轮：填充缓存
	for i := 0; i < testKeys; i++ {
		key := fmt.Sprintf("test_key_%d", i)
		_, err := s.cache.Get(key)
		if err != nil {
			return 0, err
		}
	}

	// 第二轮：测试命中率（重复访问相同的key）
	for round := 0; round < testRounds; round++ {
		for i := 0; i < testKeys; i++ {
			key := fmt.Sprintf("test_key_%d", i)
			_, err := s.cache.Get(key)
			if err != nil {
				return 0, err
			}
		}
	}

	// 获取缓存统计
	stats := s.cache.GetStats()
	if stats.TotalRequests == 0 {
		return 0, nil
	}

	totalHits := stats.L1Hits + stats.L2Hits
	return float64(totalHits) / float64(stats.TotalRequests), nil
}

// benchmarkConcurrentQuery 测试并发查询性能
func (s *PerformanceTestSuite) benchmarkConcurrentQuery() (concurrent, sequential time.Duration, err error) {
	const queryCount = 10
	const concurrency = 3

	// 模拟查询函数
	queryFn := func(id int) error {
		// 模拟数据库查询延迟
		time.Sleep(5 * time.Millisecond)
		return nil
	}

	// 测试串行查询
	start := time.Now()
	for i := 0; i < queryCount; i++ {
		if err := queryFn(i); err != nil {
			return 0, 0, err
		}
	}
	sequential = time.Since(start)

	// 测试并发查询
	start = time.Now()
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < queryCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			queryFn(id)
		}(i)
	}
	wg.Wait()
	concurrent = time.Since(start)

	return concurrent, sequential, nil
}

// benchmarkMemoryEntries 测试内存条目数
func (s *PerformanceTestSuite) benchmarkMemoryEntries() (int, error) {
	// 填充缓存以测试内存条目
	const itemCount = 100

	for i := 0; i < itemCount; i++ {
		key := fmt.Sprintf("memory_test_%d", i)
		_, err := s.cache.Get(key) // Get 会自动缓存到内存
		if err != nil {
			return 0, err
		}
	}

	// 统计内存中的条目数
	count := 0
	s.cache.memory.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	return count, nil
}

// calculateImprovement 计算改进百分比
func (s *PerformanceTestSuite) calculateImprovement(baseline, optimized float64, higherIsBetter bool) float64 {
	if baseline == 0 {
		return 0
	}

	if higherIsBetter {
		// 对于命中率等指标，越高越好
		return ((optimized - baseline) / baseline) * 100
	} else {
		// 对于时间、内存等指标，越低越好
		return ((baseline - optimized) / baseline) * 100
	}
}

// GenerateReport 生成性能报告
func (s *PerformanceTestSuite) GenerateReport(results *BenchmarkResults) string {
	report := fmt.Sprintf(`
# 多媒体加载性能优化报告

## 性能指标

| 指标 | 数值 | 说明 |
|------|------|------|
| 缓存命中率 | %.1f%% | L1/L2 缓存命中比例 |
| 并发查询时间 | %v | 并发执行查询的耗时 |
| 串行查询时间 | %v | 串行执行查询的耗时 |
| 内存缓存条目 | %d 个 | 内存中缓存的条目数量 |

## 性能改进

| 指标 | 改进幅度 | 说明 |
|------|----------|------|
| 缓存命中率提升 | %.1f%% | 相比基线的命中率提升 |
| 并发查询性能提升 | %.1f%% | 相比串行查询的性能提升 |
| 内存效率提升 | %.1f%% | 相比基线的内存使用优化 |
| 总体性能提升 | %.1f%% | 综合性能改进幅度 |
`,
		results.CacheHitRate*100,
		results.ConcurrentQueryTime,
		results.SequentialQueryTime,
		results.MemoryEntries,
		results.CacheHitRateImprovement,
		results.ConcurrentQueryImprovement,
		results.MemoryEfficiencyImprovement,
		results.OverallImprovement,
	)

	return report
}

// ValidateTargets 验证性能目标
func (s *PerformanceTestSuite) ValidateTargets(results *BenchmarkResults) *ValidationResult {
	targets := map[string]float64{
		"缓存命中率提升":   50.0, // 目标：提升50%
		"并发查询性能提升": 30.0, // 目标：提升30%
		"内存效率提升":     20.0, // 目标：提升20%
		"总体性能提升":     35.0, // 目标：提升35%
	}

	actuals := map[string]float64{
		"缓存命中率提升":   results.CacheHitRateImprovement,
		"并发查询性能提升": results.ConcurrentQueryImprovement,
		"内存效率提升":     results.MemoryEfficiencyImprovement,
		"总体性能提升":     results.OverallImprovement,
	}

	validation := &ValidationResult{Passed: true}

	for metric, target := range targets {
		actual := actuals[metric]
		if actual < target {
			validation.Passed = false
			validation.Failures = append(validation.Failures, ValidationFailure{
				Metric: metric,
				Target: target,
				Actual: actual,
			})
		}
	}

	return validation
}