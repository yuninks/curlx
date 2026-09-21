package curlx

import (
	"context"
	"fmt"
	"io"
)

// Post 使用指定的 Content-Type 发送请求体，返回完整的 Response。
//
// 与 http.Post 不同，这里会走 curlx 的超时、连接池、代理与日志配置。
// 注意：请求体会被完整读入内存，大文件请使用 SetParamsFormFileReader 流式上传。
// 返回的 error 只表示请求未能发出；是否成功请检查 GetError / IsSuccess。
func (c *Curlx) Post(ctx context.Context, url, contentType string, body io.Reader) (*Response, error) {
	if body == nil {
		return nil, fmt.Errorf("curlx: 请求体不能为空")
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("curlx: 读取请求体失败: %w", err)
	}

	resp := c.SendWithResponse(ctx,
		SetParamsUrl(url),
		SetParamsMethod(MethodPost),
		SetParamsBody(data),
		SetParamsContentType(ContentType(contentType)),
	)
	if err := resp.GetError(); err != nil {
		return nil, err
	}
	return resp, nil
}
