package curlx

import (
	"encoding/json"
	"net/http"
)

type ClientParams struct {
	Url         string
	Method      Method // GET/POST/PUT/DELETE
	Body        []byte
	Headers     http.Header
	Cookies     []http.Cookie
	ContentType ContentType // FORM,JSON,XML
}

func defaultParams() ClientParams {
	return ClientParams{
		Headers: http.Header{},
	}
}

type Param func(*ClientParams)

func SetParamsAll(cp ClientParams) Param {
	return func(param *ClientParams) {
		param.Url = cp.Url
		param.Method = cp.Method
		param.Body = cp.Body
		param.Headers = cp.Headers
		param.Cookies = cp.Cookies
		param.ContentType = cp.ContentType
	}
}

/**
 * 设置URL
 */
func SetParamsUrl(url string) Param {
	return func(param *ClientParams) {
		param.Url = url
	}
}

/**
 * 设置方法
 */
func SetParamsMethod(m Method) Param {
	return func(param *ClientParams) {
		param.Method = m
	}
}

/**
 * 设置参数
 */
func SetParamsBody(by []byte) Param {
	return func(param *ClientParams) {
		param.Body = by
	}
}

func SetParamsBodyAny(v interface{}) Param {
	return func(param *ClientParams) {
		switch value := v.(type) {
		case []byte:
			param.Body = value
		case string:
			param.Body = []byte(value)
		default:
			param.Body, _ = json.Marshal(value)
		}
	}
}

/**
 * 表单文本参数
 */
func SetParamsFormText(fieldName, fieldValue string) Param {
	return func(param *ClientParams) {
		m := []FormParam{}
		if param.Body != nil {
			json.Unmarshal(param.Body, &m)
		}
		m = append(m, FormParam{
			FieldName:  fieldName,
			FieldValue: fieldValue,
			FieldType:  FieldTypeText,
		})

		fp, _ := json.Marshal(m)

		param.Body = fp
	}
}

/**
 * 表单文件上传
 */
func SetParamsFormFile(fieldName, fileName string, fileBytes []byte) Param {
	return func(param *ClientParams) {

		fp := []FormParam{}

		if param.Body != nil {
			json.Unmarshal(param.Body, &fp)
		}

		fp = append(fp, FormParam{
			FieldName: fieldName,
			FieldType: FieldTypeFile,
			FileName:  fileName,
			FileBytes: fileBytes,
		})

		fpb, _ := json.Marshal(fp)

		param.Body = fpb
	}
}

/**
 * 设置请求头
 */
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

/**
 * 设置请求头
 */
func SetParamsHeader(key, value string) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Add(key, value)
	}
}

/**
 * 设置UserAgent
 */
func SetUserAgent(userAgent UserAgent) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Set("User-Agent", string(userAgent))
	}
}

func SetCookie(name, value string) Param {
	return func(param *ClientParams) {
		param.Cookies = append(param.Cookies, http.Cookie{
			Name:  name,
			Value: value,
		})
	}
}

func SetCookies(cookies []http.Cookie) Param {
	return func(param *ClientParams) {
		param.Cookies = append(param.Cookies, cookies...)
	}
}

/**
 * 设置Referer
 */
func SetReferer(referer string) Param {
	return func(param *ClientParams) {
		if param.Headers == nil {
			param.Headers = http.Header{}
		}
		param.Headers.Set("Referer", referer)
	}
}

/**
 * 设置cookies
 */
func SetParamsCookies(c []http.Cookie) Param {
	return func(param *ClientParams) {
		param.Cookies = c
	}
}

/**
 * 设置请求方法
 */
func SetParamsContentType(t ContentType) Param {
	return func(param *ClientParams) {
		param.ContentType = t
	}
}

type FieldType string

const (
	FieldTypeFile FieldType = "file"
	FieldTypeText FieldType = "text"
)

type FormParam struct {
	FieldName  string    `json:"field_name"`  // 字段名
	FieldValue string    `json:"field_value"` // 字段值
	FieldType  FieldType `json:"field_type"`  // 动作(file/text)
	FileName   string    `json:"file_name"`   // 文件名
	FileBytes  []byte    `json:"file_bytes"`  // 文件内容
}

type ContentType string

const (
	ContentTypeForm       ContentType = "multipart/form-data"
	ContentTypeJson       ContentType = "application/json"
	ContentTypeXml        ContentType = "application/xml"
	ContentTypeText       ContentType = "text/plain"
	ContentTypeUrlEncoded ContentType = "application/x-www-form-urlencoded"
)

type Method string

const (
	MethodGet  Method = "GET"
	MethodPost Method = "POST"
)
