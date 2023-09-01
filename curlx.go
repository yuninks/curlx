package curlx

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

/**
 * Author: Yun
 * Date: 2023年7月12日11:35:01
 */

type dataType string

const (
	DataTypeForm   dataType = "form"
	DataTypeJson   dataType = "json"
	DataTypeXml    dataType = "xml"
	DataTypeEncode dataType = "encode"
	DataTypeText   dataType = "text"
)

type method string

const (
	MethodGet  method = "GET"
	MethodPost method = "POST"
)

type CurlParams struct {
	Url      string
	Method   method // GET/POST
	Params   interface{}
	Headers  map[string]interface{}
	Cookies  interface{}
	DataType dataType // FORM,JSON,XML
	timeOutSecond int
}

var (
	// 默认的transport
	transport http.Transport = http.Transport{
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
		// ResponseHeaderTimeout: time.Second * time.Duration(defaultTimeOut), // 响应超时
		DisableKeepAlives:   false,           // 短连接（默认是使用长连接，连接过多时会造成服务器拒绝服务问题）
		MaxIdleConns:        0,               // 所有host的连接池最大连接数量，默认无穷大
		MaxIdleConnsPerHost: 5,               // 每个host的连接池最大空闲连接收，默认2
		MaxConnsPerHost:     0,               // 每个host的最大连接数量
		IdleConnTimeout:     time.Second * 2, // 空闲连接超时关闭的时间
	}
	// client  = &http.Client{}

)

type DialContext func(ctx context.Context, network, addr string) (net.Conn, error)

type curlx struct {
	transport *http.Transport
}

func NewCurlx() *curlx {
	return &curlx{
		transport: &transport,
		timeOutSecond: 60
	}
}

/**
 * 使用Socks5代理
 */
func (c *curlx) WithSocks5(address string) error {
	baseDialer := &net.Dialer{
		Timeout:   180 * time.Second,
		KeepAlive: 180 * time.Second,
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
 * 不校验HTTPS证书
 */
func (c *curlx) WithInsecureSkipVerify() {
	c.transport.TLSClientConfig.InsecureSkipVerify = true
}

/**
 * 设置超时时间,单位秒
 */
func (c *curlx) WithTimeout(timeout int) {
	c.timeOutSecond = timeout
}

/**
 * 简单请求
 */
func (c *curlx) Send(ctx context.Context, p *CurlParams) (res string, httpcode int, err error) {
	response, err := c.sendExec(ctx, p)
	if err != nil {
		return "", -1, err
	}

	defer response.Body.Close() // 处理完关闭

	// stdout := os.Stdout                     // 将结果定位到标准输出，也可以直接打印出来，或定位到其他地方进行相应处理
	// _, err = io.Copy(stdout, response.Body) // 将第二个参数拷贝到第一个参数，直到第二参数到达EOF或发生错误，返回拷贝的值
	status := response.StatusCode // 获取状态码，正常是200

	var body []byte
	switch response.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err := gzip.NewReader(response.Body)
		if err != nil {
			return "", status, err
		}
		for {
			buf := make([]byte, 1024)
			n, err := reader.Read(buf)
			if err != nil && err != io.EOF {
				panic(err)
			}
			if n == 0 {
				break
			}
			body = append(body, buf...)
		}
	default:
		body, _ = io.ReadAll(response.Body)
	}
	return string(body), status, nil
}

/**
 * 执行发送
 * 注意：外部使用需要加这一句 defer response.Body.Close()
 */
func (c *curlx) sendExec(ctx context.Context, p *CurlParams) (resp *http.Response, err error) {
	client := &http.Client{
		Timeout:   time.Second * time.Duration(p.timeOutSecond), // 设置该条连接的超时
		Transport: c.transport,                                 //
	}

	err = p.parseMethod()
	if err != nil {
		return nil, err
	}

	// 判断和处理url
	err = p.parseUrl()
	if err != nil {
		return nil, err
	}

	// 处理参数
	reqParams, err := p.parseParams()
	if err != nil {
		return nil, err
	}

	// 初始化句柄
	request, err := http.NewRequest( // 提交请求 用指定的方法
		string(p.Method),
		p.Url,
		reqParams,
	)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return response, nil
}

/**
 * 流式请求
 */
func (c *curlx) SendChan(ctx context.Context, p *CurlParams) (<-chan string, error) {

	data := make(chan string, 1000)

	go func() {
		defer close(data)

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*30)
		defer cancel()

		response, err := c.sendExec(ctx, p)
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
