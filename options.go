package curlx

import "time"

type clientOptions struct {
	TimeOut            time.Duration
	InsecureSkipVerify bool
}

func defaultOptions() clientOptions {
	return clientOptions{
		TimeOut: time.Second * 120, // 默认超时120
	}
}

type Option func(*clientOptions)

/**
 * 设置超时时间
 */
func SetOptionTimeOut(t time.Duration) Option {
	return func(options *clientOptions) {
		options.TimeOut = t
	}
}

/**
 * 不校验HTTPS证书
 */
func SetOptionTLSInsecureSkipVerify() Option {
	return func(options *clientOptions) {
		options.InsecureSkipVerify = true
	}
}
