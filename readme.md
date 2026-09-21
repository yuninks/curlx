# curlx

基于 `net/http` 封装的网络请求库：更少的样板代码，同时保留超时、连接池、代理、证书固定等生产环境需要的能力。

## 安装

```bash
go get github.com/yuninks/curlx
```

更完整的可运行示例（含连接复用统计、流式上传、代理、证书固定等）见 [example/](example/README.md)，
直接 `go test ./example/ -v` 即可运行：

```bash
go test ./example/ -v
go doc -all github.com/yuninks/curlx/example
```

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yuninks/curlx"
)

func main() {
	// 客户端应当复用（内部维护连接池），不要每次请求都创建
	client := curlx.NewCurlx(
		curlx.WithTimeout(10*time.Second),
		curlx.WithLogger(curlx.NewStdLogger()), // 默认不输出日志
	)

	ctx := context.Background()

	// GET
	body, err := client.Get(ctx, "https://httpbin.org/get")
	fmt.Println(string(body), err)

	// POST JSON
	body, err = client.PostJsonAny(ctx, "https://httpbin.org/post", map[string]any{"name": "yun"})

	// 需要状态码 / 响应头时
	resp := client.SendWithResponse(ctx,
		curlx.SetParamsUrl("https://httpbin.org/get"),
		curlx.SetParamsMethod(curlx.MethodGet),
		curlx.SetParamsQueryAny(map[string]any{"page": 1}),
	)
	defer resp.Close()
	if resp.GetError() != nil {
		return
	}
	if !resp.IsSuccess() {
		fmt.Println("状态码:", resp.Status())
		return
	}
	var out struct {
		Args map[string]string `json:"args"`
	}
	_ = resp.JSON(&out)
}
```

## 常见用法

### 请求头、Cookie、认证

```go
client.Send(ctx,
	curlx.SetParamsUrl(url),
	curlx.SetParamsMethod(curlx.MethodPost),
	curlx.SetBearerToken("token"),          // Authorization: Bearer token
	curlx.SetBasicAuth("user", "pass"),
	curlx.SetUserAgent(curlx.UserAgentChrome),
	curlx.SetParamsHeaders(map[string]string{"X-Trace-Id": "abc"}),
	curlx.SetCookie("sid", "123"),
	curlx.SetParamsJson(map[string]any{"k": "v"}), // 自动设置 Content-Type: application/json
)
```

### 表单与文件上传

```go
// 内存中的小文件：Content-Length 已知，可跟随重定向
client.Send(ctx,
	curlx.SetParamsUrl(url),
	curlx.SetParamsMethod(curlx.MethodPost),
	curlx.SetParamsContentType(curlx.ContentTypeForm),
	curlx.SetParamsFormText("name", "yun"),
	curlx.SetParamsFormFile("file", "a.png", fileBytes),
)

// 大文件：以 chunked 方式边读边发，不会把整个文件读进内存
file, _ := os.Open("big.iso")
defer file.Close()
client.Send(ctx,
	curlx.SetParamsUrl(url),
	curlx.SetParamsMethod(curlx.MethodPost),
	curlx.SetParamsContentType(curlx.ContentTypeForm),
	curlx.SetParamsFormFileReader("file", "big.iso", file),
)

// urlencoded
client.Send(ctx,
	curlx.SetParamsUrl(url),
	curlx.SetParamsMethod(curlx.MethodPost),
	curlx.SetParamsFormValues(url.Values{"a": {"1"}}),
)
```

### 流式读取（SSE / 大响应）

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

ch, err := client.SendStream(ctx, curlx.SetParamsUrl(sseURL), curlx.SetParamsMethod(curlx.MethodGet))
if err != nil { // 建连失败、状态码非 2xx 会同步返回
	return
}
for res := range ch {
	if res.Err != nil {
		break
	}
	fmt.Println(res.Line)
}
```

放弃读取时必须 `cancel()`，否则读取 goroutine 会等到超时才退出。

### 代理与指定地址

```go
client.WithProxyHttp("http://127.0.0.1:8080")
client.WithProxySocks5("socks5://127.0.0.1:1080") // 也接受 "127.0.0.1:1080"

// 强制连接到指定 IP（URL 中的主机名仍用于 Host 头与 TLS SNI）
client.WithAddress("10.0.0.1")
```

这两个方法可以在运行期调用：内部会复制一份 transport，不会影响正在进行的请求。

### 证书

```go
// 跳过校验（仅调试/自签名环境）
curlx.NewCurlx(curlx.WithTLSInsecureSkipVerify())

// 证书固定：SHA-256 十六进制（可带冒号）或 "sha256/<base64>"（SPKI）
curlx.NewCurlx(curlx.WithTLSPin("a1b2..."))
```

指纹非法时会"失败关闭"：所有 HTTPS 请求都会返回错误，而不会退化成不校验。

## 错误处理

```go
body, err := client.Send(ctx, curlx.SetParamsUrl(url), curlx.SetParamsMethod(curlx.MethodGet))

// 状态码不是 2xx
if errors.Is(err, curlx.ErrStatusNotOK) {
	var se *curlx.StatusError
	if errors.As(err, &se) {
		log.Println(se.StatusCode, string(se.Body))
	}
}

// 超时
if resp.IsTimeout() { ... }

// 响应体超过 WithMaxResponseBytes 限制
if errors.Is(err, curlx.ErrResponseTooLarge) { ... }
```

## 配置项

