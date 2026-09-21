package curlx

// UserAgent 预置的浏览器 User-Agent。
//
// 注意：这些字符串只是"看起来像浏览器"，不保证与真实浏览器版本一致，
// 也不保证目标站点接受。需要精确控制时请使用 SetUserAgent 传入自己的值。
type UserAgent string

const (
	UserAgentChrome  UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	UserAgentEdge    UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36 Edg/123.0.0.0"
	UserAgentFirefox UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0"
	UserAgentIE      UserAgent = "Mozilla/5.0 (compatible; MSIE 9.0; Windows NT 6.1; Trident/5.0)"
	UserAgentWechat  UserAgent = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/53.0.2785.116 Safari/537.36 MicroMessenger/6.5.16.1000"
)
