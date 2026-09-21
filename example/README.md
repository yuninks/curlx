# curlx 使用示例

这里的所有示例都是**可直接运行、可复制**的，并且不依赖外网：
每个示例内部用 `httptest` 起一个本地服务（见 [`helper_test.go`](helper_test.go) 里的 `demoServer`），
实际使用时把 `srv.URL` 换成你自己的接口地址即可。

## 运行

```bash
# 跑一遍全部示例（带 Output 的示例会被 go test 校验输出）
go test ./example/ -v

# 只跑某一个
go test ./example/ -run Example_multipartUpload -v

# 在浏览器里看带文档的示例
go doc -all github.com/yuninks/curlx/example
```

## 示例索引

> 想深入了解连接复用（配置、验证方式、常见破坏复用的写法），见
> [CONNECTION_REUSE.md](CONNECTION_REUSE.md)。

| 示例 | 内容 |
| --- | --- |
| [`Example_quickstart`](quickstart_example_test.go) | 创建客户端、GET、POST JSON、`SendWithResponse` + `JSON()` |
| [`Example_responseHelpers`](quickstart_example_test.go) | `IsSuccess` / `Status` / 响应头 / `String()` / JSON 解析错误 |
| [`Example_gzipAndRedirect`](quickstart_example_test.go) | gzip 自动解压、302 自动跟随 |
| [`Example_requestParams`](request_example_test.go) | 请求头、Bearer/Basic 认证、Cookie、查询参数、urlencoded |
| [`Example_perRequestTimeout`](request_example_test.go) | 按请求覆盖超时（`SetParamsTimeout`）与 `IsTimeout()` |
| [`Example_customMethod`](request_example_test.go) | 预置常量之外的请求方法（如 `PROPFIND`） |
| [`Example_multipartUpload`](upload_example_test.go) | 小文件表单上传（内存，Content-Length 已知） |
| [`Example_streamUpload`](upload_example_test.go) | 大文件流式上传（chunked，不占内存） |
| [`Example_uploadFromDisk`](upload_example_test.go) | 从磁盘流式上传的生产写法 |
| [`Example_jsonBody`](upload_example_test.go) | 三种构造 JSON 请求体的方式 |
| [`Example_stream`](stream_example_test.go) | `SendStream` 读取 SSE；`TestStreamCancel` 演示取消 |
| [`Example_errorHandling`](advanced_example_test.go) | 状态码错误 / 连接失败 / 参数非法 / nil ctx |
| [`Example_maxResponseBytes`](advanced_example_test.go) | 限制响应体大小，防止内存被打爆 |
| [`Example_customLogger`](advanced_example_test.go) | 自定义 `OptionLogger`，以及 Authorization 自动脱敏 |
| [`Example_httpProxy`](advanced_example_test.go) | HTTP 代理（示例内含一个最小正向代理）与地址校验 |
| [`Example_dialAddress`](advanced_example_test.go) | `WithAddress` 绕过 DNS，把请求固定打到指定 IP |
| [`Example_tlsPin`](advanced_example_test.go) | 证书指纹固定，以及指纹非法时的"失败关闭" |
| [`Example_connectionReuse`](reuse_example_test.go) | 连接复用统计（`httptrace` 统计新建/复用次数） |
| [`Example_reuseAfterErrorStatus`](reuse_example_test.go) | 非 2xx 也不会破坏连接复用 |
| [`Example_separatePools`](reuse_example_test.go) | 不同客户端各自维护连接池 |
| [`TestConcurrentRequests`](reuse_example_test.go) | 并发请求与 `MaxConnsPerHost` 约束 |
| [`ConnectionPoolManager`](connection_pool_example.go) | 连接池管理器、自定义 `curlx.Option` 的写法 |

## 几个容易踩的点

1. **客户端要复用**：`curlx.NewCurlx()` 内部维护连接池，应当作为长生命周期对象持有，
   不要每次请求都创建。需要不同配置时创建多个客户端即可（它们的连接池相互独立）。
2. **一定要传 context**：`ctx` 是取消与超时的统一入口；`nil` 会返回 `ErrNilContext`。
   超时既可以用客户端级 `WithTimeout`，也可以用请求级 `SetParamsTimeout`。
3. **`SendWithResponse` 记得 `Close`**：即使请求失败也应当 `defer resp.Close()`（幂等且不会 panic）。
   如果既不读 body 也不 `Close`，连接不会被复用。
4. **`Send` 只把 2xx 当成功**：非 2xx 返回 `*StatusError`，
   用 `errors.As` 取状态码、`errors.Is(err, curlx.ErrStatusNotOK)` 判断。
5. **大文件用 `SetParamsFormFileReader`**：`SetParamsFormFile` 会把整个文件读进内存；
   流式方式以 chunked 发送，代价是无法跟随需要重发请求体的重定向。
6. **`SendStream` 要消费完 channel 或 cancel ctx**：否则读取 goroutine 会等到超时才退出；
   长连接（如 SSE）记得把 `WithTimeout` 设为 0 或足够大。
7. **默认不输出日志**：需要时 `curlx.WithLogger(curlx.NewStdLogger())`；
   日志里的 `Authorization`/`Cookie` 会被自动替换为 `[REDACTED]`。
8. **代理与 `WithAddress` 互斥**：两者都改写拨号方式，后调用者生效；
   它们会复制一份 transport，因此可以在运行期安全调用。
