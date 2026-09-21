package curlx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

/**
 * 处理请求类型
 */
func (p *ClientParams) parseMethod() error {
	p.Method = Method(strings.ToUpper(strings.TrimSpace(string(p.Method))))
	if p.Method == "" {
		return fmt.Errorf("curlx: 请求方法不能为空")
	}
	return nil
}

/**
 * 处理URL
 *
 * 缺少协议时补全 http://；协议与主机名非法时提前报错，而不是等到 client.Do 才失败。
 * 这里刻意不做完整解析（http.NewRequestWithContext 还会解析一次），
 * 只做零分配的快速校验，避免每次请求多一次 net/url 解析开销。
 */
func (p *ClientParams) parseUrl() error {
	p.Url = strings.TrimSpace(p.Url)
	if p.Url == "" {
		return fmt.Errorf("curlx: 请求URL不能为空")
	}
	if !strings.Contains(p.Url, "://") {
		p.Url = "http://" + p.Url
	}

	scheme, rest, _ := strings.Cut(p.Url, "://")
	switch {
	case strings.EqualFold(scheme, "http"), strings.EqualFold(scheme, "https"):
	default:
		return fmt.Errorf("curlx: 不支持的URL协议 %q", scheme)
	}

	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}
	if host == "" {
		return fmt.Errorf("curlx: URL缺少主机名: %s", p.Url)
	}
	// 其余畸形写法（如主机名带空格）由 http.NewRequestWithContext 报错
	return nil
}

// setHeader 在需要时惰性创建 Header map。
func (p *ClientParams) setHeader(key, value string) {
	if p.Headers == nil {
		p.Headers = http.Header{}
	}
	p.Headers.Set(key, value)
}

/**
 * 处理请求头Header
 * 复制而非引用，避免把内部 map 暴露给 http.Request，也避免污染调用方数据。
 */
