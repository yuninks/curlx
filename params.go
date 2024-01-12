package curlx

type ClientParams struct {
	Url         string
	Method      Method // GET/POST
	Body        interface{}
	Headers     map[string]interface{}
	Cookies     interface{}
	ContentType ContentType // FORM,JSON,XML
}

func defaultParams() ClientParams {
	return ClientParams{}
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
func SetParamsBody(p interface{}) Param {
	return func(param *ClientParams) {
		param.Body = p
	}
}

/**
 * 表单文本参数
 */
func SetParamsFormText(fieldName, fieldValue string) Param {
	return func(param *ClientParams) {
		fp := param.Body.([]FormParam)
		fp = append(fp, FormParam{
			FieldName:  fieldName,
			FieldValue: fieldValue,
			FieldType:  FieldTypeText,
		})
		param.Body = fp
	}
}

/**
 * 表单文件上传
 */
func SetParamsFormFile(fieldName, fileName string, fileBytes []byte) Param {
	return func(param *ClientParams) {
		fp := param.Body.([]FormParam)
		fp = append(fp, FormParam{
			FieldName: fieldName,
			FieldType: FieldTypeFile,
			FileName:  fileName,
			FileBytes: fileBytes,
		})
		param.Body = fp
	}
}

/**
 * 设置请求头
 */
func SetParamsHeaders(h map[string]interface{}) Param {
	return func(param *ClientParams) {
		param.Headers = h
	}
}

/**
 * 设置cookies
 */
func SetParamsCookies(c interface{}) Param {
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
