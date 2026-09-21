package curlx

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

/**
 * Author: Yun
 * Date: 2023年7月12日11:35:01
 */

// maxRedirects 是允许的最大重定向次数（与标准库默认值一致）。
const maxRedirects = 10

// defaultMaxStreamLineBytes 是流式读取时单行允许的最大长度（bufio.Scanner 默认仅 64KB）。
const defaultMaxStreamLineBytes = 4 << 20

// Curlx 是一个可并发使用的 HTTP 客户端。
//
// 所有配置项在 NewCurlx 时确定；代理/指定地址等运行期变更通过复制 transport 完成，
// 不会修改正在被其它 goroutine 使用的 transport。
type Curlx struct {
	// opts 在构造完成后不再修改，因此读取无需加锁。
	opts ClientOptions

	// baseDialContext 是最原始的拨号器，供 WithAddress 复用。
	baseDialContext func(ctx context.Context, network, address string) (net.Conn, error)

	mu        sync.RWMutex
	client    *http.Client
	transport *http.Transport
}

// NewCurlx 创建一个客户端。
func NewCurlx(opts ...Option) *Curlx {
	o := defaultOptions()
	for _, apply := range opts {
		if apply == nil {
			continue
		}
		apply(&o)
	}
	if o.Logger == nil {
		o.Logger = NoopLogger{}
	}
	// 记录日志是否真正开启，热路径据此跳过日志参数的构造
	switch o.Logger.(type) {
	case NoopLogger, *NoopLogger:
		o.logEnabled = false
	default:
		o.logEnabled = true
	}
	if o.LoggerLength < 0 {
		o.LoggerLength = 0
	}
	if o.DialTimeout < 0 {
		o.DialTimeout = 0
	}

	dialer := &net.Dialer{Timeout: o.DialTimeout, KeepAlive: o.KeepAlive}
	transport := &http.Transport{
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           o.MaxIdleConns,
		MaxIdleConnsPerHost:    o.MaxIdleConnsPerHost,
		MaxConnsPerHost:        o.MaxConnsPerHost,
		IdleConnTimeout:        o.IdleConnTimeout,
		TLSHandshakeTimeout:    10 * time.Second,
		ExpectContinueTimeout:  time.Second,
		ResponseHeaderTimeout:  o.ResponseHeaderTimeout,
		MaxResponseHeaderBytes: o.MaxResponseHeaderBytes,
		TLSClientConfig:        o.buildTLSConfig(),
	}
	client := &http.Client{
		Timeout:       o.TimeOut,
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}

	return &Curlx{
		opts:            o,
		baseDialContext: dialer.DialContext,
		client:          client,
		transport:       transport,
	}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("curlx: stopped after %d redirects", maxRedirects)
	}
	return nil
}

// updateTransport 复制一份 transport 并应用修改，避免在请求进行中改动共享字段。
//
// 注意：Transport.Clone 只复制导出字段，连接池是新的空池。因此必须关闭旧 transport
// 的空闲连接，否则每次变更配置都会留下一批最长存活 IdleConnTimeout 的连接（连接泄漏）。
func (c *Curlx) updateTransport(apply func(t *http.Transport)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	transport := c.transport.Clone()
	apply(transport)

	client := *c.client
	client.Transport = transport

	old := c.transport
	c.transport = transport
	c.client = &client

	old.CloseIdleConnections()
}

// CloseIdleConnections 关闭连接池中的空闲连接。
func (c *Curlx) CloseIdleConnections() {
	c.mu.RLock()
	transport := c.transport
	c.mu.RUnlock()
	transport.CloseIdleConnections()
}

// Transport 返回底层 http.RoundTripper，可用于观测连接状态。
func (c *Curlx) Transport() http.RoundTripper {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.transport
}

// TLSClientConfig 返回当前使用的 TLS 配置副本，便于检查证书校验策略是否生效。
func (c *Curlx) TLSClientConfig() *tls.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cfg := c.transport.TLSClientConfig
	if cfg == nil {
		return nil
	}
	return cfg.Clone()
}

