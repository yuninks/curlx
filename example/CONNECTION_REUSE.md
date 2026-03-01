# HTTP连接复用最佳实践

## 什么是连接复用？

HTTP连接复用（Connection Reuse）是指在同一个HTTP客户端实例中，对相同目标主机的多个请求复用已建立的TCP连接，而不是为每个请求都创建新的连接。这可以显著提高性能并减少资源消耗。

## 为什么需要连接复用？

1. **性能提升**：避免重复的TCP三次握手和TLS握手
2. **资源节约**：减少系统文件描述符和内存使用
3. **降低延迟**：复用已建立的连接减少连接建立时间
4. **服务器友好**：减少服务器连接压力

## curlx中的连接复用配置

### 基本配置参数

```go
client := NewCurlx(
    // 连接池大小配置
    WithMaxIdleConns(100),        // 总空闲连接数上限
    WithMaxIdleConnsPerHost(10),  // 每个主机的空闲连接数
    WithMaxConnsPerHost(50),      // 每个主机的最大连接数
    WithIdleConnTimeout(90*time.Second), // 空闲连接超时时间
    
    // 其他优化配置
    SetOptionTimeOut(30*time.Second),
)
```

### 参数详解

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `MaxIdleConns` | 100 | 连接池中保持的最大空闲连接总数 |
| `MaxIdleConnsPerHost` | 10 | 对每个主机保持的最大空闲连接数 |
| `MaxConnsPerHost` | 50 | 对每个主机允许的最大并发连接数 |
| `IdleConnTimeout` | 90s | 空闲连接的超时时间 |

## 最佳实践

### 1. 正确使用单例模式

```go
// ❌ 错误做法：每次请求都创建新客户端
func badExample() {
    for i := 0; i < 100; i++ {
        client := NewCurlx() // 每次都新建，无法复用连接
        client.Get(context.Background(), "https://example.com")
    }
}

// ✅ 正确做法：复用客户端实例
func goodExample() {
    client := NewCurlx( // 只创建一次
        WithMaxIdleConns(50),
        WithMaxIdleConnsPerHost(5),
    )
    
    for i := 0; i < 100; i++ {
        client.Get(context.Background(), "https://example.com") // 复用连接
    }
}
```

### 2. 合理设置连接池大小

```go
// 根据应用场景调整配置
func getConfigForScenario(scenario string) []Option {
    switch scenario {
    case "high_concurrency":
        return []Option{
            WithMaxIdleConns(200),
            WithMaxIdleConnsPerHost(20),
            WithMaxConnsPerHost(100),
        }
    case "low_resource":
        return []Option{
            WithMaxIdleConns(20),
            WithMaxIdleConnsPerHost(2),
            WithMaxConnsPerHost(10),
        }
    default:
        return []Option{
            WithMaxIdleConns(100),
            WithMaxIdleConnsPerHost(10),
            WithMaxConnsPerHost(50),
        }
    }
}
```

### 3. 监控连接池状态

```go
func monitorConnectionPool(client *Curlx) {
    manager := &ConnectionPoolManager{
        client: client, 
        transport: client.transport,
    }
    
    // 定期检查连接池状态
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        manager.PrintPoolStats()
    }
}
```

## 性能对比测试

```go
func BenchmarkConnectionReuse(b *testing.B) {
    client := NewCurlx(
        WithMaxIdleConns(50),
        WithMaxIdleConnsPerHost(10),
    )
    
    b.Run("with_reuse", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            client.Get(context.Background(), "https://httpbin.org/get")
        }
    })
}

func BenchmarkWithoutReuse(b *testing.B) {
    b.Run("without_reuse", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            client := NewCurlx() // 每次新建客户端
            client.Get(context.Background(), "https://httpbin.org/get")
        }
    })
}
```

## 常见问题解答

### Q: 连接池满了怎么办？
A: 当连接池满时，新的请求会等待空闲连接。可以通过增加`MaxConnsPerHost`来缓解。

### Q: 如何清理空闲连接？
A: 空闲连接会在`IdleConnTimeout`后自动关闭，也可以手动调用`transport.CloseIdleConnections()`。

### Q: 不同主机的连接是否共享？
A: 不同主机的连接是隔离的，每个主机维护自己的连接池。

### Q: HTTPS连接也能复用吗？
A: 是的，HTTPS连接同样支持复用，包括TLS会话复用。

## 调试技巧

```go
// 启用详细的HTTP跟踪
import "net/http/httptrace"

func debugWithTrace() {
    trace := &httptrace.ClientTrace{
        GotConn: func(info httptrace.GotConnInfo) {
            fmt.Printf("连接复用: %v, 来自空闲池: %v\n", 
                info.Reused, info.WasIdle)
        },
        ConnectStart: func(network, addr string) {
            fmt.Printf("开始连接: %s %s\n", network, addr)
        },
        ConnectDone: func(network, addr string, err error) {
            if err != nil {
                fmt.Printf("连接完成: %v\n", err)
            }
        },
    }
    
    ctx := httptrace.WithClientTrace(context.Background(), trace)
    client := NewCurlx()
    client.Get(ctx, "https://httpbin.org/get")
}
```

## 生产环境建议

1. **预热连接**：应用启动时进行连接预热
2. **监控指标**：监控连接池使用率和错误率
3. **优雅关闭**：应用关闭时清理连接资源
4. **负载均衡**：考虑使用连接池配合负载均衡

```go
// 生产环境推荐配置
func productionConfig() *Curlx {
    return NewCurlx(
        WithMaxIdleConns(200),
        WithMaxIdleConnsPerHost(20),
        WithMaxConnsPerHost(100),
        WithIdleConnTimeout(120*time.Second),
        SetOptionTimeOut(30*time.Second),
    )
}
```