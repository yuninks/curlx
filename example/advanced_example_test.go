package example

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/yuninks/curlx"
)

// Example_errorHandling 演示如何区分与处理各类错误。
func Example_errorHandling() {
	srv := demoServer()
	defer srv.Close()

	client := curlx.NewCurlx()
	ctx := context.Background()

	// 1) 状态码不是 2xx：返回 *StatusError，携带状态码与响应体
	_, err := client.Get(ctx, srv.URL+"/api/error")
	fmt.Println("errors.Is(ErrStatusNotOK):", errors.Is(err, curlx.ErrStatusNotOK))

	var statusErr *curlx.StatusError
	if errors.As(err, &statusErr) {
		fmt.Println("StatusCode:", statusErr.StatusCode)
		fmt.Println("Body:", string(statusErr.Body))
	}

	// 2) 连接失败 / 超时：错误里带有请求方法与地址
	_, err = client.Get(ctx, "http://127.0.0.1:1/unreachable")
	fmt.Println("连接失败:", strings.Contains(err.Error(), "127.0.0.1:1"))

	// 3) 参数非法：在发起请求前就报错
	_, err = client.Send(ctx, curlx.SetParamsUrl("ftp://example.com"), curlx.SetParamsMethod(curlx.MethodGet))
	fmt.Println("协议不支持:", err != nil && strings.Contains(err.Error(), "不支持的URL协议"))

	_, err = client.Get(nil, srv.URL)
	fmt.Println("nil ctx:", errors.Is(err, curlx.ErrNilContext))

	// Output:
	// errors.Is(ErrStatusNotOK): true
	// StatusCode: 500
	// Body: {"error":"boom","code":50001}
	// 连接失败: true
	// 协议不支持: true
	// nil ctx: true
}

// Example_maxResponseBytes 演示限制响应体大小，避免异常服务端耗尽内存。
func Example_maxResponseBytes() {
	srv := demoServer()
	defer srv.Close()

	// 服务端返回 4KB，这里限制 1KB
	client := curlx.NewCurlx(curlx.WithMaxResponseBytes(1024))

	_, err := client.Get(context.Background(), srv.URL+"/api/large")
	fmt.Println("超过限制:", errors.Is(err, curlx.ErrResponseTooLarge))

	// 放宽限制即可正常读取
	client = curlx.NewCurlx(curlx.WithMaxResponseBytes(64 * 1024))
	body, err := client.Get(context.Background(), srv.URL+"/api/large")
	fmt.Println("放宽后:", err == nil, len(body))

	// Output:
	// 超过限制: true
	// 放宽后: true 4096
}

// Example_customLogger 演示自定义日志实现，以及敏感头部的自动脱敏。
//
// 默认不输出任何日志；这里把日志收集到内存里方便断言。
func Example_customLogger() {
	srv := demoServer()
	defer srv.Close()

	logger := &memoryLogger{}
	client := curlx.NewCurlx(
		curlx.WithLogger(logger),
		curlx.WithLoggerLength(20),
	)

	_, err := client.Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/echo"),
		curlx.SetParamsMethod(curlx.MethodGet),
		curlx.SetBearerToken("SUPER-SECRET-TOKEN"),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	all := logger.all()
	fmt.Println("记录了日志:", len(all) > 0)
	fmt.Println("Authorization 已脱敏:", !strings.Contains(all, "SUPER-SECRET-TOKEN"))
	fmt.Println("包含脱敏标记:", strings.Contains(all, "[REDACTED]"))

	// Output:
	// 记录了日志: true
	// Authorization 已脱敏: true
	// 包含脱敏标记: true
}

