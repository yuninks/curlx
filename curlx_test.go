package curlx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// echoHandler 把请求的关键信息回显成 JSON，便于断言。
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"method":      r.Method,
			"url":         r.URL.String(),
			"query":       r.URL.RawQuery,
			"body":        string(body),
			"contentType": r.Header.Get("Content-Type"),
			"headers":     r.Header,
			"ua":          r.Header.Get("User-Agent"),
			"auth":        r.Header.Get("Authorization"),
			"cookie":      r.Header.Get("Cookie"),
		})
	})
}

func decodeEcho(t *testing.T, body []byte) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("响应不是合法 JSON: %v, body=%s", err, body)
	}
	return out
}

func TestSendSuccess(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()

	body, err := NewCurlx().Get(context.Background(), srv.URL+"/hello")
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	got := decodeEcho(t, body)
	if got["method"] != "GET" {
		t.Fatalf("method = %v", got["method"])
	}
	if !strings.HasPrefix(got["ua"].(string), "Mozilla/5.0") {
		t.Fatalf("默认 User-Agent 未生效: %v", got["ua"])
	}
}

func TestSendAcceptsAll2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/created":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case "/nocontent":
			w.WriteHeader(http.StatusNoContent)
		case "/partial":
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("part"))
		}
	}))
	defer srv.Close()

	c := NewCurlx()
	ctx := context.Background()

	body, err := c.Get(ctx, srv.URL+"/created")
	if err != nil {
		t.Fatalf("201 应视为成功, err = %v", err)
	}
	if string(body) != `{"id":1}` {
		t.Fatalf("201 body = %q", body)
	}
	if _, err := c.Get(ctx, srv.URL+"/nocontent"); err != nil {
		t.Fatalf("204 应视为成功, err = %v", err)
	}
	if _, err := c.Get(ctx, srv.URL+"/partial"); err != nil {
		t.Fatalf("206 应视为成功, err = %v", err)
	}
}

func TestSendStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	_, err := NewCurlx().Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("500 应返回错误")
	}
	if !errors.Is(err, ErrStatusNotOK) {
		t.Fatalf("errors.Is(err, ErrStatusNotOK) = false, err = %v", err)
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("errors.As(*StatusError) 失败, err = %v", err)
	}
	if statusErr.StatusCode != 500 {
		t.Fatalf("StatusCode = %d", statusErr.StatusCode)
	}
	if !strings.Contains(string(statusErr.Body), "boom") {
		t.Fatalf("StatusError 应携带响应体, got %q", statusErr.Body)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("错误信息应包含状态码: %v", err)
	}
}

func TestTLSInsecureSkipVerifyDoesNotPanic(t *testing.T) {
	srv := httptest.NewTLSServer(echoHandler())
	defer srv.Close()

	// 不带该选项时自签名证书应当失败
	if _, err := NewCurlx().Get(context.Background(), srv.URL); err == nil {
		t.Fatal("自签名证书默认应当校验失败")
	}
	// 带该选项时不应 panic，且请求成功
	if _, err := NewCurlx(WithTLSInsecureSkipVerify()).Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("WithTLSInsecureSkipVerify 后请求失败: %v", err)
	}
}

func TestTLSPin(t *testing.T) {
	srv := httptest.NewTLSServer(echoHandler())
	defer srv.Close()

	sum := sha256.Sum256(srv.Certificate().Raw)
	fingerprint := hex.EncodeToString(sum[:])

	c := NewCurlx(WithTLSInsecureSkipVerify(), WithTLSPin(fingerprint))
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("指纹匹配时请求应当成功: %v", err)
	}

	wrong := strings.Repeat("ab", 32)
	c2 := NewCurlx(WithTLSInsecureSkipVerify(), WithTLSPin(wrong))
	_, err := c2.Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("指纹不匹配时应报错, err = %v", err)
	}

	// 非法指纹必须"失败关闭"，而不是静默忽略
	c3 := NewCurlx(WithTLSInsecureSkipVerify(), WithTLSPin("这不是指纹"))
	_, err = c3.Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "invalid certificate fingerprint") {
		t.Fatalf("非法指纹应失败关闭, err = %v", err)
	}
}

