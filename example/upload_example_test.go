package example

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yuninks/curlx"
)

// Example_multipartUpload 演示小文件的表单上传。
//
// 文件内容保存在内存中，Content-Length 已知，可以跟随重定向；
// 文件较大时请改用 Example_streamUpload。
func Example_multipartUpload() {
	srv := demoServer()
	defer srv.Close()

	csv := []byte("name,age\nyun,30\n")

	body, err := curlx.NewCurlx().Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/upload"),
		curlx.SetParamsMethod(curlx.MethodPost),
		curlx.SetParamsContentType(curlx.ContentTypeForm),
		curlx.SetParamsFormText("note", "季度报表"),
		curlx.SetParamsFormFile("file", "report.csv", csv),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	var result uploadResult
	if err := unmarshal(body, &result); err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("file:", result.File)
	fmt.Println("size:", result.Size)
	fmt.Println("note:", result.Note)
	fmt.Println("sha256(前8字节):", result.SHA256)

	// Output:
	// file: report.csv
	// size: 16
	// note: 季度报表
	// sha256(前8字节): f880df97a4761aca
}

// Example_streamUpload 演示大文件的流式上传。
//
// SetParamsFormFileReader 会边读边发（chunked），不会把整个文件读进内存。
// 代价是无法预知 Content-Length，因此不能跟随需要重发请求体的重定向。
func Example_streamUpload() {
	srv := demoServer()
	defer srv.Close()

	// 这里用 bytes.Reader 模拟文件；真实场景传 *os.File 即可
	content := strings.Repeat("streaming-upload-", 8)
	reader := bytes.NewReader([]byte(content))

	body, err := curlx.NewCurlx().Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/upload"),
		curlx.SetParamsMethod(curlx.MethodPost),
		curlx.SetParamsContentType(curlx.ContentTypeForm),
		curlx.SetParamsFormFileReader("file", "big.bin", reader),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	var result uploadResult
	_ = unmarshal(body, &result)
	fmt.Println("file:", result.File)
	fmt.Println("size:", result.Size)

	// Output:
	// file: big.bin
	// size: 136
}

// Example_uploadFromDisk 演示从磁盘流式上传（推荐的生产写法）。
//
// 该示例不会真正上传，只是展示读者 r 的所有权仍归调用方：curlx 不会关闭它。
func Example_uploadFromDisk() {
	file, err := os.CreateTemp("", "curlx-example-*.txt")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	name := file.Name()
	defer os.Remove(name)

	_, _ = file.WriteString("hello curlx")
	if _, err := file.Seek(0, 0); err != nil { // 流式上传前记得回到起点
		fmt.Println("err:", err)
		return
	}
	defer file.Close()

	srv := demoServer()
	defer srv.Close()

	body, err := curlx.NewCurlx().Send(context.Background(),
		curlx.SetParamsUrl(srv.URL+"/api/upload"),
		curlx.SetParamsMethod(curlx.MethodPost),
		curlx.SetParamsContentType(curlx.ContentTypeForm),
		curlx.SetParamsFormFileReader("file", "from-disk.txt", file),
		curlx.SetParamsTimeout(5*time.Second),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}

	var result uploadResult
	_ = unmarshal(body, &result)
	fmt.Println("file:", result.File)
	fmt.Println("content:", result.Content)

	// Output:
	// file: from-disk.txt
	// content: hello curlx
}

// Example_jsonBody 演示几种构造 JSON 请求体的方式。
func Example_jsonBody() {
	srv := demoServer()
	defer srv.Close()

	client := curlx.NewCurlx()
	ctx := context.Background()

	// 1) 结构体 / map：自动序列化，并设置 Content-Type
	for _, p := range []curlx.Param{
		curlx.SetParamsJson(map[string]any{"k": "v"}),
		curlx.SetParamsBodyString(`{"k":"v"}`),
		curlx.SetParamsBody([]byte(`{"k":"v"}`)),
	} {
		body, err := client.Send(ctx,
			curlx.SetParamsUrl(srv.URL+"/api/echo"),
			curlx.SetParamsMethod(curlx.MethodPost),
			p,
		)
		if err != nil {
			fmt.Println("err:", err)
			return
		}
		var echo echoResult
		_ = unmarshal(body, &echo)
		fmt.Printf("content-type=%q body=%s\n", echo.ContentType, echo.Body)
	}

	// Output:
	// content-type="application/json" body={"k":"v"}
	// content-type="" body={"k":"v"}
	// content-type="" body={"k":"v"}
}
