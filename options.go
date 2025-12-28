package curlx

import (
	"context"
	"log"
	"time"
)

type clientOptions struct {
	TimeOut            time.Duration
	InsecureSkipVerify bool
	Logger             OptionLogger
	LoggerLength       int // 日志输出长度
}

func defaultOptions() clientOptions {
	return clientOptions{
		TimeOut:      time.Second * 120, // 默认超时120
		Logger:       defaultLogger{},
		LoggerLength: 100,
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

/**
 * 设置日志输出
 */
func SetOptionLog(log OptionLogger) Option {
	return func(options *clientOptions) {
		options.Logger = log
	}
}

/**
 * 设置日志输出长度
 */
func WithLoggerLength(length int) Option {
	return func(options *clientOptions) {
		options.LoggerLength = length
	}
}

type OptionLogger interface {
	Infof(ctx context.Context, format string, args ...interface{})
	Errorf(ctx context.Context, format string, args ...interface{})
}

type defaultLogger struct{}

func (defaultLogger) Errorf(ctx context.Context, format string, args ...interface{}) {
	// 输出日志
	log.Printf(format, args...)
}

func (defaultLogger) Infof(ctx context.Context, format string, args ...interface{}) {
	// 输出日志
	log.Printf(format, args...)
}