func TestCloseOnFailedResponseDoesNotPanic(t *testing.T) {
	c := NewCurlx()
	resp := c.SendWithResponse(context.Background(),
		SetParamsUrl("http://127.0.0.1:1/unreachable"),
		SetParamsMethod(MethodGet),
	)
	if resp.GetError() == nil {
		t.Fatal("应当返回连接错误")
	}
	if err := resp.Close(); err != nil {
		t.Fatalf("Close 不应报错: %v", err)
	}
	// 重复 Close 也必须安全
	if err := resp.Close(); err != nil {
		t.Fatalf("重复 Close 不应报错: %v", err)
	}
	if _, err := resp.GetBody(); err == nil {
		t.Fatal("失败的响应读取 body 应返回错误")
	}
}

func TestCloseIsIdempotentAndGetBodyAfterClose(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()

	resp := NewCurlx().SendWithResponse(context.Background(), SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	if err := resp.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if _, err := resp.GetBody(); !errors.Is(err, ErrBodyClosed) {
		t.Fatalf("Close 后读取 body 应返回 ErrBodyClosed, got %v", err)
	}
}

func TestNilLoggerAndNilContext(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()

	// nil logger 不应 panic
	c := NewCurlx(WithLogger(nil))
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("nil logger 下请求失败: %v", err)
	}

	// nil context 应返回错误而不是 panic
	if _, err := NewCurlx().Get(nil, srv.URL); !errors.Is(err, ErrNilContext) {
		t.Fatalf("nil ctx 应返回 ErrNilContext, got %v", err)
	}
	if _, err := NewCurlx().SendStream(nil, SetParamsUrl(srv.URL), SetParamsMethod(MethodGet)); !errors.Is(err, ErrNilContext) {
		t.Fatalf("nil ctx 应返回 ErrNilContext, got %v", err)
	}
}

func TestSendStreamIgnoresNilParam(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()
	ch, err := NewCurlx(WithLogger(nil)).SendStream(context.Background(), nil, SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	if err != nil {
		t.Fatalf("nil Param 应被忽略: %v", err)
	}
	for range ch {
	}
}

func TestSendStreamErrorIsVisible(t *testing.T) {
	// 连接失败必须能返回错误，而不是静默给出空 channel
	if _, err := NewCurlx().SendStream(context.Background(),
		SetParamsUrl("http://127.0.0.1:1/unreachable"),
		SetParamsMethod(MethodGet),
	); err == nil {
		t.Fatal("建立连接失败时应返回错误")
	}

	// 非 2xx 同样应返回错误
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	_, err := NewCurlx().SendStream(context.Background(), SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("非 2xx 应返回 *StatusError, got %v", err)
	}
}

func TestSendStreamReportsReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 声明 100 字节但只写 6 字节：客户端会在读取时得到 unexpected EOF
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("hello\n"))
	}))
	defer srv.Close()

	ch, err := NewCurlx().SendStream(context.Background(), SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	if err != nil {
		t.Fatalf("SendStream err = %v", err)
	}

	var lines []string
	var gotErr error
	for res := range ch {
		if res.Err != nil {
			gotErr = res.Err
			continue
		}
		lines = append(lines, res.Line)
	}
	if len(lines) != 1 || lines[0] != "hello" {
		t.Fatalf("lines = %v", lines)
	}
	if gotErr == nil {
		t.Fatal("读取中断应通过 StreamResult.Err 上报")
	}
}

func TestSendStreamLongLine(t *testing.T) {
	long := strings.Repeat("a", 100_000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, long)
	}))
	defer srv.Close()

	ch, err := NewCurlx().SendStream(context.Background(), SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	if err != nil {
		t.Fatalf("SendStream err = %v", err)
	}
	var got []string
	for res := range ch {
		if res.Err != nil {
			t.Fatalf("不应有错误: %v", res.Err)
		}
		got = append(got, res.Line)
	}
	if len(got) != 1 || len(got[0]) != len(long) {
		t.Fatalf("100KB 长行被丢弃: 收到 %d 条, 首条长度 %d", len(got), len(got[0]))
	}
}

