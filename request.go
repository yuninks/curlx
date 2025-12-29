package curlx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"code.yun.ink/pkg/convx"
)

/**
 * 处理请求类型
 */
func (p *ClientParams) parseMethod() error {
	if p.Method == "" {
		return errors.New("请求类型不能为空")
	}
	return nil
}

/**
 * 处理URL
 */
func (p *ClientParams) parseUrl() error {
	_, err := url.Parse(p.Url)
	if err != nil {
		return err
	}
	return nil
}

/**
 * 处理请求头Header
 */
func (p *ClientParams) parseHeaders(r *http.Request) {

	if p.Headers.Get("User-Agent") == "" {
		p.Headers.Add("User-Agent", string(UserAgentChrome))
	}

	r.Header = p.Headers

}

/**
 * 处理请求参数
 */
func (p *ClientParams) parseParams() (str io.Reader, err error) {
	err = nil

	// 初始化(如未初始化)
	if p.Headers == nil {
		p.Headers = http.Header{}
	}

	// 添加Content-Type
	if _, ok := p.Headers["Content-Type"]; !ok {
		p.Headers.Set("Content-Type", string(p.ContentType))
	}

	if len(p.Body) == 0 {
		return nil, nil
	}

	switch p.ContentType {
	case ContentTypeJson:
		// JSON
		return bytes.NewReader(p.Body), nil
	case ContentTypeForm:
		// 表单
		params := []FormParam{}
		err = json.Unmarshal(p.Body, &params)
		if err != nil {
			return nil, err
		}
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for _, v := range params {
			if v.FieldType == FieldTypeFile {
				part, _ := writer.CreateFormFile(v.FieldName, v.FileName)
				io.Copy(part, bytes.NewBuffer(v.FileBytes))
			} else {
				_ = writer.WriteField(v.FieldName, v.FieldValue)
			}
		}
		writer.Close()
		p.Headers.Set("Content-Type", writer.FormDataContentType())
		return body, nil
	case ContentTypeXml:
		// XML
		return bytes.NewReader(p.Body), nil
	case ContentTypeText:
		// TEXT
		return bytes.NewReader(p.Body), nil
	case ContentTypeUrlEncoded:
		// URL编码
		m := map[string]any{}
		if err = json.Unmarshal(p.Body, &m); err != nil {
			return nil, err
		}
		values := url.Values{}
		for k, v := range m {
			val := convx.ToString(v)
			values.Set(k, val)
		}

		return strings.NewReader(values.Encode()), nil
	default:
		if p.Method == MethodGet {

			m := map[string]any{}
			if err = json.Unmarshal(p.Body, &m); err != nil {
				return nil, err
			}

			url, err := url.Parse(p.Url) // 解析URL
			if err != nil {
				return nil, err
			}
			query := url.Query()
			for k, v := range m {
				val := convx.ToString(v)
				query[k] = append(query[k], val)
			}
			url.RawQuery = query.Encode()
			p.Url = url.String()

		} else {
			return nil, errors.New("curlx 不支持的数据类型")
		}

	}
	return
}

/**
 * 处理Cookie
 */
func (p *ClientParams) parseCookies(r *http.Request) {
	for _, cookie := range p.Cookies {
		r.AddCookie(&cookie)
	}
}

// func (r *Request) parseQuery() {
// 	switch r.opts.Query.(type) {
// 	case string:
// 		str := r.opts.Query.(string)
// 		r.req.URL.RawQuery = str
// 	case map[string]interface{}:
// 		q := r.req.URL.Query()
// 		for k, v := range r.opts.Query.(map[string]interface{}) {
// 			if vv, ok := v.(string); ok {
// 				q.Set(k, vv)
// 				continue
// 			}
// 			if vv, ok := v.([]string); ok {
// 				for _, vvv := range vv {
// 					q.Add(k, vvv)
// 				}
// 			}
// 		}
// 		r.req.URL.RawQuery = q.Encode()
// 	}
// }
