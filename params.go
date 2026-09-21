package curlx

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ClientParams 是一次请求的全部参数，按顺序由 Param 函数修改。
type ClientParams struct {
	Url         string
	Method      Method // GET/POST/PUT/DELETE...
	Body        []byte
	Headers     http.Header
	Cookies     []http.Cookie
	ContentType ContentType // FORM,JSON,XML...

	// Query 是显式的 URL 查询参数（GET 等无 body 的场景推荐直接使用）。
	Query url.Values
	// Forms 是 multipart/form-data 的字段列表，由 SetParamsFormText /
	// SetParamsFormFile / SetParamsFormFileReader 填充。
	Forms []FormParam
	// FormValues 是 application/x-www-form-urlencoded 的字段列表。
	FormValues url.Values

	// bodyErr 记录参数构造阶段的错误（如 JSON 序列化失败），在发送时返回。
	bodyErr error
	// timeout 是本次请求的独立超时。
	timeout time.Duration
}

func defaultParams() ClientParams {
	// Headers 延迟分配：没有请求头时不产生任何 map 分配
	return ClientParams{}
}

// Param 是请求参数的设置函数。
type Param func(*ClientParams)

// SetParamsAll 用给定的 ClientParams 覆盖当前参数。
// Headers/Cookies/Query/Forms 会被深拷贝，之后的修改不会影响调用方传入的结构体。
func SetParamsAll(cp ClientParams) Param {
	return func(param *ClientParams) {
		param.Url = cp.Url
		param.Method = cp.Method
		param.Body = cp.Body
		param.Headers = cloneHeader(cp.Headers)
		param.Cookies = append([]http.Cookie(nil), cp.Cookies...)
		param.ContentType = cp.ContentType
		param.Query = cloneValues(cp.Query)
		param.Forms = append([]FormParam(nil), cp.Forms...)
		param.FormValues = cloneValues(cp.FormValues)
	}
}

// SetParamsUrl 设置请求 URL。缺少协议时会自动补全 "http://"。
func SetParamsUrl(rawUrl string) Param {
	return func(param *ClientParams) { param.Url = rawUrl }
}

// SetParamsMethod 设置请求方法。
func SetParamsMethod(m Method) Param {
	return func(param *ClientParams) { param.Method = m }
}

// SetParamsBody 设置原始请求体。
func SetParamsBody(by []byte) Param {
	return func(param *ClientParams) { param.Body = by }
}

// SetParamsBodyString 设置字符串请求体。
func SetParamsBodyString(body string) Param {
	return func(param *ClientParams) { param.Body = []byte(body) }
}

// SetParamsBodyAny 设置请求体：[]byte / string 原样使用，其它类型做 JSON 序列化。
// 序列化失败会返回错误（在发送时由 Send 返回），不会再静默发出空 body。
func SetParamsBodyAny(v interface{}) Param {
	return func(param *ClientParams) {
		switch value := v.(type) {
		case nil:
			param.Body = nil
		case []byte:
			param.Body = value
		case string:
			param.Body = []byte(value)
		default:
			b, err := json.Marshal(value)
			if err != nil {
				param.bodyErr = fmt.Errorf("curlx: 序列化请求体失败: %w", err)
				return
			}
			param.Body = b
		}
	}
}

// SetParamsJson 是 SetParamsBodyAny 的语义化封装，同时设置 Content-Type 为 JSON。
func SetParamsJson(v interface{}) Param {
	return func(param *ClientParams) {
		SetParamsContentType(ContentTypeJson)(param)
		SetParamsBodyAny(v)(param)
	}
}

// SetParamsFormText 追加一个 multipart 文本字段。
func SetParamsFormText(fieldName, fieldValue string) Param {
	return func(param *ClientParams) {
		param.Forms = append(param.Forms, FormParam{
			FieldName:  fieldName,
			FieldValue: fieldValue,
			FieldType:  FieldTypeText,
		})
	}
}

// SetParamsFormFile 追加一个内存中的 multipart 文件字段。
func SetParamsFormFile(fieldName, fileName string, fileBytes []byte) Param {
	return func(param *ClientParams) {
		param.Forms = append(param.Forms, FormParam{
			FieldName: fieldName,
			FieldType: FieldTypeFile,
			FileName:  fileName,
			FileBytes: fileBytes,
		})
	}
}

// SetParamsFormFileReader 追加一个流式的 multipart 文件字段。
//
// 使用该方式时请求体会以 chunked 方式发送（无法预知 Content-Length），
// 因此无法跟随需要重发 body 的重定向；reader 的所有权仍归调用方，curlx 不会关闭它。
func SetParamsFormFileReader(fieldName, fileName string, reader io.Reader) Param {
	return func(param *ClientParams) {
		param.Forms = append(param.Forms, FormParam{
			FieldName:  fieldName,
			FieldType:  FieldTypeFile,
			FileName:   fileName,
			FileReader: reader,
		})
	}
}

// SetParamsFormValues 设置 application/x-www-form-urlencoded 的字段（会自动设置 Content-Type）。
func SetParamsFormValues(values url.Values) Param {
	return func(param *ClientParams) {
		param.ContentType = ContentTypeUrlEncoded
		param.FormValues = values
	}
}

// SetParamsFormMap 设置 application/x-www-form-urlencoded 的字段。
// 会自动设置 Content-Type。
func SetParamsFormMap(values map[string]string) Param {
	return func(param *ClientParams) {
		param.ContentType = ContentTypeUrlEncoded
		v := make(url.Values, len(values))
		for k, val := range values {
			v.Set(k, val)
		}
		param.FormValues = v
	}
}

