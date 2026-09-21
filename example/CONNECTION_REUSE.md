# HTTP 连接复用最佳实践

本文档中的代码都可以在 [`reuse_example_test.go`](reuse_example_test.go) 与
[`connection_pool_example.go`](connection_pool_example.go) 里找到可运行版本：

```bash
go test ./example/ -run 'Example_connectionReuse|Example_reuseAfterErrorStatus|TestConcurrentRequests' -v
```

## 结论先说

1. **复用客户端实例**是连接复用的前提：`curlx.NewCurlx()` 内部持有连接池。
2. **把响应体读完**（`Get`/`Send` 已自动完成，`SendWithResponse` 需要你读或 `Close`）连接才会回到池子里。
3. 想验证复用是否生效，用 `httptrace` 统计"新建/复用"次数，而不是猜。

## 为什么需要连接复用

- 省掉重复的 TCP 三次握手，HTTPS 还省掉 TLS 握手（新连接才需要 TLS 会话恢复）；
- 更少的文件描述符与内存；
- 更低的尾延迟，对服务端也更友好。

## 配置参数

```go
client := curlx.NewCurlx(
	curlx.WithMaxIdleConns(100),               // 连接池中所有主机的空闲连接总数
	curlx.WithMaxIdleConnsPerHost(10),         // 每个主机保留的空闲连接数
	curlx.WithMaxConnsPerHost(50),             // 每个主机的最大连接数（含正在使用的）
	curlx.WithIdleConnTimeout(90*time.Second), // 空闲连接多久后被关闭
	curlx.WithTimeout(30*time.Second),         // 单次请求总超时
)
```

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `MaxIdleConns` | 100 | 所有主机的空闲连接总数上限 |
| `MaxIdleConnsPerHost` | 10 | 每个主机保留的空闲连接数；并发高时可调大 |
| `MaxConnsPerHost` | 50 | 每个主机的最大连接数，超出的请求会排队等待 |
| `IdleConnTimeout` | 90s | 空闲连接超时，超时后连接被关闭 |
| `DialTimeout` | 30s | 建立 TCP 连接的超时 |
| `KeepAlive` | 30s | TCP keep-alive 探测周期 |
| `Timeout` | 120s | 单次请求（含读 body）的总超时，可用 `SetParamsTimeout` 按请求覆盖 |

## 正确用法

### 1. 复用客户端实例

```go
// ❌ 每次请求都新建客户端：连接池无法复用，等价于短连接
for i := 0; i < 100; i++ {
	curlx.NewCurlx().Get(context.Background(), "https://example.com")
}

// ✅ 客户端只创建一次
client := curlx.NewCurlx(curlx.WithMaxIdleConns(50), curlx.WithMaxIdleConnsPerHost(10))
for i := 0; i < 100; i++ {
	client.Get(context.Background(), "https://example.com")
}
```

需要多套配置时创建多个客户端即可，它们的连接池相互独立
（见 [`Example_separatePools`](reuse_example_test.go)）。

### 2. 按场景调整池大小

```go
func optionsFor(scenario string) []curlx.Option {
	switch scenario {
	case "high_concurrency":
		return []curlx.Option{
			curlx.WithMaxIdleConns(200),
			curlx.WithMaxIdleConnsPerHost(50),
			curlx.WithMaxConnsPerHost(200),
		}
	case "low_resource":
		return []curlx.Option{
			curlx.WithMaxIdleConns(20),
			curlx.WithMaxIdleConnsPerHost(2),
			curlx.WithMaxConnsPerHost(10),
		}
	default:
		return []curlx.Option{
			curlx.WithMaxIdleConns(100),
			curlx.WithMaxIdleConnsPerHost(10),
			curlx.WithMaxConnsPerHost(50),
		}
	}
}
```

`MaxConnsPerHost` 是"限流阀"：设成 4 时，10 个并发 goroutine 也只会建立 4 条连接，
其余请求排队等待复用。可参考 [`TestConcurrentRequests`](reuse_example_test.go)。

