package curlx

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
)

// maxDrainBytes 是 Close 时主动丢弃的最大字节数，用于让小响应体所在的连接回到连接池。
const maxDrainBytes = 32 << 10

// Response 是一次请求的结果。
// 它不是并发安全的：请由同一个 goroutine 读取，并在使用完毕后调用 Close。
type Response struct {
	mu sync.Mutex

	response *http.Response
	request  *http.Request

	body     []byte
	bodyRead bool
	bodyErr  error

	err      error
	closed   bool
	closeErr error

	// cancel 用于释放请求级超时的 context。
	cancel context.CancelFunc
	// maxBytes 是响应体大小上限，0 表示不限制。
	maxBytes int64
}

// fail 记录失败原因并输出日志。
func (r *Response) fail(logger OptionLogger, ctx context.Context, err error) *Response {
	r.err = err
	logger.Errorf(ctx, "%v", err)
	return r
}

// Close 关闭响应体。重复调用是安全的。
//
// 若响应体尚未被读取，会先尽力丢弃一段内容，以便底层连接能够复用。
func (r *Response) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeLocked()
}

func (r *Response) closeLocked() error {
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.response == nil || r.response.Body == nil {
		return nil
	}
	if !r.bodyRead {
		// 尽量读掉剩余内容；读不完也没关系，连接会被丢弃而不是复用
		_, _ = io.CopyN(io.Discard, r.response.Body, maxDrainBytes)
	}
	r.closeErr = r.response.Body.Close()
	return r.closeErr
}

// GetRequest get request object
func (r *Response) GetRequest() *http.Request {
	if r == nil {
		return nil
	}
	return r.request
}

// GetResponse 返回底层 *http.Response。
func (r *Response) GetResponse() *http.Response {
	if r == nil {
		return nil
	}
	return r.response
}

// GetError 返回请求本身（连接、超时、参数等）的错误。
// 服务端返回非 2xx 不会设置该错误，请用 IsSuccess / GetStatusCode 判断。
func (r *Response) GetError() error {
	if r == nil {
		return ErrNilResponse
	}
	return r.err
}

// GetBody 读取并缓存响应体（自动处理 gzip）。读取完成后会关闭 Body。
func (r *Response) GetBody() ([]byte, error) {
	if r == nil {
		return nil, ErrNilResponse
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bodyLocked()
}

func (r *Response) bodyLocked() ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.bodyRead {
		return r.body, r.bodyErr
	}
	if r.response == nil || r.response.Body == nil {
		return nil, nil
	}
	if r.closed {
		r.bodyRead = true
		r.bodyErr = ErrBodyClosed
		return nil, r.bodyErr
	}

	r.bodyRead = true
	body, err := readResponseBody(r.response, r.maxBytes)
	r.body, r.bodyErr = body, err

	// 读完（或读失败）后关闭连接体，让连接可复用
	if closeErr := r.closeLocked(); err == nil && closeErr != nil {
		r.bodyErr = closeErr
	}
	return r.body, r.bodyErr
}

// readResponseBody 读取响应体，必要时解压 gzip，并限制最大长度。
func readResponseBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	var reader io.Reader = resp.Body
	// net/http 自动解压时会移除 Content-Encoding；手动设置了 Accept-Encoding 时这里兜底
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("curlx: 解压gzip响应失败: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	if maxBytes <= 0 {
		return io.ReadAll(reader)
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%w (limit=%d)", ErrResponseTooLarge, maxBytes)
	}
	return body, nil
}

// String 返回响应体字符串（读取失败时返回空字符串）。
//
// 实现了 fmt.Stringer，因此 fmt.Println(resp) 打印的是响应体而不是结构体。
func (r *Response) String() string {
	body, _ := r.GetBody()
	return string(body)
}

// JSON 把响应体按 JSON 解析到 v。
func (r *Response) JSON(v any) error {
	body, err := r.GetBody()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("curlx: 解析响应JSON失败: %w", err)
	}
	return nil
}

// GetStatusCode 返回 HTTP 状态码，请求未执行时返回 0。
func (r *Response) GetStatusCode() int {
	if r == nil || r.response == nil {
		return 0
	}
	return r.response.StatusCode
}

// Status 是 GetStatusCode 的简写。
func (r *Response) Status() int { return r.GetStatusCode() }

// IsSuccess 判断状态码是否为 2xx。
func (r *Response) IsSuccess() bool {
	code := r.GetStatusCode()
	return code >= 200 && code < 300
}

// IsTimeout get if request is timeout
func (r *Response) IsTimeout() bool {
	if r == nil || r.err == nil {
		return false
	}
	if errors.Is(r.err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(r.err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// GetParsedBody parse response body with gjson
func (r *Response) GetParsedBody() (*gjson.Result, error) {
	body, err := r.GetBody()
	if err != nil {
		return nil, err
	}
	pb := gjson.ParseBytes(body)
	return &pb, nil
}

// GetHeaders 返回响应头的副本，修改它不会影响原响应。
func (r *Response) GetHeaders() map[string][]string {
	if r == nil || r.response == nil {
		return nil
	}
	return cloneHeader(r.response.Header)
}

// GetHeader 返回指定响应头的全部值（大小写不敏感）。
func (r *Response) GetHeader(name string) []string {
	if r == nil || r.response == nil {
		return nil
	}
	return r.response.Header.Values(name)
}

// GetHeaderLine 返回指定响应头的第一个值。
func (r *Response) GetHeaderLine(name string) string {
	if r == nil || r.response == nil {
		return ""
	}
	return r.response.Header.Get(name)
}

// HasHeader get if header exsits in response headers
func (r *Response) HasHeader(name string) bool {
	if r == nil || r.response == nil {
		return false
	}
	_, ok := r.response.Header[http.CanonicalHeaderKey(name)]
	return ok
}