// SetParamsQuery 设置 URL 查询参数（与 URL 中已有的参数合并）。
func SetParamsQuery(values url.Values) Param {
	return func(param *ClientParams) { param.Query = values }
}

// SetParamsQueryAny 设置 URL 查询参数（与 URL 中已有的参数合并），值会转换为字符串。
func SetParamsQueryAny(values map[string]any) Param {
	return func(param *ClientParams) {
		if param.Query == nil {
			param.Query = url.Values{}
		}
		for k, v := range values {
			param.Query.Set(k, stringifyValue(v))
		}
	}
}

// SetParamsHeaders 批量设置请求头（同名的旧值会被覆盖）。
func SetParamsHeaders(h map[string]string) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		for k, v := range h {
			param.Headers.Set(k, v)
		}
	}
}

// SetParamsHeader 设置单个请求头（同名旧值会被覆盖）。需要追加多值时用 AddParamsHeader。
func SetParamsHeader(key, value string) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Set(key, value)
	}
}

// AddParamsHeader 追加一个请求头（保留同名已存在的值）。
func AddParamsHeader(key, value string) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Add(key, value)
	}
}

// SetUserAgent 设置 User-Agent，未设置时默认使用 UserAgentChrome。
func SetUserAgent(userAgent UserAgent) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Set("User-Agent", string(userAgent))
	}
}

// SetCookie 追加一个 Cookie。
func SetCookie(name, value string) Param {
	return func(param *ClientParams) {
		param.Cookies = append(param.Cookies, http.Cookie{Name: name, Value: value})
	}
}

// SetCookies 追加多个 Cookie。
func SetCookies(cookies []http.Cookie) Param {
	return func(param *ClientParams) {
		param.Cookies = append(param.Cookies, cookies...)
	}
}

// SetReferer 设置 Referer。
func SetReferer(referer string) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Set("Referer", referer)
	}
}

// SetBearerToken 设置 Authorization: Bearer <token>。
func SetBearerToken(token string) Param {
	return SetParamsHeader("Authorization", "Bearer "+token)
}

// SetBasicAuth 设置 HTTP Basic 认证。
func SetBasicAuth(username, password string) Param {
	return func(param *ClientParams) {
		req := &http.Request{Header: http.Header{}}
		req.SetBasicAuth(username, password)
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Set("Authorization", req.Header.Get("Authorization"))
	}
}

// SetParamsCookies 覆盖全部 Cookie。
func SetParamsCookies(c []http.Cookie) Param {
	return func(param *ClientParams) { param.Cookies = c }
}

// SetParamsContentType 设置 Content-Type。
func SetParamsContentType(t ContentType) Param {
	return func(param *ClientParams) { param.ContentType = t }
}

// SetParamsTimeout 为本次请求设置独立的超时（基于 context 实现，优先级高于客户端 TimeOut）。
func SetParamsTimeout(d time.Duration) Param {
	return func(param *ClientParams) { param.timeout = d }
}

// FieldType 是表单字段类型。
type FieldType string

const (
	FieldTypeFile FieldType = "file"
	FieldTypeText FieldType = "text"
)

// FormParam 描述一个 multipart 字段。
type FormParam struct {
	FieldName  string    `json:"field_name"`  // 字段名
	FieldValue string    `json:"field_value"` // 字段值
	FieldType  FieldType `json:"field_type"`  // 动作(file/text)
	FileName   string    `json:"file_name"`   // 文件名
	FileBytes  []byte    `json:"file_bytes"`  // 文件内容（内存方式）
	// FileReader 是流式文件内容，优先级高于 FileBytes。不会被序列化，也不会被关闭。
	FileReader io.Reader `json:"-"`
}

// ContentType 是请求体的 MIME 类型。
type ContentType string

const (
	ContentTypeForm        ContentType = "multipart/form-data"
	ContentTypeJson        ContentType = "application/json"
	ContentTypeJsonUtf8    ContentType = "application/json; charset=utf-8"
	ContentTypeXml         ContentType = "application/xml"
	ContentTypeText        ContentType = "text/plain"
	ContentTypeHtml        ContentType = "text/html"
	ContentTypeUrlEncoded  ContentType = "application/x-www-form-urlencoded"
	ContentTypeOctetStream ContentType = "application/octet-stream"
)

// Method 是 HTTP 请求方法。
type Method string

const (
	MethodGet     Method = "GET"
	MethodHead    Method = "HEAD"
	MethodPost    Method = "POST"
	MethodPut     Method = "PUT"
	MethodPatch   Method = "PATCH"
	MethodDelete  Method = "DELETE"
	MethodOptions Method = "OPTIONS"
)

// isBodylessMethod 判断方法是否通常不携带请求体。
func isBodylessMethod(m Method) bool {
	switch m {
	case MethodGet, MethodHead, MethodDelete, MethodOptions:
		return true
	default:
		return false
	}
}

// stringifyValue 把任意值转换为适合放进 URL 查询串/表单的字符串。
// 标量走原生转换，其它类型退化为 JSON。
func stringifyValue(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	case json.Number:
		return value.String()
	case []byte:
		return string(value)
	case fmt.Stringer:
		return value.String()
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(b)
	}
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	out := make(http.Header, len(h))
	for k, vs := range h {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

func cloneValues(v url.Values) url.Values {
	if v == nil {
		return nil
	}
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