**注意默认值的搭配**：默认 `MaxConnsPerHost=50` 而 `MaxIdleConnsPerHost=10`，
意味着一次并发高峰会建立 50 条连接、高峰过后只保留 10 条，其余被回收；
下一波高峰又需要重新建连（拨号抖动）。持续高并发场景建议把
`MaxIdleConnsPerHost` 调到与 `MaxConnsPerHost` 同一量级。

### 3. 优雅关闭

```go
client := curlx.NewCurlx()
defer client.CloseIdleConnections() // 关闭池中空闲连接（不影响进行中的请求）
```

## 验证复用是否真的生效

`net/http` 从 Go 1.10 起不再暴露连接池计数（`IdleConnCount`/`TotalConnCount` 已被移除），
所以旧版本示例里的 `PrintPoolStats` 只能是个空实现。正确做法是用 `httptrace`：

```go
var created, reused int64

trace := &httptrace.ClientTrace{
	GotConn: func(info httptrace.GotConnInfo) {
		if info.Reused {
			atomic.AddInt64(&reused, 1)
			return
		}
		atomic.AddInt64(&created, 1)
	},
}
ctx := httptrace.WithClientTrace(context.Background(), trace)

client := curlx.NewCurlx()
for i := 0; i < 5; i++ {
	client.Get(ctx, "https://example.com/api/user/42")
}
fmt.Println("新建:", created, "复用:", reused) // 新建: 1 复用: 4
```

[`ConnectionPoolManager`](connection_pool_example.go) 就是这套统计的封装，
`Example_connectionReuse` 的期望输出是 `新建连接: 1 / 复用连接: 4`。

## 什么会破坏连接复用

| 行为 | 后果 |
| --- | --- |
| 每次请求都 `NewCurlx()` | 连接池不共享，等于短连接 |
| `SendWithResponse` 后既不读 body 也不 `Close` | 连接被强制关闭，不会回到池中 |
| 只读了 body 的一部分就 `Close`（>32KB 的部分） | 同上（`Close` 只会尽力 drain 32KB） |
| 请求被取消 / 超时 | 连接会被关闭（这是正确行为） |
| 响应带 `Connection: close` | 服务端要求关闭 |
| `IdleConnTimeout` 太短（高并发低 QPS 场景） | 连接在下次请求前就过期了 |

需要注意的是：**非 2xx 响应不会破坏复用**。`Get`/`Send` 即使遇到 500 也会把响应体读完，
因此连接同样会被放回连接池（见 [`Example_reuseAfterErrorStatus`](reuse_example_test.go)）。

## 常见问题

**Q：连接池满了会怎样？**
超出 `MaxConnsPerHost` 的请求会等待空闲连接（受请求超时约束），而不是报错。

**Q：不同主机的连接共享吗？**
空闲连接总数受 `MaxIdleConns` 限制，但每个主机各自有 `MaxIdleConnsPerHost` 约束，互不挤占。

**Q：HTTPS 连接也能复用吗？**
能。复用已有连接时不需要重新握手；新建连接时 `crypto/tls` 会尝试会话恢复。

**Q：为什么并发请求时新建连接数比 `MaxConnsPerHost` 小很多？**
因为请求很快结束并归还连接，后续请求直接复用。实际峰值连接数取决于并发度与响应耗时。

**Q：`Transport()` 能拿到什么？**
返回底层 `http.RoundTripper`，可用于观测；但不要修改它——配置都由 `Option` 在构造时确定。

## 生产环境建议

1. **客户端单例**：进程内全局一个客户端（或按目标站点分组）。
2. **监控新建/复用比**：复用率长期偏低通常意味着池配置或用法有问题。
3. **预热**：启动时对关键下游发一次请求，避免首个真实请求承担握手成本。
4. **合理超时**：`Timeout` 应大于下游 P99 耗时，`IdleConnTimeout` 不要小于请求间隔。
