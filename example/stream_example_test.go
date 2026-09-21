package example

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yuninks/curlx"
)

// Example_stream 演示流式读取服务端推送（SSE）。
//
// 建连失败、状态码非 2xx 会由 SendStream 同步返回；
// 后续读取过程中的错误通过 StreamResult.Err 上报。
func Example_stream() {
	srv := demoServer()
	defer srv.Close()

	// 放弃读取时必须 cancel，否则读取 goroutine 会一直阻塞到超时
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := curlx.NewCurlx().SendStream(ctx,
		curlx.SetParamsUrl(srv.URL+"/api/events"),
		curlx.SetParamsMethod(curlx.MethodGet),
		curlx.SetParamsHeader("Accept", "text/event-stream"),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	for res := range ch {
		if res.Err != nil {
			fmt.Println("stream err:", res.Err)
			break
		}
		fmt.Println(res.Line)
	}

	// Output:
	// data: event-1
	// data: event-2
	// data: event-3
}

// TestStreamCancel 演示取消 context 后流式读取会及时结束，不会泄漏 goroutine。
func TestStreamCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			_, _ = fmt.Fprintln(w, "tick")
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch, err := curlx.NewCurlx().
		SendStream(ctx, curlx.SetParamsUrl(srv.URL), curlx.SetParamsMethod(curlx.MethodGet))
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
		if n == 0 {
			t.Fatal("取消前应当已经收到若干条数据")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("context 取消后 channel 没有关闭，疑似 goroutine 泄漏")
	}
}
