package curlx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 这些基准测试用于观察 curlx 相对标准库的额外开销，以及并发下的表现。
//
//	go test -bench . -benchmem -run '^$'
//	go test -bench BenchmarkGetParallel -benchmem -cpu 1,2,4,8 -run '^$'
func benchServer(b *testing.B, payload string) *httptest.Server {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))
	b.Cleanup(srv.Close)
	return srv
}

const benchPayload = `{"id":42,"name":"yun","tags":["go","http"]}`

// BenchmarkBuildRequest 只测量 curlx 自身的参数处理开销（不含网络），
// 与 BenchmarkBuildRequestNetHTTP 对比可以看出封装层的额外分配。
func BenchmarkBuildRequest(b *testing.B) {
	ctx := context.Background()
	p := defaultParams()
	p.Url = "http://127.0.0.1:8080/api/user/42"
	p.Method = MethodGet

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pp := p
		if err := pp.parseUrl(); err != nil {
			b.Fatal(err)
		}
		body, err := pp.parseParams()
		if err != nil {
			b.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, string(pp.Method), pp.Url, body)
		if err != nil {
			b.Fatal(err)
		}
		_ = req
	}
}

// BenchmarkBuildRequestNetHTTP 是对照组：标准库构造一次请求。
func BenchmarkBuildRequestNetHTTP(b *testing.B) {
	ctx := context.Background()
	const url = "http://127.0.0.1:8080/api/user/42"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = req
	}
}

// BenchmarkRoundTripOnly 只走"建连+发请求+取响应头"，不读响应体，
// 用来把 curlx 的封装开销从"读 body"的开销里分离出来。
func BenchmarkRoundTripOnly(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := NewCurlx()
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := client.SendWithResponse(ctx, SetParamsUrl(url), SetParamsMethod(MethodGet))
		if resp.GetError() != nil {
			b.Fatal(resp.GetError())
		}
		_ = resp.Close()
	}
}

// BenchmarkRoundTripOnlyNetHTTP 是与上面对应的标准库写法。
func BenchmarkRoundTripOnlyNetHTTP(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 10}}
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}

// BenchmarkGetNetHTTPCurlxTransport 用标准库的请求方式 + curlx 的 transport，
// 用来区分"transport 配置的开销"和"封装代码的开销"。
func BenchmarkGetNetHTTPCurlxTransport(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := &http.Client{Timeout: 120 * time.Second, Transport: NewCurlx().Transport()}
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadAll(resp.Body); err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}

// BenchmarkGet 是最常见的场景：GET 一个小 JSON。
func BenchmarkGet(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := NewCurlx()
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.Get(ctx, url); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetNetHTTP 是对照组：同样复用客户端、同样把响应体读进内存、同样设置总超时，
// 使用标准库实现。与 BenchmarkGet 的差值即 curlx 封装层的净开销。
func BenchmarkGetNetHTTP(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := &http.Client{
		Timeout:   120 * time.Second, // 与 curlx 默认值一致，否则对比不公平
		Transport: &http.Transport{MaxIdleConnsPerHost: 10},
	}
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadAll(resp.Body); err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}

// BenchmarkGetNetHTTPNoTimeout 与 BenchmarkGetNetHTTP 的唯一区别是不设置客户端超时，
// 用来量化 http.Client.Timeout 在每个请求上的固定开销。
func BenchmarkGetNetHTTPNoTimeout(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 10}}
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadAll(resp.Body); err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}

// BenchmarkGetParallel 观察高并发下的吞吐与分配。
func BenchmarkGetParallel(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := NewCurlx()
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := client.Get(ctx, url); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostJSON 覆盖 JSON POST 路径。
func BenchmarkPostJSON(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := NewCurlx()
	ctx := context.Background()
	url := srv.URL + "/json"
	body := map[string]any{"name": "yun", "age": 30, "tags": []string{"go", "http"}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.PostJsonAny(ctx, url, body); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSendWithResponse 覆盖需要状态码/响应头的路径（含 Response 分配与互斥锁）。
func BenchmarkSendWithResponse(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := NewCurlx()
	ctx := context.Background()
	url := srv.URL + "/json"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := client.SendWithResponse(ctx,
			SetParamsUrl(url),
			SetParamsMethod(MethodGet),
		)
		if _, err := resp.GetBody(); err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

// BenchmarkFormUpload 覆盖 multipart 构建路径。
func BenchmarkFormUpload(b *testing.B) {
	srv := benchServer(b, benchPayload)
	client := NewCurlx()
	ctx := context.Background()
	url := srv.URL + "/upload"
	file := make([]byte, 64*1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.Send(ctx,
			SetParamsUrl(url),
			SetParamsMethod(MethodPost),
			SetParamsContentType(ContentTypeForm),
			SetParamsFormText("note", "bench"),
			SetParamsFormFile("file", "a.bin", file),
		); err != nil {
			b.Fatal(err)
		}
	}
}
