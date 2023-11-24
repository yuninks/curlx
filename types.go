package curlx

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

type method string

const (
	MethodGet  method = "GET"
	MethodPost method = "POST"
)

type CurlParams struct {
	Url         string
	Method      method // GET/POST
	Params      interface{}
	Headers     map[string]interface{}
	Cookies     interface{}
	ContentType ContentType // FORM,JSON,XML
}