// Example_httpProxy 演示 HTTP 代理。
func Example_httpProxy() {
	srv := demoServer()
	defer srv.Close()
	proxy := demoProxy()
	defer proxy.Close()

	client := curlx.NewCurlx()
	// 支持 "http://host:port"、"https://host:port"、"socks5://host:port"，也可以省略 scheme
	if err := client.WithProxyHttp(proxy.URL); err != nil {
		fmt.Println("err:", err)
		return
	}

	resp := client.SendWithResponse(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/user/42"),
		curlx.SetParamsMethod(curlx.MethodGet),
	)
	defer resp.Close()

	fmt.Println("X-Via:", resp.GetHeaderLine("X-Via"))
	fmt.Println("body:", strings.TrimSpace(resp.String()))

	// 非法代理地址会立即报错，而不是等到第一次请求
	fmt.Println("非法协议:", client.WithProxyHttp("ftp://127.0.0.1:8080") != nil)
	fmt.Println("缺少端口:", client.WithProxyHttp("http://127.0.0.1") != nil)
	fmt.Println("socks5 缺端口:", client.WithProxySocks5("127.0.0.1") != nil)

	// Output:
	// X-Via: demo-proxy
	// body: {"id":42,"name":"yun","tags":["go","http"]}
	// 非法协议: true
	// 缺少端口: true
	// socks5 缺端口: true
}

// Example_dialAddress 演示绕过 DNS，把请求固定打到指定地址。
//
// URL 里的主机名仍然用于 Host 头与 TLS SNI，只是 TCP 连接被改写。
func Example_dialAddress() {
	srv := demoServer()
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	client := curlx.NewCurlx()
	if err := client.WithAddress("127.0.0.1"); err != nil { // 不写端口则沿用目标端口
		fmt.Println("err:", err)
		return
	}

	// 主机名无法解析，但因为指定了 IP，请求依然成功
	body, err := client.Get(context.Background(), "http://curlx-does-not-resolve.invalid:"+u.Port()+"/api/user/42")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("body:", strings.TrimSpace(string(body)))

	// 也可以写成 host:port 或 IPv6 字面量
	fmt.Println("带端口:", client.WithAddress("127.0.0.1:8080") == nil)
	fmt.Println("IPv6:", client.WithAddress("::1") == nil)
	fmt.Println("非法端口:", client.WithAddress("127.0.0.1:abc") != nil)

	// Output:
	// body: {"id":42,"name":"yun","tags":["go","http"]}
	// 带端口: true
	// IPv6: true
	// 非法端口: true
}

// Example_tlsPin 演示证书指纹固定（certificate pinning）。
//
// 自签名证书需要同时开启 WithTLSInsecureSkipVerify，此时校验完全由指纹承担；
// 指纹不匹配或格式非法都会让请求失败（失败关闭，而不是静默跳过校验）。
func Example_tlsPin() {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secure"))
	}))
	defer srv.Close()

	sum := sha256.Sum256(srv.Certificate().Raw)
	fingerprint := hex.EncodeToString(sum[:])

	ok := curlx.NewCurlx(curlx.WithTLSInsecureSkipVerify(), curlx.WithTLSPin(fingerprint))
	body, err := ok.Get(context.Background(), srv.URL)
	fmt.Println("指纹匹配:", err == nil, string(body))

	bad := curlx.NewCurlx(curlx.WithTLSInsecureSkipVerify(), curlx.WithTLSPin(strings.Repeat("ab", 32)))
	_, err = bad.Get(context.Background(), srv.URL)
	fmt.Println("指纹不匹配被拒绝:", err != nil && strings.Contains(err.Error(), "fingerprint mismatch"))

	invalid := curlx.NewCurlx(curlx.WithTLSInsecureSkipVerify(), curlx.WithTLSPin("不是指纹"))
	_, err = invalid.Get(context.Background(), srv.URL)
	fmt.Println("非法指纹失败关闭:", err != nil && strings.Contains(err.Error(), "invalid certificate fingerprint"))

	// Output:
	// 指纹匹配: true secure
	// 指纹不匹配被拒绝: true
	// 非法指纹失败关闭: true
}

// memoryLogger 是一个把日志收集到内存的 OptionLogger 实现。
type memoryLogger struct {
	lines []string
}

func (l *memoryLogger) Infof(ctx context.Context, format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *memoryLogger) Errorf(ctx context.Context, format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *memoryLogger) all() string { return strings.Join(l.lines, "\n") }
