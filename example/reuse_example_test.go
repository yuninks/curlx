package example

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/yuninks/curlx"
)

// Example_connectionReuse 演示连接复用：复用同一个客户端请求同一主机时，
// 只有第一次需要新建 TCP 连接。
func Example_connectionReuse() {
	srv := demoServer()
	defer srv.Close()

	manager := NewConnectionPoolManager(curlx.WithTimeout(10 * time.Second))

	for i := 0; i < 5; i++ {
		if _, err := manager.Get(context.Background(), srv.URL+"/api/user/42"); err != nil {
			fmt.Println("err:", err)
			return
		}
	}
	manager.PrintStats()
	manager.CloseIdleConnections()

	// Output:
	// === 连接池统计 ===
	// 新建连接: 1
	// 复用连接: 4
}

// Example_reuseAfterErrorStatus 演示非 2xx 响应也会被完整读完，连接因此可以继续复用。
//
// 如果只用 SendWithResponse 却既不读 body、也不 drain 就 Close，连接无法复用，
// 每次错误都会新建 TCP 连接（Get/Send 已经替你做完了这件事）。
func Example_reuseAfterErrorStatus() {
	srv := demoServer()
	defer srv.Close()

	manager := NewConnectionPoolManager()

	// ?big=1 让服务端返回 200KB 的错误响应体
	for i := 0; i < 3; i++ {
		_, err := manager.Get(context.Background(), srv.URL+"/api/error?big=1")

		var statusErr *curlx.StatusError
		if errors.As(err, &statusErr) {
			fmt.Printf("status: %d, 错误体截断到 %d 字节\n", statusErr.StatusCode, len(statusErr.Body))
		}
	}
	manager.PrintStats()

	// Output:
	// status: 500, 错误体截断到 512 字节
	// status: 500, 错误体截断到 512 字节
	// status: 500, 错误体截断到 512 字节
	// === 连接池统计 ===
	// 新建连接: 1
	// 复用连接: 2
}

// Example_separatePools 演示不同客户端各自维护连接池，互不复用。
func Example_separatePools() {
	srv := demoServer()
	defer srv.Close()

	first := NewConnectionPoolManager()
	second := NewConnectionPoolManager()

	for _, m := range []*ConnectionPoolManager{first, second} {
		if _, err := m.Get(context.Background(), srv.URL+"/api/user/42"); err != nil {
			fmt.Println("err:", err)
			return
		}
	}

	created1, _ := first.Stats()
	created2, _ := second.Stats()
	fmt.Println("客户端1 新建连接:", created1)
	fmt.Println("客户端2 新建连接:", created2)

	// Output:
	// 客户端1 新建连接: 1
	// 客户端2 新建连接: 1
}

// TestConcurrentRequests 演示并发请求：连接数受 MaxConnsPerHost 约束，
// 请求结束后多余的连接会被回收，不会无限增长。
func TestConcurrentRequests(t *testing.T) {
	srv := demoServer()
	defer srv.Close()

	const (
		workers   = 10
		perWorker = 5
	)

	manager := NewConnectionPoolManager(
		curlx.WithMaxConnsPerHost(4), // 每主机最多 4 条连接，其余请求排队复用
		curlx.WithTimeout(10*time.Second),
	)

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				if _, err := manager.Get(context.Background(), srv.URL+"/api/user/42"); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("并发请求失败: %v", err)
	}

	created, reused := manager.Stats()
	total := created + reused
	if total != workers*perWorker {
		t.Fatalf("请求总数 %d != %d", total, workers*perWorker)
	}
	if created > 4 {
		t.Fatalf("MaxConnsPerHost=4 时新建连接不应超过 4，实际 %d", created)
	}
	if reused == 0 {
		t.Fatal("并发请求应当产生连接复用")
	}
	t.Logf("新建连接 %d，复用连接 %d", created, reused)
	manager.CloseIdleConnections()
}
