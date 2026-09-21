package curlx

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrStatusNotOK 表示服务端返回了非 2xx 的状态码。
//
// 判断方式：
//
//	if errors.Is(err, curlx.ErrStatusNotOK) { ... }        // 是否状态码错误
//	var se *curlx.StatusError
//	if errors.As(err, &se) { se.StatusCode; se.Body }      // 取出状态码与响应体
var ErrStatusNotOK = errors.New("curlx: unexpected http status")

var (
	// ErrNilResponse 表示 Response 为空指针（请求尚未执行或已释放）。
	ErrNilResponse = errors.New("curlx: response is nil")
	// ErrNilContext 表示传入了 nil 的 context.Context。
	ErrNilContext = errors.New("curlx: nil context")
	// ErrBodyClosed 表示响应体已被 Close，无法再读取。
	ErrBodyClosed = errors.New("curlx: response body already closed")
	// ErrResponseTooLarge 表示响应体超过了 MaxResponseBytes 限制。
	ErrResponseTooLarge = errors.New("curlx: response body exceeds the configured limit")
)

// StatusError 描述一次"HTTP 请求成功但状态码不是 2xx"的错误。
type StatusError struct {
	Method     string
	URL        string
	StatusCode int
	// Status 是服务端返回的原始状态行（如 "500 Internal Server Error"）。
	Status string
	// Body 是响应体，最多保留 errorBodyLimit 字节，避免错误信息过大。
	Body []byte
}

// errorBodyLimit 是 StatusError 中保留的响应体最大字节数。
const errorBodyLimit = 512

func (e *StatusError) Error() string {
	status := e.Status
	if status == "" {
		status = strconv.Itoa(e.StatusCode)
	}
	msg := fmt.Sprintf("curlx: %s %s: unexpected status %s", e.Method, e.URL, status)
	if len(e.Body) > 0 {
		msg += ": " + truncateForLog(e.Body, errorBodyLimit)
	}
	return msg
}

// Is 让 errors.Is(err, ErrStatusNotOK) 成立，兼容旧版本的判断方式。
func (e *StatusError) Is(target error) bool { return target == ErrStatusNotOK }

// newStatusError 根据响应构造状态码错误。
func newStatusError(resp *Response, body []byte) *StatusError {
	se := &StatusError{Body: truncateBytes(body, errorBodyLimit)}
	if resp == nil {
		return se
	}
	if req := resp.GetRequest(); req != nil {
		se.Method = req.Method
		if req.URL != nil {
			se.URL = req.URL.String()
		}
	}
	if raw := resp.GetResponse(); raw != nil {
		se.StatusCode = raw.StatusCode
		se.Status = raw.Status
	}
	return se
}

// truncateBytes 返回最多 n 字节的副本。
func truncateBytes(b []byte, n int) []byte {
	if n <= 0 || len(b) <= n {
		return b
	}
	out := make([]byte, n)
	copy(out, b)
	return out
}