/**
 * 使用Socks5代理
 * @param address "127.0.0.1:1080"，也兼容 "socks5://127.0.0.1:1080"
 *
 * 目标主机名会交给代理做远程解析；重复调用时最后一次生效。
 */
func (c *Curlx) WithProxySocks5(address string) error {
	addr, err := parseProxyAddr(address, "socks5", "socks5h")
	if err != nil {
		c.opts.Logger.Errorf(context.Background(), "curlx: SOCKS5代理配置失败: %v", err)
		return err
	}

	dialer := &net.Dialer{Timeout: c.opts.DialTimeout, KeepAlive: c.opts.KeepAlive}
	dial, err := proxy.SOCKS5("tcp", addr, nil, dialer)
	if err != nil {
		c.opts.Logger.Errorf(context.Background(), "curlx: 创建SOCKS5代理失败: %v", err)
		return fmt.Errorf("curlx: 创建SOCKS5代理失败: %w", err)
	}
	contextDialer, ok := dial.(proxy.ContextDialer)
	if !ok {
		return errors.New("curlx: SOCKS5 dialer 不支持 context")
	}

	c.updateTransport(func(t *http.Transport) {
		t.DialContext = contextDialer.DialContext
	})
	return nil
}

/**
 * 使用HTTP/HTTPS代理
 * @param proxyAddr "http://proxyserver:port"，也兼容 "proxyserver:port"
 */
func (c *Curlx) WithProxyHttp(proxyAddr string) error {
	proxyAddr = strings.TrimSpace(proxyAddr)
	if proxyAddr == "" {
		return errors.New("curlx: 代理地址不能为空")
	}
	if !strings.Contains(proxyAddr, "://") {
		proxyAddr = "http://" + proxyAddr
	}
	u, err := url.Parse(proxyAddr)
	if err != nil {
		c.opts.Logger.Errorf(context.Background(), "curlx: 解析代理地址失败: %v", err)
		return fmt.Errorf("curlx: 解析代理地址失败: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5":
	default:
		return fmt.Errorf("curlx: 不支持的代理协议 %q", u.Scheme)
	}
	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		return fmt.Errorf("curlx: 代理地址必须包含端口: %s", proxyAddr)
	}

	c.updateTransport(func(t *http.Transport) {
		t.Proxy = http.ProxyURL(u)
	})
	return nil
}

// WithAddress 强制连接到指定地址，URL 中的主机名仍用于 Host 头与 TLS SNI。
//
// 典型用途：指定 IP 回源、绕过本地 DNS、内网直连。address 可以是 "10.0.0.1"、
// "10.0.0.1:8443" 或 IPv6 字面量；不指定端口时沿用目标端口。
// 与代理设置互斥，后调用者生效。
func (c *Curlx) WithAddress(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("curlx: 地址不能为空")
	}
	if strings.Contains(address, "://") {
		return fmt.Errorf("curlx: 地址不能包含协议前缀: %s", address)
	}

	host, port := address, ""
	if h, p, err := net.SplitHostPort(address); err == nil {
		host, port = h, p
	} else if net.ParseIP(address) != nil || !strings.Contains(address, ":") {
		host, port = address, ""
	} else {
		return fmt.Errorf("curlx: 非法的地址 %q: %w", address, err)
	}
	if host == "" {
		return errors.New("curlx: 地址缺少主机名")
	}
	if port != "" {
		if _, err := strconv.Atoi(port); err != nil {
			return fmt.Errorf("curlx: 非法的端口 %q", port)
		}
	}

	base := c.baseDialContext
	c.updateTransport(func(t *http.Transport) {
		t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			targetPort := port
			if targetPort == "" {
				if _, p, err := net.SplitHostPort(addr); err == nil {
					targetPort = p
				}
			}
			return base(ctx, network, net.JoinHostPort(host, targetPort))
		}
	})
	return nil
}

