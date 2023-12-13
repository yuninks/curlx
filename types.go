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


type UserAgent string

const(
	UserAgentChrome UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
	UserAgentFirefox UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:61.0) Gecko/20100101 "
	UserAgentSafari UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_6) AppleWebKit/605.1.15 "
	UserAgentOpera UserAgent = "Opera/9.80 (Windows NT 6.1; WOW64) Presto/2.12.388 Version/12.18 "
	UserAgentIE UserAgent = "Mozilla/5.0 (compatible; MSIE 9.0; Windows NT 6.1; Trident/5.0; "
	UserAgentEdge UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
	UserAgentQQ UserAgent = "Mozilla/5.0 (compatible; MSIE 9.0; Windows NT 6.1; Trident/5.0; "
	UserAgentMaxthon UserAgent = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 "
	UserAgentUC UserAgent = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 "
	UserAgentSougou UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgentLBBROWSER UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgent2345 UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgentQihu UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgentXiaoMi UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgentQuark UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgentQiyu UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgentWechat UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
	UserAgentTaobao UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 "
)