func (p *ClientParams) parseHeaders(r *http.Request) {
	for k, vs := range p.Headers {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	if r.Header.Get("User-Agent") == "" {
		r.Header.Set("User-Agent", string(UserAgentChrome))
	}
}

// canonicalizeHeaders 把 Header 的 key 统一成规范形式。
//
// 直接构造 http.Header{"content-type": ...} 时 key 不是规范的，
// 而 http.Header.Get 是按规范 key 查表的，会导致取值失败（历史上会发出两个 Content-Type）。
// 这里按 key 排序后重新写入，保证结果稳定。
func (p *ClientParams) canonicalizeHeaders() {
	if len(p.Headers) == 0 {
		return
	}
	keys := make([]string, 0, len(p.Headers))
	for k := range p.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(http.Header, len(p.Headers))
	for _, k := range keys {
		for _, v := range p.Headers[k] {
			out.Add(k, v)
		}
	}
	p.Headers = out
}

/**
 * 处理请求参数
 */
func (p *ClientParams) parseParams() (io.Reader, error) {
	if p.bodyErr != nil {
		return nil, p.bodyErr
	}
	p.canonicalizeHeaders()

	if len(p.Query) > 0 {
		if err := p.applyQuery(p.Query); err != nil {
			return nil, err
		}
	}

	switch p.ContentType {
	case ContentTypeForm:
		return p.parseForm()
	case ContentTypeUrlEncoded:
		return p.parseUrlEncoded()
	}
	// 使用方只设置了 FormValues 而没设置 Content-Type 时，按 urlencoded 处理
	if len(p.FormValues) > 0 {
		return p.parseUrlEncoded()
	}

	if p.ContentType != "" && p.Headers.Get("Content-Type") == "" {
		p.setHeader("Content-Type", string(p.ContentType))
	}

	if len(p.Body) == 0 {
		return nil, nil
	}

	// 兼容历史行为：GET 等无 body 的方法在未指定 Content-Type 时，把 JSON body 当作 query 参数
	if p.ContentType == "" && isBodylessMethod(p.Method) {
		return p.parseQueryFromBody()
	}

	// 其余情况原样透传，不再因为"没设置 Content-Type"而直接报错
	return bytes.NewReader(p.Body), nil
}

// applyQuery 把参数合并到 URL 上。
func (p *ClientParams) applyQuery(values url.Values) error {
	u, err := url.Parse(p.Url)
	if err != nil {
		return fmt.Errorf("curlx: 解析URL失败: %w", err)
	}
	q := u.Query()
	for k, vs := range values {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
	p.Url = u.String()
	return nil
}

// parseQueryFromBody 把 JSON 对象 body 转成查询参数。
func (p *ClientParams) parseQueryFromBody() (io.Reader, error) {
	m := map[string]any{}
	if err := json.Unmarshal(p.Body, &m); err != nil {
		// 不是 JSON 对象就按原始 body 发送
		return bytes.NewReader(p.Body), nil
	}
	values := url.Values{}
	for k, v := range m {
		values.Set(k, stringifyValue(v))
	}
	if err := p.applyQuery(values); err != nil {
		return nil, err
	}
	return nil, nil
}

// parseUrlEncoded 处理 application/x-www-form-urlencoded。
func (p *ClientParams) parseUrlEncoded() (io.Reader, error) {
	p.setHeader("Content-Type", string(ContentTypeUrlEncoded))
	if len(p.FormValues) > 0 {
		return strings.NewReader(p.FormValues.Encode()), nil
	}
	if len(p.Body) == 0 {
		return nil, nil
	}
	// 兼容历史行为：body 是 JSON 对象时转成表单
	m := map[string]any{}
	if err := json.Unmarshal(p.Body, &m); err == nil {
		values := url.Values{}
		for k, v := range m {
			values.Set(k, stringifyValue(v))
		}
		return strings.NewReader(values.Encode()), nil
	}
	// 已经是 "a=1&b=2" 形式，原样发送
	return bytes.NewReader(p.Body), nil
}

// formParams 返回本次请求的 multipart 字段列表。
func (p *ClientParams) formParams() ([]FormParam, error) {
	if len(p.Forms) > 0 {
		return p.Forms, nil
	}
	if len(p.Body) == 0 {
		return nil, nil
	}
	// 兼容历史用法：Body 是 []FormParam 的 JSON
	params := []FormParam{}
	if err := json.Unmarshal(p.Body, &params); err != nil {
		return nil, fmt.Errorf("curlx: 解析表单参数失败: %w", err)
	}
	return params, nil
}

// parseForm 处理 multipart/form-data。
//
// 全部为内存字段时一次性构建（Content-Length 已知，可跟随重定向）；
// 存在流式 reader 时通过 io.Pipe 边写边发，避免大文件整体驻留内存。
func (p *ClientParams) parseForm() (io.Reader, error) {
	params, err := p.formParams()
	if err != nil {
		return nil, err
	}

	streaming := false
	for _, v := range params {
		if v.FieldType == FieldTypeFile && v.FileReader != nil {
			streaming = true
			break
		}
	}

	if !streaming {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for _, v := range params {
			if err := writeFormField(writer, v); err != nil {
				_ = writer.Close()
				return nil, fmt.Errorf("curlx: 构建表单失败: %w", err)
			}
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("curlx: 构建表单失败: %w", err)
		}
		p.setHeader("Content-Type", writer.FormDataContentType())
		return body, nil
	}

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	p.setHeader("Content-Type", writer.FormDataContentType())
	// 请求体被运输层关闭（例如建连失败）时，同时关闭写端，避免写入 goroutine 永久阻塞
	body := &pipeBody{reader: pr, writer: pw}
	go func() {
		var writeErr error
		for _, v := range params {
			if writeErr = writeFormField(writer, v); writeErr != nil {
				break
			}
		}
		if closeErr := writer.Close(); writeErr == nil {
			writeErr = closeErr
		}
		// 把写入错误传递给读取端，最终由 client.Do 返回给调用方
		_ = pw.CloseWithError(writeErr)
	}()
	return body, nil
}

// pipeBody 把 io.Pipe 的两端绑在一起，Close 时同时关闭写端。
type pipeBody struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (b *pipeBody) Read(p []byte) (int, error) { return b.reader.Read(p) }

func (b *pipeBody) Close() error {
	err := b.reader.Close()
	_ = b.writer.CloseWithError(err)
	return err
}

// writeFormField 写入一个 multipart 字段。
func writeFormField(w *multipart.Writer, v FormParam) error {
	if v.FieldType != FieldTypeFile {
		return w.WriteField(v.FieldName, v.FieldValue)
	}
	part, err := createFormFilePart(w, v.FieldName, v.FileName)
	if err != nil {
		return err
	}
	if v.FileReader != nil {
		_, err = io.Copy(part, v.FileReader)
		return err
	}
	_, err = part.Write(v.FileBytes)
	return err
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// createFormFilePart 创建文件字段，并按扩展名给出更准确的 Content-Type。
func createFormFilePart(w *multipart.Writer, fieldName, fileName string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
		quoteEscaper.Replace(fieldName), quoteEscaper.Replace(fileName)))
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	if contentType == "" {
		contentType = string(ContentTypeOctetStream)
	}
	h.Set("Content-Type", contentType)
	return w.CreatePart(h)
}

/**
 * 处理Cookie
 */
func (p *ClientParams) parseCookies(r *http.Request) {
	for i := range p.Cookies {
		r.AddCookie(&p.Cookies[i])
	}
}