// parseProxyAddr 解析并校验代理地址，返回 host:port。
func parseProxyAddr(address string, schemes ...string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", errors.New("curlx: 代理地址不能为空")
	}

	if strings.Contains(address, "://") {
		u, err := url.Parse(address)
		if err != nil {
			return "", fmt.Errorf("curlx: 解析代理地址失败: %w", err)
		}
		supported := false
		for _, s := range schemes {
			if strings.EqualFold(u.Scheme, s) {
				supported = true
				break
			}
		}
		if !supported {
			return "", fmt.Errorf("curlx: 不支持的代理协议 %q", u.Scheme)
		}
		if _, _, err := net.SplitHostPort(u.Host); err != nil {
			return "", fmt.Errorf("curlx: 代理地址必须包含端口: %s", address)
		}
		return u.Host, nil
	}

	if _, _, err := net.SplitHostPort(address); err != nil {
		return "", fmt.Errorf("curlx: 代理地址必须形如 host:port: %s", address)
	}
	return address, nil
}

/**
 * 简单请求
 * 2xx 返回响应体；非 2xx 返回 *StatusError（errors.Is(err, ErrStatusNotOK) 为 true）。
 */
func (c *Curlx) Send(ctx context.Context, p ...Param) ([]byte, error) {
	params := defaultParams()
	applyParams(&params, p)
	return c.sendParams(ctx, params)
}

// sendParams 是 Send 的内部实现：直接使用已构造好的参数，避免闭包与切片的额外分配。
func (c *Curlx) sendParams(ctx context.Context, params ClientParams) ([]byte, error) {
	resp := c.execParams(ctx, params)
	defer resp.Close()

	if err := resp.GetError(); err != nil {
		return nil, err
	}

	body, err := resp.GetBody()
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx: 读取响应体失败: %v", err)
		return nil, err
	}

	if !resp.IsSuccess() {
		statusErr := newStatusError(resp, body)
		c.opts.Logger.Errorf(ctx, "%v", statusErr)
		return nil, statusErr
	}

	if c.opts.logEnabled && c.opts.LogResponseBody {
		c.opts.Logger.Infof(ctx, "curlx: response status=%d body:%s",
			resp.GetStatusCode(), truncateForLog(body, c.opts.LoggerLength))
	}
	return body, nil
}

// applyParams 依次应用参数函数。
func applyParams(p *ClientParams, ps []Param) {
	for _, param := range ps {
		if param == nil {
			continue
		}
		param(p)
	}
}

// Do 以指定方法发送请求。
func (c *Curlx) Do(ctx context.Context, method Method, url string, ps ...Param) ([]byte, error) {
	params := defaultParams()
	params.Url = url
	params.Method = method
	applyParams(&params, ps)
	return c.sendParams(ctx, params)
}

// PostJson 发送JSON数据
func (c *Curlx) PostJson(ctx context.Context, url string, jsonStr string) ([]byte, error) {
	return c.sendParams(ctx, ClientParams{
		Url:         url,
		Method:      MethodPost,
		Body:        []byte(jsonStr),
		ContentType: ContentTypeJson,
	})
}

// PostJsonAny 把任意值序列化为 JSON 后以 POST 发送。
func (c *Curlx) PostJsonAny(ctx context.Context, url string, v interface{}) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("curlx: 序列化请求体失败: %w", err)
	}
	return c.sendParams(ctx, ClientParams{
		Url:         url,
		Method:      MethodPost,
		Body:        body,
		ContentType: ContentTypeJson,
	})
}

// Get 简单GET请求
func (c *Curlx) Get(ctx context.Context, url string) ([]byte, error) {
	return c.sendParams(ctx, ClientParams{Url: url, Method: MethodGet})
}

// SendWithResponse 返回完整的 Response，使用方需要在使用完毕后调用 Close。
// 返回值不会为 nil；请求是否成功请检查 GetError / IsSuccess。
func (c *Curlx) SendWithResponse(ctx context.Context, ps ...Param) *Response {
	params := defaultParams()
	applyParams(&params, ps)
	return c.execParams(ctx, params)
}

/**
 * 执行发送
 *
 * 注意：外部使用需要加 defer response.Close()
 */
func (c *Curlx) exec(ctx context.Context, ps ...Param) *Response {
	params := defaultParams()
	applyParams(&params, ps)
	return c.execParams(ctx, params)
}

