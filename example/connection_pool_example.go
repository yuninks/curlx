package example

import (
	"context"
	"fmt"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/yuninks/curlx"
)

// ConnectionPoolManager 演示如何在业务代码里观测连接复用情况。
//
// 说明：net/http 从 Go 1.10 起不再暴露连接池计数（旧版本示例里的
// IdleConnCount/TotalConnCount 已被移除），因此这里用 httptrace 统计
// "新建连接"与"复用连接"的次数，这比打印池内计数更能说明问题。
type ConnectionPoolManager struct {
	client *curlx.Curlx

	mu      sync.Mutex
	created int
	reused  int
}

// NewConnectionPoolManager 创建一个带默认连接池配置的客户端。
//
// 默认配置放在最前面，调用方传入的 Option 在最后生效，因此可以被覆盖。
func NewConnectionPoolManager(opts ...curlx.Option) *ConnectionPoolManager {
	// 注意：不要写成 append(opts, ...)，那样可能覆盖调用方切片的底层数组
	poolOpts := make([]curlx.Option, 0, len(opts)+1)
	poolOpts = append(poolOpts, WithConnectionPoolSettings(
		100,            // 连接池总空闲连接数
		10,             // 每个主机的空闲连接数
		50,             // 每个主机的最大连接数
		90*time.Second, // 空闲连接超时
	))
	poolOpts = append(poolOpts, opts...)
	return &ConnectionPoolManager{client: curlx.NewCurlx(poolOpts...)}
}

// Client 返回底层客户端，便于在 manager.Get 之外直接发起请求（POST、上传等）。
func (m *ConnectionPoolManager) Client() *curlx.Curlx { return m.client }

// Context 返回一个带 httptrace 的 context，用于统计连接的新建与复用。
func (m *ConnectionPoolManager) Context(ctx context.Context) context.Context {
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if info.Reused {
				m.reused++
				return
			}
			m.created++
		},
	}
	return httptrace.WithClientTrace(ctx, trace)
}

// Get 发起一次 GET，并统计本次使用的连接。
func (m *ConnectionPoolManager) Get(ctx context.Context, url string) ([]byte, error) {
	return m.client.Get(m.Context(ctx), url)
}

// Stats 返回 (新建连接数, 复用连接数)。
func (m *ConnectionPoolManager) Stats() (created, reused int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.created, m.reused
}

// PrintStats 打印连接统计。
func (m *ConnectionPoolManager) PrintStats() {
	created, reused := m.Stats()
	fmt.Println("=== 连接池统计 ===")
	fmt.Println("新建连接:", created)
	fmt.Println("复用连接:", reused)
}

// CloseIdleConnections 释放连接池中的空闲连接（进程退出前或配置变更后调用）。
func (m *ConnectionPoolManager) CloseIdleConnections() {
	m.client.CloseIdleConnections()
}

// WithConnectionPoolSettings 演示如何自定义一个 curlx.Option：
// Option 就是 func(*curlx.ClientOptions)，可以直接访问并修改配置字段。
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
