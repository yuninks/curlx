package example

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yuninks/curlx"
)

// Example_quickstart 演示最常见的三种调用方式。
//
// 注意：客户端应当复用（内部维护连接池），不要每次请求都 NewCurlx。
func Example_quickstart() {
	srv := demoServer()
	defer srv.Close()

	client := curlx.NewCurlx(
		curlx.WithTimeout(5*time.Second),
		curlx.WithLogger(curlx.NewStdLogger()), // 默认不输出日志，需要时显式打开
	)
	ctx := context.Background()

	// 1) 最简 GET：直接拿到响应体
	body, err := client.Get(ctx, srv.URL+"/api/user/42")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("GET ->", strings.TrimSpace(string(body)))

	// 2) POST JSON：结构体会被自动序列化
	body, err = client.PostJsonAny(ctx, srv.URL+"/api/echo", map[string]any{"name": "yun", "age": 30})
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	var echo struct {
		Method      string `json:"method"`
		ContentType string `json:"content_type"`
		Body        string `json:"body"`
	}
	if err := json.Unmarshal(body, &echo); err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("POST ->", echo.Method, echo.ContentType, echo.Body)

	// 3) 需要状态码 / 响应头时用 SendWithResponse
	resp := client.SendWithResponse(ctx,
		curlx.SetParamsUrl(srv.URL+"/api/user/42"),
		curlx.SetParamsMethod(curlx.MethodGet),
	)
	defer resp.Close()

	fmt.Println("status ->", resp.Status(), "x-request-id ->", resp.GetHeaderLine("X-Request-Id"))

	// 也可以直接反序列化到结构体
	var user struct {
		ID   int      `json:"id"`
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := resp.JSON(&user); err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Printf("user -> id=%d name=%s tags=%v\n", user.ID, user.Name, user.Tags)

	// Output:
	// GET -> {"id":42,"name":"yun","tags":["go","http"]}
	// POST -> POST application/json {"age":30,"name":"yun"}
	// status -> 200 x-request-id -> req-42
	// user -> id=42 name=yun tags=[go http]
}

// Example_responseHelpers 演示 Response 上的便捷方法。
func Example_responseHelpers() {
	srv := demoServer()
	defer srv.Close()

	client := curlx.NewCurlx()
	resp := client.SendWithResponse(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/user/42"),
		curlx.SetParamsMethod(curlx.MethodGet),
	)
	defer resp.Close()

	fmt.Println("IsSuccess:", resp.IsSuccess())
	fmt.Println("HasHeader(x-request-id):", resp.HasHeader("X-Request-Id"))
	fmt.Println("Content-Type:", resp.GetHeaderLine("Content-Type"))

	// JSON 解析失败会返回错误，而不是静默得到零值
	var notAnArray []int
	if err := resp.JSON(&notAnArray); err == nil {
		fmt.Println("unexpected: 解析应当失败")
	} else {
		fmt.Println("JSON 解析错误已被返回:", strings.Contains(err.Error(), "解析响应JSON失败"))
	}

	fmt.Println("body:", strings.TrimSpace(resp.String()))

	// Output:
	// IsSuccess: true
	// HasHeader(x-request-id): true
	// Content-Type: application/json
	// JSON 解析错误已被返回: true
	// body: {"id":42,"name":"yun","tags":["go","http"]}
}

// Example_gzipAndRedirect 演示 gzip 自动解压与重定向跟随。
func Example_gzipAndRedirect() {
	srv := demoServer()
	defer srv.Close()

	client := curlx.NewCurlx()
	ctx := context.Background()

	body, err := client.Get(ctx, srv.URL+"/api/gzip")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("gzip ->", string(body))

	// /api/redirect 会 302 到 /api/user/42，curlx 默认跟随
	body, err = client.Get(ctx, srv.URL+"/api/redirect")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("redirect ->", string(body))

	// Output:
	// gzip -> {"compressed":true}
	// redirect -> {"id":42,"name":"yun","tags":["go","http"]}
}
