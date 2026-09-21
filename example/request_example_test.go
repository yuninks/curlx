package example

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/yuninks/curlx"
)

// Example_requestParams 演示请求头、认证、Cookie、查询参数与请求方法的构造。
//
// 参数是"函数式"的：按顺序调用，后面的可以覆盖前面的。
func Example_requestParams() {
	srv := demoServer()
	defer srv.Close()

	client := curlx.NewCurlx()

	// 请求头 / 认证 / Cookie / 查询参数
	body, err := client.Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/echo"),
		curlx.SetParamsMethod(curlx.MethodGet),
		curlx.SetParamsQueryAny(map[string]any{"page": 2, "size": 20}),
		curlx.SetBearerToken("t0k3n"), // Authorization: Bearer t0k3n
		curlx.SetCookie("sid", "abc123"),
		curlx.SetParamsHeaders(map[string]string{"X-Trace-Id": "trace-001"}),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	var echo struct {
		Method        string `json:"method"`
		Query         string `json:"query"`
		Authorization string `json:"authorization"`
		Cookie        string `json:"cookie"`
		TraceID       string `json:"trace_id"`
	}
	if err := unmarshal(body, &echo); err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("query:", echo.Query)
	fmt.Println("auth:", echo.Authorization)
	fmt.Println("cookie:", echo.Cookie)
	fmt.Println("trace:", echo.TraceID)

	// Basic 认证
	body, err = client.Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/echo"),
		curlx.SetParamsMethod(curlx.MethodGet),
		curlx.SetBasicAuth("user", "pass"),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	_ = unmarshal(body, &echo)
	fmt.Println("basic:", echo.Authorization)

	// 表单参数（urlencoded）
	body, err = client.Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/form"),
		curlx.SetParamsMethod(curlx.MethodPost),
		curlx.SetParamsFormValues(url.Values{"a": {"1"}, "b": {"2"}}),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("form:", string(body))

	// Output:
	// query: page=2&size=20
	// auth: Bearer t0k3n
	// cookie: sid=abc123
	// trace: trace-001
	// basic: Basic dXNlcjpwYXNz
	// form: {"content_type":"application/x-www-form-urlencoded","body":"a=1&b=2"}
}

// Example_perRequestTimeout 演示按请求覆盖超时时间。
//
// 客户端超时 5s，本次请求只给 100ms，因此 /api/slow（300ms）会超时。
func Example_perRequestTimeout() {
	srv := demoServer()
	defer srv.Close()

	client := curlx.NewCurlx(curlx.WithTimeout(5 * time.Second))

	resp := client.SendWithResponse(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/slow"),
		curlx.SetParamsMethod(curlx.MethodGet),
		curlx.SetParamsTimeout(100*time.Millisecond),
	)
	defer resp.Close()

	fmt.Println("请求失败:", resp.GetError() != nil)
	fmt.Println("是超时:", resp.IsTimeout())

	// 给足时间就没问题
	body, err := client.Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/slow"),
		curlx.SetParamsMethod(curlx.MethodGet),
		curlx.SetParamsTimeout(2*time.Second),
	)
	fmt.Println("足够超时:", err == nil, string(body))

	// Output:
	// 请求失败: true
	// 是超时: true
	// 足够超时: true slow-done
}

// Example_customMethod 演示预置方法常量之外的请求方法（如 PROPFIND）。
func Example_customMethod() {
	srv := demoServer()
	defer srv.Close()

	body, err := curlx.NewCurlx().Do(context.Background(), curlx.Method("PROPFIND"), srv.URL+"/api/echo")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	var echo struct {
		Method string `json:"method"`
	}
	_ = unmarshal(body, &echo)
	fmt.Println("method:", echo.Method)

	// Output:
	// method: PROPFIND
}