func TestSendStreamHonoursCallerContext(t *testing.T) {
	const total = 30
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		for i := 0; i < total; i++ {
			_, _ = fmt.Fprintf(w, "line-%d\n", i)
			if flusher != nil {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch, err := NewCurlx().SendStream(ctx, SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	if err != nil {
		t.Fatalf("SendStream err = %v", err)
	}

	done := make(chan int, 1)
	go func() {
		n := 0
		for range ch {
			n++
		}
		done <- n
	}()

	select {
	case n := <-done:
		if n >= total {
			t.Fatalf("调用方 context 未生效，收到全部 %d 条", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("调用方 context 取消后 channel 未关闭（goroutine 泄漏）")
	}
}

func TestFormUpload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		_, _ = fmt.Fprintf(w, "name=%s;size=%d;content=%s;field=%s;partType=%s",
			header.Filename, header.Size, content, r.FormValue("k"), header.Header.Get("Content-Type"))
	}))
	defer srv.Close()

	body, err := NewCurlx().Send(context.Background(),
		SetParamsUrl(srv.URL),
		SetParamsMethod(MethodPost),
		SetParamsContentType(ContentTypeForm),
		SetParamsFormText("k", "v"),
		SetParamsFormFile("file", "hello.txt", []byte("file-content")),
	)
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	got := string(body)
	for _, want := range []string{"name=hello.txt", "size=12", "content=file-content", "field=v", "partType=text/plain"} {
		if !strings.Contains(got, want) {
			t.Fatalf("表单结果缺少 %q: %s", want, got)
		}
	}
}

func TestFormUploadStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		_, _ = fmt.Fprintf(w, "name=%s;content=%s;mime=%s", header.Filename, content, r.Header.Get("Content-Type"))
	}))
	defer srv.Close()

	body, err := NewCurlx().Send(context.Background(),
		SetParamsUrl(srv.URL),
		SetParamsMethod(MethodPost),
		SetParamsContentType(ContentTypeForm),
		SetParamsFormFileReader("file", "stream.bin", strings.NewReader("streamed-bytes")),
	)
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	if got := string(body); !strings.Contains(got, "content=streamed-bytes") || !strings.Contains(got, "multipart/form-data") {
		t.Fatalf("流式上传结果异常: %s", got)
	}
}

func TestLegacyFormBodyStillWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	params := []FormParam{{FieldName: "file", FileName: "legacy.txt", FieldType: FieldTypeFile, FileBytes: []byte("legacy")}}
	raw, _ := json.Marshal(params)

	body, err := NewCurlx().Send(context.Background(), SetParamsAll(ClientParams{
		Url:         srv.URL,
		Method:      MethodPost,
		Body:        raw,
		ContentType: ContentTypeForm,
	}))
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	if string(body) != "legacy" {
		t.Fatalf("body = %q", body)
	}
}

func TestUrlEncoded(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()
	c := NewCurlx()
	ctx := context.Background()

	// 原生 "a=1&b=2" 形式应当被接受
	body, err := c.Send(ctx,
		SetParamsUrl(srv.URL),
		SetParamsMethod(MethodPost),
		SetParamsContentType(ContentTypeUrlEncoded),
		SetParamsBodyString("a=1&b=2"),
	)
	if err != nil {
		t.Fatalf("原生 urlencoded body 失败: %v", err)
	}
	got := decodeEcho(t, body)
	if got["body"] != "a=1&b=2" {
		t.Fatalf("body = %v", got["body"])
	}

	// url.Values 形式
	body, err = c.Send(ctx,
		SetParamsUrl(srv.URL),
		SetParamsMethod(MethodPost),
		SetParamsFormValues(url.Values{"x": {"1"}, "y": {"2"}}),
	)
	if err != nil {
		t.Fatalf("FormValues 失败: %v", err)
	}
	got = decodeEcho(t, body)
	if got["contentType"] != string(ContentTypeUrlEncoded) {
		t.Fatalf("Content-Type = %v", got["contentType"])
	}
	if got["body"] != "x=1&y=2" {
		t.Fatalf("body = %v", got["body"])
	}
}

