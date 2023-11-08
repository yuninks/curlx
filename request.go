package curlx

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type FormParam struct {
	Key    string `json:"key"`    // 字段名
	Action string `json:"action"` // 动作(file/text)
	Name   string `json:"name"`   // 文件名
	Bytes  []byte `json:"bytes"`  // 文件内容
	Value  string `json:"value"`  // 值
}

/**
 * 处理请求类型
 */
func (p *CurlParams) parseMethod() error {
	if p.Method == "" {
		return errors.New("请求类型不能为空")
	}
	return nil
}

/**
 * 处理URL
 */
func (p *CurlParams) parseUrl() error {
	_, err := url.Parse(p.Url)
	if err != nil {
		return err
	}
	return nil
}

/**
 * 处理请求头Header
 */
func (p *CurlParams) parseHeaders(r *http.Request) {
	if p.Headers != nil {
		if r.Header.Get("User-Agent") == "" {
			r.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:87.0) Gecko/20100101 Firefox/87.0 Send By Golang")
		}
		for k, v := range p.Headers {
			if vv, ok := v.(string); ok {
				r.Header.Set(k, vv)
				continue
			}
			if vv, ok := v.([]string); ok {
				for _, vvv := range vv {
					r.Header.Add(k, vvv)
				}
			}
		}
	}
}

/**
 * 处理请求参数
 */
func (p *CurlParams) parseParams() (str io.Reader, err error) {
	err = nil

	// 初始化(如未初始化)
	if p.Headers == nil {
		p.Headers = make(map[string]interface{})
	}

	if p.Params != nil {
		if p.DataType == DataTypeJson {
			// 判断是否存在
			if _, ok := p.Headers["Content-Type"]; !ok {
				p.Headers["Content-Type"] = "application/json"
			}
			strParam, ok := p.Params.(string)
			if ok {
				return bytes.NewReader([]byte(strParam)), nil
			}
			b, err := json.Marshal(p.Params)
			if err == nil {
				return bytes.NewReader(b), nil
			}
		} else if p.DataType == DataTypeForm {
			// 表单上传（可能有文件）
			// 文件上传的
			formParam, ok := p.Params.([]*FormParam)
			if ok {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				for _, v := range formParam {
					if v.Action == "file" {
						part, _ := writer.CreateFormFile(v.Key, v.Name)
						io.Copy(part, strings.NewReader(v.Value))
					} else {
						_ = writer.WriteField(v.Key, v.Value)
					}
				}
				writer.Close()
				p.Headers["Content-Type"] = writer.FormDataContentType()
				return body, nil
			}
			return nil, errors.New("表单上传的参数格式不正确")

		} else if p.DataType == DataTypeXml {
			if _, ok := p.Headers["Content-Type"]; !ok {
				p.Headers["Content-Type"] = "application/xml"
			}
			var string_data string
			if value, ok := p.Params.(string); ok {
				string_data = string(value)
			} else {
				var by []byte
				by, err = xml.Marshal(p.Params)
				if err != nil {
					return
				}
				string_data = string(by)
			}
			return strings.NewReader(string_data), nil
		} else if p.DataType == DataTypeText {
			if _, ok := p.Headers["Content-Type"]; !ok {
				p.Headers["Content-Type"] = "text/plain"
			}

			var string_data string
			if value, ok := p.Params.(string); ok {
				string_data = string(value)
			} else {
				err = errors.New("TEXT类型的参数仅支持字符串")
				return
			}

			return strings.NewReader(string_data), nil
		} else if p.DataType == DataTypeUrlEncoded {
			if _, ok := p.Headers["Content-Type"]; !ok {
				p.Headers["Content-Type"] = "application/x-www-form-urlencoded"
			}

			// 判断需要map[string]interface{}类型
			paramValue, ok := p.Params.(map[string]interface{})
			if !ok {
				return strings.NewReader(""), errors.New("参数需map[string]interface{}")
			}

			values := url.Values{}
			for k, v := range paramValue {
				// 字符串
				if v_string, ok := v.(string); ok {
					values.Set(k, v_string)
				}
				// 字符串切片
				if vv, ok := v.([]string); ok {
					for _, vvv := range vv {
						values.Add(k+"[]", vvv)
					}
				}
				// int转string
				if v_int, ok := v.(int); ok {
					values.Set(k, strconv.Itoa(v_int))
				}
				// int64转string
				if v_int64, ok := v.(int64); ok {
					values.Set(k, strconv.FormatInt(v_int64, 10))
				}
				// float32转string
				if v_float32, ok := v.(float32); ok {
					values.Set(k, strconv.FormatFloat(float64(v_float32), 'f', -1, 32))
				}
				// float64转string
				if v_float64, ok := v.(float64); ok {
					values.Set(k, strconv.FormatFloat(v_float64, 'f', -1, 64))
				}
			}
			return strings.NewReader(values.Encode()), nil
		} else {
			return nil, errors.New("不支持的数据类型")
		}

	}
	return
}

/**
 * 处理Cookie
 */
func (p *CurlParams) parseCookies(r *http.Request) {
	switch p.Cookies.(type) {
	case string:
		cookies := p.Cookies.(string)
		r.Header.Add("Cookie", cookies)
	case map[string]string:
		cookies := p.Cookies.(map[string]string)
		for k, v := range cookies {
			r.AddCookie(&http.Cookie{
				Name:  k,
				Value: v,
			})
		}
	case []*http.Cookie:
		cookies := p.Cookies.([]*http.Cookie)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
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
