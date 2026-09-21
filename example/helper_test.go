package example

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"
)

// demoServer 启动一个本地 HTTP 服务，覆盖示例用到的各种接口。
//
// 所有示例都跑在本地，不依赖外网，因此可以直接 `go test ./example/ -v` 运行。
// 实际使用时把 srv.URL 换成你自己的接口地址即可。
func demoServer() *httptest.Server {
	mux := http.NewServeMux()

	// 返回固定 JSON，用于演示读取响应体 / 反序列化 / 响应头。
	mux.HandleFunc("/api/user/42", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-42")
		_, _ = io.WriteString(w, `{"id":42,"name":"yun","tags":["go","http"]}`)
	})

	// 回显请求信息，用于演示请求头 / Cookie / 查询参数 / 请求体的构造结果。
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(echoResult{
			Method:        r.Method,
			Query:         r.URL.RawQuery,
			ContentType:   r.Header.Get("Content-Type"),
			Body:          string(body),
			Authorization: r.Header.Get("Authorization"),
			Cookie:        r.Header.Get("Cookie"),
			TraceID:       r.Header.Get("X-Trace-Id"),
			UserAgent:     shortUA(r.Header.Get("User-Agent")),
		})
	})

	// multipart/form-data 上传。
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
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
		sum := sha256.Sum256(content)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(uploadResult{
			File:    header.Filename,
			Size:    len(content),
			Content: string(content),
			SHA256:  hex.EncodeToString(sum[:8]),
			Note:    r.FormValue("note"),
		})
	})

	// application/x-www-form-urlencoded，原样回显请求体。
	mux.HandleFunc("/api/form", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"content_type":%q,"body":%q}`+"\n",
			r.Header.Get("Content-Type"), string(body))
	})

	// 服务端推送（SSE 风格），用于演示 SendStream。
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 1; i <= 3; i++ {
			_, _ = fmt.Fprintf(w, "data: event-%d\n\n", i)
			if flusher != nil {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	})

	// 非 2xx 响应；带 ?big=1 时返回 200KB，用于演示错误响应同样不会破坏连接复用。
	mux.HandleFunc("/api/error", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if r.URL.Query().Get("big") != "" {
			for i := 0; i < 200*1024; i++ {
				_, _ = io.WriteString(w, "e")
			}
			return
		}
		_, _ = io.WriteString(w, `{"error":"boom","code":50001}`)
	})

	// 慢响应，用于演示超时。
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(300 * time.Millisecond):
		}
		_, _ = io.WriteString(w, "slow-done")
	})

	// 4KB 响应，用于演示 WithMaxResponseBytes。
	mux.HandleFunc("/api/large", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 0; i < 4096; i++ {
			_, _ = io.WriteString(w, "x")
		}
	})

	// 302 跳转，curlx 默认跟随（最多 10 次）。
	mux.HandleFunc("/api/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/user/42", http.StatusFound)
	})

	// gzip 压缩响应，curlx 会自动解压。
	mux.HandleFunc("/api/gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = io.WriteString(gz, `{"compressed":true}`)
		_ = gz.Close()
	})

	return httptest.NewServer(mux)
}

// demoProxy 启动一个最小的正向代理，用于演示 WithProxyHttp。
// 它会给响应加上 X-Via 头，示例据此确认请求确实走了代理。
func demoProxy() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 作为代理收到的是绝对地址（http://host/path），直接转发即可
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Header = r.Header.Clone()

		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("X-Via", "demo-proxy")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
}

// echoResult 是 /api/echo 的回显结构，字段顺序固定，保证示例输出稳定。
type echoResult struct {
	Method        string `json:"method"`
	Query         string `json:"query"`
	ContentType   string `json:"content_type"`
	Body          string `json:"body"`
	Authorization string `json:"authorization"`
	Cookie        string `json:"cookie"`
	TraceID       string `json:"trace_id"`
	UserAgent     string `json:"user_agent"`
}

type uploadResult struct {
	File    string `json:"file"`
	Size    int    `json:"size"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
	Note    string `json:"note"`
}

// shortUA 截断 User-Agent，避免示例输出过长。
func shortUA(ua string) string {
	const max = 20
	if len(ua) > max {
		return ua[:max] + "..."
	}
	return ua
}

// unmarshal 只是 json.Unmarshal 的简写，让示例聚焦在 curlx 的用法上。
func unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