func TestPostBodyWithoutContentType(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()

	body, err := NewCurlx().Send(context.Background(),
		SetParamsUrl(srv.URL),
		SetParamsMethod(MethodPost),
		SetParamsBodyString("raw-payload"),
	)
	if err != nil {
		t.Fatalf("未设置 Content-Type 不应报错: %v", err)
	}
	if got := decodeEcho(t, body)["body"]; got != "raw-payload" {
		t.Fatalf("body = %v", got)
	}
}

func TestQueryParams(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()
	c := NewCurlx()
	ctx := context.Background()

	// 显式 query
	body, err := c.Send(ctx,
		SetParamsUrl(srv.URL+"/a?keep=1"),
		SetParamsMethod(MethodGet),
		SetParamsQuery(url.Values{"x": {"1"}, "y": {"2"}}),
	)
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	got := decodeEcho(t, body)
	if got["query"] != "keep=1&x=1&y=2" {
		t.Fatalf("query = %v", got["query"])
	}

	// 兼容历史行为：GET + JSON body 且无 Content-Type 时作为 query
	body, err = c.Send(ctx,
		SetParamsUrl(srv.URL),
		SetParamsMethod(MethodGet),
		SetParamsBodyAny(map[string]any{"a": 1, "b": "x"}),
	)
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	got = decodeEcho(t, body)
	if got["query"] != "a=1&b=x" || got["body"] != "" {
		t.Fatalf("query = %v, body = %v", got["query"], got["body"])
	}
}

func TestHeadersAreCanonicalizedAndCallerDataIsNotMutated(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()

	cp := ClientParams{
		Url:         srv.URL,
		Method:      MethodGet,
		Headers:     http.Header{"x-custom": {"v"}, "content-type": {"application/x-custom"}},
		ContentType: ContentTypeJson,
	}
	body, err := NewCurlx().Send(context.Background(), SetParamsAll(cp))
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	got := decodeEcho(t, body)
	if got["contentType"] != "application/x-custom" {
		t.Fatalf("非规范大小写的 Content-Type 未被识别: %v", got["contentType"])
	}
	if got["headers"].(map[string]any)["X-Custom"] == nil {
		t.Fatalf("自定义头丢失: %v", got["headers"])
	}

	// 调用方传入的结构体不应被 Send 修改
	if len(cp.Headers) != 2 {
		t.Fatalf("调用方 Headers 被修改: %v", cp.Headers)
	}
	if _, ok := cp.Headers["User-Agent"]; ok {
		t.Fatalf("调用方 Headers 被写入了 User-Agent: %v", cp.Headers)
	}
}

func TestSetBasicAuthAndBearerAndCookie(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()
	ctx := context.Background()
	c := NewCurlx()

	body, err := c.Send(ctx, SetParamsUrl(srv.URL), SetParamsMethod(MethodGet), SetBearerToken("t0k3n"), SetCookie("sid", "abc"))
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	got := decodeEcho(t, body)
	if got["auth"] != "Bearer t0k3n" {
		t.Fatalf("auth = %v", got["auth"])
	}
	if !strings.Contains(got["cookie"].(string), "sid=abc") {
		t.Fatalf("cookie = %v", got["cookie"])
	}

	body, err = c.Send(ctx, SetParamsUrl(srv.URL), SetParamsMethod(MethodGet), SetBasicAuth("u", "p"))
	if err != nil {
		t.Fatalf("Send err = %v", err)
	}
	if auth := decodeEcho(t, body)["auth"].(string); !strings.HasPrefix(auth, "Basic ") {
		t.Fatalf("auth = %v", auth)
	}
}

