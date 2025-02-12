package curlx

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
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
	opts      clientOptions
	transport *http.Transport
}

func NewCurlx(opts ...Option) *Curlx {
	defaultOpts := defaultOptions()
	for _, apply := range opts {
		apply(&defaultOpts)
	}

	transport := &http.Transport{
		// Dial: func(netw, addr string) (net.Conn, error) {
		// 	// 这里指定域名访问的IP
		// 	// if addr == "api.hk.blueoceanpay.com:443" {
		// 	// 	addr = "47.56.200.21:443"
		// 	// }
		// 	conn, err := net.DialTimeout(netw, addr, time.Second*time.Duration(timeOut)) // 设置建立连接超时
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	// conn.RemoteAddr().String()
		// 	conn.SetDeadline(time.Now().Add(time.Second * time.Duration(timeOut))) // 设置发送接收数据超时
		// 	return conn, nil
		// },
		// DialContext: (&net.Dialer{
		// 	Timeout:   3 * time.Second, // 建立TCP链接的超时时间
		// 	KeepAlive: 30 * time.Second, // TCP keepalive超时时间
		// }).DialContext,
		// TLSHandshakeTimeout: time.Second * 10, // TLS握手超时
		// ResponseHeaderTimeout: time.Second * 10, // 接收响应头的超时时间
		// ExpectContinueTimeout: time.Second * 10, // 发送请求头超时时间 100-continue状态码超时时间
		DisableKeepAlives:   false,           // 短连接（默认是使用长连接，连接过多时会造成服务器拒绝服务问题）
		MaxIdleConns:        0,               // 所有host的连接池最大连接数量，默认无穷大
		MaxIdleConnsPerHost: 5,               // 每个host的连接池最大空闲连接收，默认2
		MaxConnsPerHost:     0,               // 每个host的最大连接数量
		IdleConnTimeout:     time.Second * 2, // 空闲连接超时关闭的时间
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
		fmt.Println("proxy.SOCKS5 err", err)
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
func (c *Curlx) Send(ctx context.Context, p ...Param) (res []byte, httpcode int, err error) {
	_, response, err := c.SendExec(ctx, p...)
	if err != nil {
		return nil, -1, err
	}

	defer response.Body.Close() // 处理完关闭

	// stdout := os.Stdout                     // 将结果定位到标准输出，也可以直接打印出来，或定位到其他地方进行相应处理
	// _, err = io.Copy(stdout, response.Body) // 将第二个参数拷贝到第一个参数，直到第二参数到达EOF或发生错误，返回拷贝的值
	status := response.StatusCode // 获取状态码，正常是200

	var body []byte
	// switch response.Header.Get("Content-Encoding") {
	// case "gzip":
	// 	reader, err := gzip.NewReader(response.Body)
	// 	if err != nil {
	// 		return nil, status, err
	// 	}
	// 	for {
	// 		buf := make([]byte, 1024)
	// 		n, err := reader.Read(buf)
	// 		if err != nil && err != io.EOF {
	// 			panic(err)
	// 		}
	// 		if n == 0 {
	// 			break
	// 		}
	// 		body = append(body, buf...)
	// 	}
	// default:
	// 	body, _ = io.ReadAll(response.Body)
	// }

	if response.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(response.Body)
		if err != nil {
			return nil, response.StatusCode, err
		}
		body, err = io.ReadAll(reader)
		if err != nil {
			return nil, response.StatusCode, err
		}
		defer reader.Close()
	} else {
		body, err = io.ReadAll(response.Body)
		if err != nil {
			return nil, response.StatusCode, err
		}
	}

	c.opts.Logger.Infof(ctx, "curlx.Send body:%s", string(body))
	return body, status, nil
}

func (c *Curlx) SendWithResponee(ctx context.Context, ps ...Param) Response {
	r := Response{}
	req, resp, err := c.SendExec(ctx, ps...)
	r.req = req
	r.resp = resp

	if err != nil {
		r.err = err
		return r
	}
	var body []byte
	// switch resp.Header.Get("Content-Encoding") {
	// case "gzip":
	// 	reader, err := gzip.NewReader(resp.Body)
	// 	if err != nil {
	// 		r.err = err
	// 		return r
	// 	}
	// 	for {
	// 		buf := make([]byte, 1024)
	// 		n, err := reader.Read(buf)
	// 		if err != nil && err != io.EOF {
	// 			panic(err)
	// 		}
	// 		if n == 0 {
	// 			break
	// 		}
	// 		// 读取n个字节
	// 		body = append(body, buf[:n]...)
	// 		// body = append(body, buf...)
	// 	}
	// default:
	// body, _ = io.ReadAll(resp.Body)
	// }

	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			r.err = err
			return r
		}
		body, err = io.ReadAll(reader)
		if err != nil {
			r.err = err
			return r
		}
		defer reader.Close()
	} else {
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			r.err = err
			return r
		}
	}

	r.body = body
	c.opts.Logger.Infof(ctx, "curlx.Send body:%s", string(body))
	return r
}

/**
 * 执行发送
 * 注意：外部使用需要加这一句 defer response.Body.Close()
 */
func (c *Curlx) SendExec(ctx context.Context, ps ...Param) (req *http.Request, resp *http.Response, err error) {
	client := &http.Client{
		Timeout:   c.opts.TimeOut, // 整个请求的超时时间 设置该条连接的超时
		Transport: c.transport,    //
	}

	p := defaultParams()
	for _, param := range ps {
		param(&p)
	}
	c.opts.Logger.Infof(ctx, "curlx.sendExec params:%+v", p)

	err = p.parseMethod()
	if err != nil {
		return nil, nil, err
	}

	// 判断和处理url
	err = p.parseUrl()
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec parseUrl err:%v", err)
		return nil, nil, err
	}

	// 处理参数
	reqParams, err := p.parseParams()
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec parseParams err:%v", err)
		return nil, nil, err
	}

	// 初始化句柄
	request, err := http.NewRequest( // 提交请求 用指定的方法
		string(p.Method),
		p.Url,
		reqParams,
	)
	if err != nil {
		c.opts.Logger.Errorf(ctx, "curlx.sendExec NewRequest err:%v", err)
		return nil, nil, err
	}

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
		return nil, nil, err
	}
	// response.StatusCode
	return request, response, nil
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

		_, response, err := c.SendExec(ctx, ps...)
		if err != nil {
			return
		}
		defer response.Body.Close() // 处理完关闭

		scanner := bufio.NewScanner(response.Body)
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