| Option | 说明 |
| --- | --- |
| `WithTimeout` | 单次请求总超时（默认 120s），也可用 `SetParamsTimeout` 按请求覆盖 |
| `WithLogger` / `WithLoggerLength` | 日志实现与 body 截断长度（默认不输出日志） |
| `WithLogRequestBody` / `WithLogResponseBody` | 是否记录请求/响应体（默认只记响应体；Authorization/Cookie 等始终脱敏） |
| `WithTLSInsecureSkipVerify` / `WithTLSPin` | 证书校验策略 |
| `WithMaxIdleConns` / `WithMaxIdleConnsPerHost` / `WithMaxConnsPerHost` / `WithIdleConnTimeout` | 连接池 |
| `WithDialTimeout` / `WithKeepAlive` / `WithResponseHeaderTimeout` / `WithMaxResponseHeaderBytes` | 传输层 |
| `WithMaxResponseBytes` | 响应体上限，超过返回 `ErrResponseTooLarge`（0 表示不限制） |

## 生产环境与高并发

### 性能

`go test -run '^$' -bench . -benchmem`（本机 i7-14700K，本地 httptest 服务，每次请求读 40 字节 JSON）：

| 实现 | ns/op | allocs/op | B/op |
| --- | --- | --- | --- |
| `net/http`（无超时） | ~90µs | 68 | 6433 |
| `net/http`（超时 120s） | ~89µs | 78 | 7473 |
| `net/http` + curlx 的 transport + 超时 | ~52µs | 79 | 7474 |
| **curlx `Get`** | **~98µs** | **83** | **8436** |

结论：
- 相比"带超时的标准库等价实现"，**封装层本身只多 4 次分配、约 1KB/请求**；构建请求的路径
  （`BenchmarkBuildRequest`）与标准库完全一致（3 allocs，零额外分配）。
- 多出来的 10 allocs 来自 `http.Client.Timeout` —— 这是每请求的超时安全网，不是浪费。
- 200 并发 × 100 请求的压测中，curlx 吞吐为标准库的 **0.87 ~ 0.93 倍**，
  p99 与错误率与之持平；这是本地零耗时接口下的最坏情况，真实网络请求中差异被网络耗时淹没。
- 响应体大小会直接影响内存：`Response.GetBody` 会把整个 body 读进内存，
  高并发下建议用 `WithMaxResponseBytes` 设上限。

### 高并发调优建议

```go
client := curlx.NewCurlx(
	curlx.WithTimeout(3*time.Second),          // 必须小于上游超时，避免请求堆积
	curlx.WithMaxIdleConns(500),               // 总空闲连接
	curlx.WithMaxIdleConnsPerHost(100),        // 建议与 MaxConnsPerHost 同量级，否则突发流量会反复重连
	curlx.WithMaxConnsPerHost(100),            // 对下游的连接数限流（超过的请求排队）
	curlx.WithIdleConnTimeout(90*time.Second),
	curlx.WithMaxResponseBytes(32<<20),        // 防止异常响应打爆内存
	curlx.WithResponseHeaderTimeout(2*time.Second),
)
```

- **客户端全局复用**：连接池在客户端内部，按下游分组创建少量客户端并长期持有。
- **`MaxIdleConnsPerHost` 不要远小于 `MaxConnsPerHost`**：默认是 10 / 50，
  持续高并发下多余连接会被回收、下一波再重连，形成拨号抖动。
- **给所有请求传 context**：上游断开时及时取消，避免占用连接。
- **必须消费或关闭响应体**：`Get`/`Send` 已自动读完；用 `SendWithResponse` 时务必
  `defer resp.Close()`，否则连接无法复用。
- **日志保持关闭**（默认即关闭）：开启日志会带来字符串拼接与 map 拷贝开销；
  热路径上 `logEnabled=false` 时不会构造任何日志参数。
- **运行期变更代理/地址会重建连接池**：`WithAddress`/`WithProxyHttp` 会复制 transport
  并关闭旧池的空闲连接，因此请在启动阶段配置，不要在请求循环里反复调用。

### 目前不提供的能力

这些需要调用方自行封装或在网关/队列层解决：

- **重试与退避**（含 `Retry-After`、幂等性判断）；
- **指标/追踪**：可用 `client.Transport()` 配合 `httptrace` 自行埋点；
- **Cookie Jar / 会话保持**：目前 Cookie 通过 `SetCookie` 按请求传递，不跨请求自动保存；
- **下载到文件/流式落盘**：`SendStream` 是按行切分的，二进制大文件建议直接用标准库或等后续版本；
- **HTTP/3**。

## 从旧版本升级

本次改动修复了若干 panic 与静默失效问题，包含少量破坏性变更：

1. `Send` 现在把 **2xx** 视为成功（原来只认 200）；非 2xx 返回 `*StatusError`，
   `errors.Is(err, ErrStatusNotOK)` 依然成立。
2. `SendWithResponse` 返回 `*Response`（原来是值类型）。
3. `SendStream` 返回 `(<-chan StreamResult, error)`，建连/状态码错误同步返回，读取错误通过
   `StreamResult.Err` 上报。
4. `WithAddress(addr string) error` 签名变更，并且真正生效（旧实现忽略参数）。
5. `WithProxySocks5` 会校验地址格式，非法地址立即返回错误。
6. **默认不再输出日志**，需要日志请显式 `WithLogger(curlx.NewStdLogger())`。
7. `SetParamsHeader` 由 `Add` 改为 `Set`，追加多值请用 `AddParamsHeader`。
8. `resopnse.go` 重命名为 `response.go`；`curlx.Post` 包级函数改为 `(*Curlx).Post`。
9. 移除了对 `code.yun.ink/pkg/convx` 的依赖。