func TestWithAddressOverridesDialTarget(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()

	_, port, err := splitHostPort(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	// 目标主机名无法解析，但被 WithAddress 强制指向本地服务
	c := NewCurlx()
	if err := c.WithAddress("127.0.0.1"); err != nil {
		t.Fatalf("WithAddress err = %v", err)
	}
	body, err := c.Get(context.Background(), "http://curlx-does-not-resolve.invalid:"+port+"/x")
	if err != nil {
		t.Fatalf("WithAddress 未生效: %v", err)
	}
	if got := decodeEcho(t, body)["url"]; got != "/x" {
		t.Fatalf("url = %v", got)
	}

	// 非法地址
	if err := NewCurlx().WithAddress("http://127.0.0.1:8080"); err == nil {
		t.Fatal("带协议前缀的地址应报错")
	}
	if err := NewCurlx().WithAddress("127.0.0.1:notaport"); err == nil {
		t.Fatal("非法端口应报错")
	}
}

func TestProxyAddressValidation(t *testing.T) {
	// 带 scheme 的 socks5 地址必须被接受（历史文档就是这么写的）
	if err := NewCurlx().WithProxySocks5("socks5://127.0.0.1:1080"); err != nil {
		t.Fatalf("socks5:// 前缀应被接受: %v", err)
	}
	if err := NewCurlx().WithProxySocks5("127.0.0.1:1080"); err != nil {
		t.Fatalf("host:port 应被接受: %v", err)
	}
	if err := NewCurlx().WithProxySocks5("127.0.0.1"); err == nil {
		t.Fatal("缺少端口应报错")
	}
	if err := NewCurlx().WithProxySocks5(""); err == nil {
		t.Fatal("空地址应报错")
	}

	if err := NewCurlx().WithProxyHttp("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("http 代理应被接受: %v", err)
	}
	if err := NewCurlx().WithProxyHttp("127.0.0.1:8080"); err != nil {
		t.Fatalf("缺省 scheme 应被接受: %v", err)
	}
	if err := NewCurlx().WithProxyHttp("ftp://127.0.0.1:8080"); err == nil {
		t.Fatal("不支持的代理协议应报错")
	}
	if err := NewCurlx().WithProxyHttp("http://127.0.0.1"); err == nil {
		t.Fatal("缺少端口应报错")
	}
}

func TestConnectionReuseAfterErrorStatus(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	payload := strings.Repeat("x", 200*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.RemoteAddr]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	c := NewCurlx()
	for i := 0; i < 5; i++ {
		if _, err := c.Get(context.Background(), srv.URL); err == nil {
			t.Fatal("应当返回状态码错误")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("非 2xx 响应未复用连接：5 次请求用了 %d 个源端口", len(seen))
	}
}

func TestMaxResponseBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("y", 4096)))
	}))
	defer srv.Close()

	_, err := NewCurlx(WithMaxResponseBytes(100)).Get(context.Background(), srv.URL)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("应返回 ErrResponseTooLarge, got %v", err)
	}

	if _, err := NewCurlx(WithMaxResponseBytes(8192)).Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("未超限时不应报错: %v", err)
	}
}

func TestTimeoutAndIsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	resp := NewCurlx(WithTimeout(80*time.Millisecond)).SendWithResponse(context.Background(),
		SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	defer resp.Close()

	if resp.GetError() == nil {
		t.Fatal("应当超时")
	}
	if !resp.IsTimeout() {
		t.Fatalf("IsTimeout() = false, err = %v", resp.GetError())
	}
}

func TestPerRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	start := time.Now()
	resp := NewCurlx(WithTimeout(30*time.Second)).SendWithResponse(context.Background(),
		SetParamsUrl(srv.URL), SetParamsMethod(MethodGet), SetParamsTimeout(80*time.Millisecond))
	defer resp.Close()

	if resp.GetError() == nil {
		t.Fatal("请求级超时未生效")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("请求级超时未生效，耗时 %v", elapsed)
	}
}

func TestRedirectLimit(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewCurlx().Get(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("重定向次数超限应报错, got %v", err)
	}
}

func TestRedirectFollowed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/target", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("arrived"))
	})
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/target", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body, err := NewCurlx().Get(context.Background(), srv.URL+"/start")
	if err != nil {
		t.Fatalf("重定向跟随失败: %v", err)
	}
	if string(body) != "arrived" {
		t.Fatalf("body = %q", body)
	}
}