// execParams 完成参数解析与请求发送。
func (c *Curlx) execParams(ctx context.Context, p ClientParams) *Response {
	resp := &Response{maxBytes: c.opts.MaxResponseBytes}

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if ctx == nil {
		return resp.fail(c.opts.Logger, context.Background(), ErrNilContext)
	}

	if err := p.parseMethod(); err != nil {
		return resp.fail(c.opts.Logger, ctx, err)
	}
	if err := p.parseUrl(); err != nil {
		return resp.fail(c.opts.Logger, ctx, err)
	}
	body, err := p.parseParams()
	if err != nil {
		return resp.fail(c.opts.Logger, ctx, err)
	}

	// 请求级超时；取消函数交由 Response.Close 调用。
	// 不能在 exec 返回时取消，否则调用方还没读完响应体就会被中断。
	requestCtx := ctx
	cancel := context.CancelFunc(nil)
	if p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
	}
	resp.cancel = cancel

	request, err := http.NewRequestWithContext(requestCtx, string(p.Method), p.Url, body)
	if err != nil {
		return resp.fail(c.opts.Logger, ctx, err)
	}
	p.parseHeaders(request)
	p.parseCookies(request)
	resp.request = request

	// 日志参数的构造（尤其是 redactHeaders 的 map 拷贝）不应出现在热路径上
	if c.opts.logEnabled {
		c.opts.Logger.Infof(ctx, "curlx: request method=%s url=%s contentType=%s headers=%v cookies=%d",
			request.Method, request.URL.String(), p.ContentType, redactHeaders(request.Header), len(p.Cookies))
		if c.opts.LogRequestBody && len(p.Body) > 0 {
			c.opts.Logger.Infof(ctx, "curlx: request body:%s", truncateForLog(p.Body, c.opts.LoggerLength))
		}
	}

	response, err := client.Do(request)
	if err != nil {
		return resp.fail(c.opts.Logger, ctx, fmt.Errorf("curlx: %s %s 请求失败: %w", request.Method, request.URL.Redacted(), err))
	}
	resp.response = response

	return resp
}

// StreamResult 是流式请求的一条结果：要么是一行数据，要么是一个错误。
type StreamResult struct {
	Line string
	Err  error
}

/**
 * 流式请求
 *
 * 同步完成连接与状态码校验，因此建立连接失败、状态码非 2xx 会直接返回 error；
 * 之后的读取错误通过 StreamResult.Err 传递。
 *
 * 使用方必须持续读取返回的 channel，或者在放弃读取时取消 ctx，
 * 否则读取 goroutine 会一直阻塞到 ctx 结束（默认受客户端 TimeOut 限制）。
 *
 * 注意：http.Client.Timeout 覆盖整个请求（含读取响应体的过程），
 * 因此长连接（如 SSE）需要把 WithTimeout 设为 0，或保证 TimeOut 足够大。
 */
func (c *Curlx) SendStream(ctx context.Context, ps ...Param) (<-chan StreamResult, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	streamCtx := ctx
	cancel := context.CancelFunc(nil)
	if c.opts.TimeOut > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, c.opts.TimeOut)
	}

	resp := c.exec(streamCtx, ps...)
	if err := resp.GetError(); err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	if !resp.IsSuccess() {
		body, _ := resp.GetBody()
		statusErr := newStatusError(resp, body)
		_ = resp.Close()
		if cancel != nil {
			cancel()
		}
		return nil, statusErr
	}

	raw := resp.GetResponse()
	if raw == nil || raw.Body == nil {
		_ = resp.Close()
		if cancel != nil {
			cancel()
		}
		return nil, ErrNilResponse
	}

	out := make(chan StreamResult, 64)
	go func() {
		defer close(out)
		defer resp.Close()
		if cancel != nil {
			defer cancel()
		}

		scanner := bufio.NewScanner(raw.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), defaultMaxStreamLineBytes)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			select {
			case <-streamCtx.Done():
				return
			case out <- StreamResult{Line: line}:
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case <-streamCtx.Done():
			case out <- StreamResult{Err: fmt.Errorf("curlx: 读取流式响应失败: %w", err)}:
			}
		}
	}()

	return out, nil
}
