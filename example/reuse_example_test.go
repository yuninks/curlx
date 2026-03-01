package example

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yuninks/curlx"
)

func TestConnectionReuse(t *testing.T) {
	// 创建带优化连接池配置的客户端
	client := curlx.NewCurlx(
		curlx.WithMaxIdleConns(50),
		curlx.WithMaxIdleConnsPerHost(10),
		curlx.WithMaxConnsPerHost(20),
		curlx.WithIdleConnTimeout(60*time.Second),
		curlx.WithOptionTimeOut(30*time.Second),
	)

	// 测试同一个主机的多次请求，观察连接复用效果
	targetURL := "https://httpbin.org/get"

	fmt.Println("=== 连接复用测试开始 ===")

	// 预热连接
	fmt.Println("1. 预热连接...")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := client.Get(ctx1, targetURL)
	cancel1()
	if err != nil {
		t.Logf("预热请求失败: %v", err)
		return
	}
	fmt.Println("   预热完成")

	// 连续请求测试
	fmt.Println("\n2. 连续请求测试...")
	for i := 1; i <= 5; i++ {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		response, err := client.Get(ctx, targetURL)
		duration := time.Since(start)

		cancel()

		if err != nil {
			t.Logf("第%d次请求失败: %v (耗时: %v)", i, err, duration)
		} else {
			t.Logf("第%d次请求成功: %d字节 (耗时: %v)", i, len(response), duration)
		}

		time.Sleep(200 * time.Millisecond) // 短暂间隔
	}

	fmt.Println("\n=== 测试完成 ===")
}

func TestConcurrentConnectionReuse(t *testing.T) {
	// 创建连接池管理器
	poolManager := NewConnectionPoolManager(
		curlx.WithMaxIdleConns(100),
		curlx.WithMaxIdleConnsPerHost(20),
		curlx.WithMaxConnsPerHost(30),
		curlx.WithIdleConnTimeout(120*time.Second),
	)

	// 测试并发请求
	urls := []string{
		"https://httpbin.org/get",
		"https://httpbin.org/uuid",
		"https://httpbin.org/user-agent",
		"https://httpbin.org/headers",
		"https://httpbin.org/ip",
	}

	fmt.Println("=== 并发连接复用测试 ===")
	poolManager.ConcurrentRequests(urls)
}

func TestPersistentConnection(t *testing.T) {
	// 创建优化的客户端
	client := curlx.NewCurlx(
		curlx.WithMaxIdleConns(30),
		curlx.WithMaxIdleConnsPerHost(5),
		curlx.WithMaxConnsPerHost(15),
		curlx.WithIdleConnTimeout(30*time.Second),
	)

	manager := &ConnectionPoolManager{
		client: client,
		// transport: client.transport,
	}

	fmt.Println("=== 持久连接测试 ===")
	manager.PersistentConnectionExample("https://httpbin.org/delay/1")
}

func Example_connectionReuse() {
	// 最佳实践示例：如何正确配置连接复用

	// 1. 创建优化配置的客户端
	client := curlx.NewCurlx(
		// 连接池配置
		curlx.WithMaxIdleConns(100),               // 总空闲连接数
		curlx.WithMaxIdleConnsPerHost(10),         // 每主机空闲连接数
		curlx.WithMaxConnsPerHost(50),             // 每主机最大连接数
		curlx.WithIdleConnTimeout(90*time.Second), // 空闲超时时间

		// 其他优化配置
		curlx.WithOptionTimeOut(30*time.Second),
	)

	// 2. 复用同一个客户端实例进行多次请求
	ctx := context.Background()

	// 第一次请求会建立新连接
	response1, err := client.Get(ctx, "https://httpbin.org/get")
	if err != nil {
		fmt.Printf("首次请求失败: %v\n", err)
		return
	}
	fmt.Printf("首次请求成功: %d字节\n", len(response1))

	// 后续请求会复用已有连接
	response2, err := client.Get(ctx, "https://httpbin.org/uuid")
	if err != nil {
		fmt.Printf("第二次请求失败: %v\n", err)
		return
	}
	fmt.Printf("第二次请求成功: %d字节\n", len(response2))

	// 3. 查看连接池状态
	manager := &ConnectionPoolManager{
		client:    client,
		// transport: client.transport,
	}
	manager.PrintPoolStats()

	// Output:
	// 首次请求成功: [字节数]
	// 第二次请求成功: [字节数]
	// === 连接池统计 ===
	// 当前空闲连接数: 1
	// 总连接数: 1
	// 等待队列长度: 0
}