func TestParameterValidation(t *testing.T) {
	c := NewCurlx()
	ctx := context.Background()

	if _, err := c.Send(ctx, SetParamsUrl("http://127.0.0.1:1")); err == nil {
		t.Fatal("空 method 应报错")
	}
	if _, err := c.Send(ctx, SetParamsUrl(""), SetParamsMethod(MethodGet)); err == nil {
		t.Fatal("空 url 应报错")
	}
	if _, err := c.Send(ctx, SetParamsUrl("ftp://example.com"), SetParamsMethod(MethodGet)); err == nil {
		t.Fatal("不支持的协议应报错")
	}
	if _, err := c.Send(ctx, SetParamsUrl("http://"), SetParamsMethod(MethodGet)); err == nil {
		t.Fatal("缺少主机名应报错")
	}
	// 缺少协议时自动补全 http://
	if _, err := c.Send(ctx, SetParamsUrl("127.0.0.1:1"), SetParamsMethod(MethodGet)); err == nil ||
		!strings.Contains(err.Error(), "http://127.0.0.1:1") {
		t.Fatalf("应补全 scheme 后再发起请求, got %v", err)
	}
}

func TestBodyMarshalErrorIsReturned(t *testing.T) {
	_, err := NewCurlx().Send(context.Background(),
		SetParamsUrl("http://127.0.0.1:1"),
		SetParamsMethod(MethodPost),
		SetParamsBodyAny(make(chan int)),
	)
	if err == nil || !strings.Contains(err.Error(), "序列化请求体失败") {
		t.Fatalf("序列化失败应返回错误, got %v", err)
	}
}

func TestJSONResponseHelper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Trace", "abc")
		_, _ = w.Write([]byte(`{"id":7,"name":"yun"}`))
	}))
	defer srv.Close()

	resp := NewCurlx().SendWithResponse(context.Background(), SetParamsUrl(srv.URL), SetParamsMethod(MethodGet))
	defer resp.Close()

	if !resp.IsSuccess() || resp.Status() != 200 {
		t.Fatalf("status = %d", resp.Status())
	}
	var out struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := resp.JSON(&out); err != nil {
		t.Fatalf("JSON err = %v", err)
	}
	if out.ID != 7 || out.Name != "yun" {
		t.Fatalf("out = %+v", out)
	}
	if !resp.HasHeader("x-trace") || resp.GetHeaderLine("X-Trace") != "abc" {
		t.Fatalf("响应头读取异常: %v", resp.GetHeaders())
	}
	if got := resp.GetHeader("X-TRACE"); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("GetHeader = %v", got)
	}
}

func TestConcurrentUseIsSafe(t *testing.T) {
	srv := httptest.NewServer(echoHandler())
	defer srv.Close()

	c := NewCurlx()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := c.Get(context.Background(), srv.URL); err != nil {
					t.Errorf("并发请求失败: %v", err)
					return
				}
			}
		}()
	}
	// 同时反复重建拨号配置，验证不会与进行中的请求产生竞争
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			_ = c.WithAddress("127.0.0.1")
		}
	}()
	wg.Wait()
}

func TestStreamFormBodyDoesNotLeakGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	payload := strings.NewReader(strings.Repeat("z", 1<<20))
	for i := 0; i < 20; i++ {
		_, err := NewCurlx().Send(context.Background(),
			SetParamsUrl("http://127.0.0.1:1/unreachable"),
			SetParamsMethod(MethodPost),
			SetParamsContentType(ContentTypeForm),
			SetParamsFormFileReader("f", "a.bin", payload),
		)
		if err == nil {
			t.Fatal("应当返回连接错误")
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("流式表单疑似 goroutine 泄漏: before=%d after=%d", before, runtime.NumGoroutine())
}

// splitHostPort 从 httptest 的 URL 中取出主机与端口。
func splitHostPort(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	host, port, err := net.SplitHostPort(u.Host)
	return host, port, err
}
