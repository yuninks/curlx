package curlx

import (
	"context"
	"log"
	"time"
)

type ClientOptions struct {
	TimeOut             time.Duration
	InsecureSkipVerify  bool
	Logger              OptionLogger
	LoggerLength        int // 日志输出长度
	CertFingerprint     string // 证书指纹验证
	
	// 连接池配置
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
}

func defaultOptions() ClientOptions {
	return ClientOptions{
		TimeOut:             time.Second * 120, // 默认超时120秒
		Logger:              defaultLogger{},
		LoggerLength:        100,
		MaxIdleConns:        100,              // 默认连接池大小
		MaxIdleConnsPerHost: 10,               // 每主机默认空闲连接数
		MaxConnsPerHost:     50,               // 每主机最大连接数
		IdleConnTimeout:     90 * time.Second, // 空闲连接超时
	}
}

type Option func(*ClientOptions)

/**
 * 设置超时时间
 */
func WithOptionTimeOut(t time.Duration) Option {
	return func(options *ClientOptions) {
		options.TimeOut = t
	}
}

// 添加证书指纹验证选项
func WithOptionTLSPin(certFingerprint string) Option {
    return func(options *ClientOptions) {
        options.CertFingerprint = certFingerprint
    }
}

/**
 * 不校验HTTPS证书
 */
func WithOptionTLSInsecureSkipVerify() Option {
	return func(options *ClientOptions) {
		options.InsecureSkipVerify = true
	}
}

/**
 * 设置日志输出
 */
func WithOptionLog(log OptionLogger) Option {
	return func(options *ClientOptions) {
		options.Logger = log
	}
}

/**
 * 设置日志输出长度
 */
func WithOptionLoggerLength(length int) Option {
	return func(options *ClientOptions) {
		options.LoggerLength = length
	}
}

// 连接池配置选项
func WithMaxIdleConns(maxIdleConns int) Option {
	return func(options *ClientOptions) {
		options.MaxIdleConns = maxIdleConns
	}
}

func WithMaxIdleConnsPerHost(maxIdleConnsPerHost int) Option {
	return func(options *ClientOptions) {
		options.MaxIdleConnsPerHost = maxIdleConnsPerHost
	}
}

func WithMaxConnsPerHost(maxConnsPerHost int) Option {
	return func(options *ClientOptions) {
		options.MaxConnsPerHost = maxConnsPerHost
	}
}

func WithIdleConnTimeout(timeout time.Duration) Option {
	return func(options *ClientOptions) {
		options.IdleConnTimeout = timeout
	}
}

type OptionLogger interface {
	Infof(ctx context.Context, format string, args ...any)
	Errorf(ctx context.Context, format string, args ...any)
}

type defaultLogger struct{}

func (defaultLogger) Errorf(ctx context.Context, format string, args ...any) {
	// 输出日志
	log.Printf(format, args...)
}

func (defaultLogger) Infof(ctx context.Context, format string, args ...any) {
	// 输出日志
	log.Printf(format, args...)
}
