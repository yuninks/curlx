package example

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/yuninks/curlx"
)

// ConnectionPoolManager 连接池管理器
type ConnectionPoolManager struct {
	client    *curlx.Curlx
	transport *http.Transport
}

// NewConnectionPoolManager 创建连接池管理器
func NewConnectionPoolManager(opts ...curlx.Option) *ConnectionPoolManager {
	// 添加连接池优化配置
	poolOpts := append(opts,
		WithConnectionPoolSettings(
			100,            // MaxIdleConns
			10,             // MaxIdleConnsPerHost
			50,             // MaxConnsPerHost
			90*time.Second, // IdleConnTimeout
		),
	)

	client := curlx.NewCurlx(poolOpts...)

	return &ConnectionPoolManager{
		client:    client,
		// transport: client.transport,
	}
}

// WithConnectionPoolSettings 连接池配置选项
func WithConnectionPoolSettings(
	maxIdleConns int,
	maxIdleConnsPerHost int,
	maxConnsPerHost int,
	idleConnTimeout time.Duration,
) curlx.Option {
	return func(options *curlx.ClientOptions) {
		options.MaxIdleConns = maxIdleConns
		options.MaxIdleConnsPerHost = maxIdleConnsPerHost
		options.MaxConnsPerHost = maxConnsPerHost
		options.IdleConnTimeout = idleConnTimeout
	}
}

// ConcurrentRequests 并发请求演示
func (cpm *ConnectionPoolManager) ConcurrentRequests(urls []string) {
	var wg sync.WaitGroup
	results := make(chan string, len(urls))

	startTime := time.Now()

	// 并发执行多个请求
	for i, url := range urls {
		wg.Add(1)
		go func(index int, targetURL string) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			start := time.Now()
			response, err := cpm.client.Get(ctx, targetURL)
			duration := time.Since(start)

			if err != nil {
				results <- fmt.Sprintf("Request %d to %s failed: %v (took %v)",
					index, targetURL, err, duration)
			} else {
				results <- fmt.Sprintf("Request %d to %s succeeded: %d bytes (took %v)",
					index, targetURL, len(response), duration)
			}
		}(i, url)
	}

	// 等待所有请求完成
	wg.Wait()
	close(results)

	totalDuration := time.Since(startTime)
	fmt.Printf("=== 并发请求完成 ===\n")
	fmt.Printf("总耗时: %v\n", totalDuration)
	fmt.Printf("平均每个请求: %v\n", totalDuration/time.Duration(len(urls)))

	// 输出结果
	for result := range results {
		fmt.Println(result)
	}

	// 输出连接池状态
	cpm.PrintPoolStats()
}

// PrintPoolStats 打印连接池统计信息
func (cpm *ConnectionPoolManager) PrintPoolStats() {
	// stats := cpm.transport

	fmt.Printf("\n=== 连接池统计 ===\n")
	// fmt.Printf("当前空闲连接数: %d\n", stats.IdleConnCount())
	// fmt.Printf("总连接数: %d\n", stats.TotalConnCount())
	// fmt.Printf("等待队列长度: %d\n", stats.WaitQueueLength())
}

// ReuseExample 连接复用示例
func (cpm *ConnectionPoolManager) ReuseExample(baseURL string, requestCount int) {
	fmt.Printf("=== 连接复用测试 ===\n")
	fmt.Printf("目标URL: %s\n", baseURL)
	fmt.Printf("请求次数: %d\n\n", requestCount)

	startTime := time.Now()

	for i := 0; i < requestCount; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		start := time.Now()
		response, err := cpm.client.Get(ctx, baseURL)
		duration := time.Since(start)

		cancel() // 释放context资源

		if err != nil {
			fmt.Printf("第%d次请求失败: %v (耗时: %v)\n", i+1, err, duration)
		} else {
			fmt.Printf("第%d次请求成功: %d字节 (耗时: %v)\n", i+1, len(response), duration)
		}

		// 小间隔避免请求过于密集
		time.Sleep(100 * time.Millisecond)
	}

	totalDuration := time.Since(startTime)
	fmt.Printf("\n=== 测试完成 ===\n")
	fmt.Printf("总耗时: %v\n", totalDuration)
	fmt.Printf("平均每请求: %v\n", totalDuration/time.Duration(requestCount))

	cpm.PrintPoolStats()
}

// PersistentConnectionExample 持久连接示例
func (cpm *ConnectionPoolManager) PersistentConnectionExample(targetURL string) {
	fmt.Printf("=== 持久连接测试 ===\n")
	fmt.Printf("测试URL: %s\n\n", targetURL)

	// 预热连接
	fmt.Println("预热阶段 - 建立初始连接...")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := cpm.client.Get(ctx1, targetURL)
	cancel1()

	if err != nil {
		fmt.Printf("预热失败: %v\n", err)
		return
	}

	cpm.PrintPoolStats()

	// 实际测试
	fmt.Println("\n实际测试阶段...")
	testStart := time.Now()

	for i := 1; i <= 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		start := time.Now()
		response, err := cpm.client.Get(ctx, targetURL)
		duration := time.Since(start)

		cancel()

		if err != nil {
			fmt.Printf("第%d次请求: 失败 (%v) - 耗时: %v\n", i, err, duration)
		} else {
			fmt.Printf("第%d次请求: 成功 (%d字节) - 耗时: %v\n", i, len(response), duration)
		}

		time.Sleep(500 * time.Millisecond)
	}

	totalTime := time.Since(testStart)
	fmt.Printf("\n持久连接测试完成，总耗时: %v\n", totalTime)
	cpm.PrintPoolStats()
}
