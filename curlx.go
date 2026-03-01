package curlx

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

/**
 * Author: Yun
 * Date: 2023年7月12日11:35:01
 */

// type DialContext func(ctx context.Context, network, addr string) (net.Conn, error)

type Curlx struct {
	opts      ClientOptions
	transport *http.Transport
}

func NewCurlx(opts ...Option) *Curlx {
	defaultOpts := defaultOptions()
	for _, apply := range opts {
		apply(&defaultOpts)
	}

	transport := &http.Transport{
		DisableKeepAlives:     false, // 启用keep-alive连接复用
		MaxIdleConns:          defaultOpts.MaxIdleConns,
		MaxIdleConnsPerHost:   defaultOpts.MaxIdleConnsPerHost,
		MaxConnsPerHost:       defaultOpts.MaxConnsPerHost,
		IdleConnTimeout:       defaultOpts.IdleConnTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
	}

	if defaultOpts.InsecureSkipVerify {
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	return &Curlx{
		opts:      defaultOpts,
		transport: transport,
	}

}

/**
 * 使用Socks5代理
 * @param address "socks5://127.0.0.1:1080"
 */
func (c *Curlx) WithProxySocks5(address string) error {
	baseDialer := &net.Dialer{
		// Timeout:   180 * time.Second,
		// KeepAlive: 180 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial:     c.transport.DialContext,
		},
	}
	dialSocksProxy, err := proxy.SOCKS5("tcp", address, nil, baseDialer)
	if err != nil {
		c.opts.Logger.Errorf(context.Background(), "proxy.SOCKS5 err: %v", err)
		return err
	}
	dialContext := (baseDialer).DialContext
	if contextDialer, ok := dialSocksProxy.(proxy.ContextDialer); ok {
		dialContext = contextDialer.DialContext
	}
	c.transport.DialContext = dialContext
	return nil
}

/**
 * 使用HTTP/HTTPS代理
 * @param proxyAddr "https://proxyserver:port"
 */
func (c *Curlx) WithProxyHttp(proxyAddr string) error {
	proxy, err := url.Parse(proxyAddr)
	if err != nil {
		c.opts.Logger.Errorf(context.Background(), "proxy.HTTP/HTTPS err: %v", err)
		return err
	}
	c.transport.Proxy = http.ProxyURL(proxy)
	return nil
}

// 指定访问的IP
// 127.0.0.1:8080
func (c *Curlx) WithAddress(ctx context.Context, addr string) {
	// network tcp/udp
	c.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial(network, addr)
	}
}

/**
 * 简单请求
 */
func (c *Curlx) Send(ctx context.Context, p ...Param) (res []byte, err error) {
	resp := c.exec(ctx, p...)
	if resp.Err != nil {
		return nil, resp.Err
	}
	defer resp.Close() // 处理完关闭

	status := resp.GetStatusCode()
	if status != 200 {
		c.opts.Logger.Errorf(ctx, "curlx.Send status not OK: %d", status)
		return nil, ErrStatusNotOK
	}

	body, err := resp.GetBody()
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.Send getBody err:%v", err)
		return nil, err
	}

	// 打印日志时截取前指定长度，避免日志过大
	bodyLog := []rune(string(body))
	if len(bodyLog) > c.opts.LoggerLength {
		bodyLog = bodyLog[:c.opts.LoggerLength]
	}
	c.opts.Logger.Infof(ctx, "curlx.Send response body:%s", string(bodyLog))
	return body, nil
}

// PostJson 发送JSON数据
func (l *Curlx) PostJson(ctx context.Context, url string, jsonStr string) ([]byte, error) {
	return l.Send(ctx,
		SetParamsUrl(url),
		SetParamsBody([]byte(jsonStr)),
		SetParamsContentType(ContentTypeJson),
		SetParamsMethod(MethodPost),
	)
}

// Get 简单GET请求
func (l *Curlx) Get(ctx context.Context, url string) ([]byte, error) {
	return l.Send(ctx,
		SetParamsUrl(url),
		SetParamsMethod(MethodGet),
	)
}

func (c *Curlx) SendWithResponse(ctx context.Context, ps ...Param) Response {
	return c.exec(ctx, ps...)
}

/**
 * 执行发送
 * 注意：外部使用需要加这一句 defer response.Body.Close()
 */
func (c *Curlx) exec(ctx context.Context, ps ...Param) Response {
	resp := Response{}

	client := &http.Client{
		Timeout:   c.opts.TimeOut, // 整个请求的超时时间 设置该条连接的超时
		Transport: c.transport,    //
	}

	// 在http.Client中添加CheckRedirect函数 实现重定向控制
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 { // 限制重定向次数
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}

	p := defaultParams()
	for _, param := range ps {
		param(&p)
	}

	// 截取Body前指定长度输出，避免日志过大
	bodyLog := []rune(string(p.Body))
	if len(bodyLog) > c.opts.LoggerLength {
		bodyLog = bodyLog[:c.opts.LoggerLength]
	}

	c.opts.Logger.Infof(ctx, "curlx.sendExec params url:%s method:%s contentType:%s body:%s headers:%+v cookies:%+v", p.Url, p.Method, p.ContentType, string(bodyLog), p.Headers, p.Cookies)

	err := p.parseMethod()
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec parseMethod err:%v", err)
		resp.Err = err
		return resp
	}

	// 判断和处理url
	err = p.parseUrl()
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec parseUrl err:%v", err)
		resp.Err = err
		return resp
	}

	// 处理参数
	reqParams, err := p.parseParams()
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec parseParams err:%v", err)
		resp.Err = err
		return resp
	}

	// 初始化句柄
	request, err := http.NewRequest( // 提交请求 用指定的方法
		string(p.Method),
		p.Url,
		reqParams,
	)
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec NewRequest err:%v", err)
		resp.Err = err
		return resp
	}

	c.opts.Logger.Infof(ctx, "curlx.sendExec request:%+v", request)
	resp.Request = request

	// 这里指定要访问的HOST,到时候服务器获取主机是获取到这个
	// request.Host = "api.hk.blueoceantech.co"

	// 设置上下文控制
	request = request.WithContext(ctx)

	// 处理请求头
	p.parseHeaders(request)

	// 处理Cookies
	p.parseCookies(request)

	// 发起请求
	response, err := client.Do(request)
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec client.Do err:%v", err)
		resp.Err = err
		return resp
	}
	resp.Response = response

	return resp
}

/**
 * 流式请求
 */
func (c *Curlx) SendStream(ctx context.Context, ps ...Param) (<-chan string, error) {

	data := make(chan string, 1000)

	go func() {
		defer close(data)

		ctx, cancel := context.WithTimeout(context.Background(), c.opts.TimeOut)
		defer cancel()

		response := c.exec(ctx, ps...)
		if response.Err != nil {
			return
		}
		defer response.Close() // 处理完关闭
		scanner := bufio.NewScanner(response.Response.Body)
		for scanner.Scan() {
			text := scanner.Text()
			if text == "" {
				continue
			}
			// 30min超时
			select {
			case <-ctx.Done():
				return
			case data <- text:
			}
		}
	}()

	return data, nil
}
