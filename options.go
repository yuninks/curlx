package curlx

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ClientOptions 是客户端的全部可配置项，请通过 Option 修改，不要在构造后直接改动。
type ClientOptions struct {
	// TimeOut 整个请求（含读取响应体）的超时时间。
	TimeOut time.Duration
	// InsecureSkipVerify 是否跳过 HTTPS 证书校验（危险，仅调试使用）。
	InsecureSkipVerify bool
	// CertFingerprint 证书指纹（见 WithTLSPin）。
	CertFingerprint string
	// Logger 日志实现，默认 NoopLogger（不输出任何日志）。
	Logger OptionLogger
	// LoggerLength 日志中请求/响应体的最大字符数，<=0 表示不输出 body。
	LoggerLength int
	// LogRequestBody 是否把请求体写入日志（默认 false，避免泄漏密码等敏感数据）。
	LogRequestBody bool
	// LogResponseBody 是否把响应体写入日志（默认 true）。
	LogResponseBody bool

	// 连接池配置
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration

	// 传输层配置
	DialTimeout            time.Duration
	KeepAlive              time.Duration
	ResponseHeaderTimeout  time.Duration
	MaxResponseHeaderBytes int64
	// MaxResponseBytes 响应体最大字节数，0 表示不限制。超过时返回 ErrResponseTooLarge。
	MaxResponseBytes int64

	// logEnabled 表示 Logger 不是 NoopLogger。为 false 时跳过所有日志参数的构造，
	// 避免在热路径上产生无谓的分配（默认配置下即为此状态）。
	logEnabled bool
}

func defaultOptions() ClientOptions {
	return ClientOptions{
		TimeOut:        120 * time.Second,
		Logger:         NoopLogger{},
		LoggerLength:   100,
		LogRequestBody: false,

		LogResponseBody: true,

		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,

		DialTimeout: 30 * time.Second,
		KeepAlive:   30 * time.Second,
	}
}

type Option func(*ClientOptions)

// WithTimeout 设置整个请求的超时时间（含建立连接、读取响应体）。
func WithTimeout(t time.Duration) Option {
	return func(options *ClientOptions) { options.TimeOut = t }
}

// WithOptionTimeOut 已废弃，请使用 WithTimeout。
//
// Deprecated: use WithTimeout.
func WithOptionTimeOut(t time.Duration) Option { return WithTimeout(t) }

// WithLogger 设置日志实现。传入 nil 等同于不输出日志。
func WithLogger(logger OptionLogger) Option {
	return func(options *ClientOptions) {
		if logger == nil {
			options.Logger = NoopLogger{}
			return
		}
		options.Logger = logger
	}
}

// WithOptionLog 已废弃，请使用 WithLogger。
//
// Deprecated: use WithLogger.
func WithOptionLog(logger OptionLogger) Option { return WithLogger(logger) }

// WithLoggerLength 设置日志中 body 的最大字符数，<=0 表示不在日志中输出 body。
func WithLoggerLength(length int) Option {
	return func(options *ClientOptions) { options.LoggerLength = length }
}

// WithOptionLoggerLength 已废弃，请使用 WithLoggerLength。
//
// Deprecated: use WithLoggerLength.
func WithOptionLoggerLength(length int) Option { return WithLoggerLength(length) }

// WithLogRequestBody 设置是否把请求体写入日志（默认 false）。开启前请确认内容不含敏感数据。
func WithLogRequestBody(enabled bool) Option {
	return func(options *ClientOptions) { options.LogRequestBody = enabled }
}

// WithLogResponseBody 设置是否把响应体写入日志（默认 true）。
func WithLogResponseBody(enabled bool) Option {
	return func(options *ClientOptions) { options.LogResponseBody = enabled }
}

// WithTLSInsecureSkipVerify 跳过 HTTPS 证书校验（危险，仅用于调试或自签名环境）。
func WithTLSInsecureSkipVerify() Option {
	return func(options *ClientOptions) { options.InsecureSkipVerify = true }
}

// WithOptionTLSInsecureSkipVerify 已废弃，请使用 WithTLSInsecureSkipVerify。
//
// Deprecated: use WithTLSInsecureSkipVerify.
func WithOptionTLSInsecureSkipVerify() Option { return WithTLSInsecureSkipVerify() }

// WithTLSPin 开启证书指纹校验（证书固定 / certificate pinning）。
//
// 支持两种写法：
//   - 64 位十六进制（可带冒号），对叶子证书的 DER 做 SHA-256，例如
//     "a1b2...:" -> "a1b2c3...e5"；
//   - "sha256/<base64>"，对证书的 SubjectPublicKeyInfo 做 SHA-256（HPKP 风格）。
//
// 自签名证书需要同时开启 WithTLSInsecureSkipVerify，此时校验完全由指纹承担。
// 指纹非法时会"失败关闭"：所有 HTTPS 请求都会返回错误，而不是静默忽略。
func WithTLSPin(certFingerprint string) Option {
	return func(options *ClientOptions) { options.CertFingerprint = strings.TrimSpace(certFingerprint) }
}

// WithOptionTLSPin 已废弃，请使用 WithTLSPin。
//
// Deprecated: use WithTLSPin.
func WithOptionTLSPin(certFingerprint string) Option { return WithTLSPin(certFingerprint) }

// WithMaxIdleConns 设置连接池总空闲连接数。
func WithMaxIdleConns(maxIdleConns int) Option {
	return func(options *ClientOptions) { options.MaxIdleConns = maxIdleConns }
}

// WithMaxIdleConnsPerHost 设置每个主机的空闲连接数。
func WithMaxIdleConnsPerHost(maxIdleConnsPerHost int) Option {
	return func(options *ClientOptions) { options.MaxIdleConnsPerHost = maxIdleConnsPerHost }
}

// WithMaxConnsPerHost 设置每个主机的最大连接数（含正在使用的连接）。
func WithMaxConnsPerHost(maxConnsPerHost int) Option {
	return func(options *ClientOptions) { options.MaxConnsPerHost = maxConnsPerHost }
}

// WithIdleConnTimeout 设置空闲连接的超时时间。
func WithIdleConnTimeout(timeout time.Duration) Option {
	return func(options *ClientOptions) { options.IdleConnTimeout = timeout }
}

// WithDialTimeout 设置建立 TCP 连接的超时时间。
func WithDialTimeout(timeout time.Duration) Option {
	return func(options *ClientOptions) { options.DialTimeout = timeout }
}

// WithKeepAlive 设置 TCP keep-alive 周期，<=0 表示关闭 keep-alive 探测。
func WithKeepAlive(keepAlive time.Duration) Option {
	return func(options *ClientOptions) { options.KeepAlive = keepAlive }
}

// WithResponseHeaderTimeout 设置等待响应头的超时时间，0 表示不额外限制（由 TimeOut 兜底）。
func WithResponseHeaderTimeout(timeout time.Duration) Option {
	return func(options *ClientOptions) { options.ResponseHeaderTimeout = timeout }
}

// WithMaxResponseHeaderBytes 设置响应头的最大字节数，0 表示使用标准库默认值。
func WithMaxResponseHeaderBytes(n int64) Option {
	return func(options *ClientOptions) { options.MaxResponseHeaderBytes = n }
}

// WithMaxResponseBytes 设置响应体的最大字节数，0 表示不限制。
// 超过限制时 GetBody 返回 ErrResponseTooLarge，避免异常服务端耗尽内存。
func WithMaxResponseBytes(n int64) Option {
	return func(options *ClientOptions) {
		if n < 0 {
			n = 0
		}
		options.MaxResponseBytes = n
	}
}

// OptionLogger 是日志接口，便于接入 zap/logrus 等实现。
type OptionLogger interface {
	Infof(ctx context.Context, format string, args ...any)
	Errorf(ctx context.Context, format string, args ...any)
}

// NoopLogger 是默认日志实现，不输出任何内容。
type NoopLogger struct{}

func (NoopLogger) Infof(ctx context.Context, format string, args ...any)  {}
func (NoopLogger) Errorf(ctx context.Context, format string, args ...any) {}

// StdLogger 使用标准库 log 输出日志。
type StdLogger struct{}

// NewStdLogger 返回一个基于标准库 log 的日志实现。
func NewStdLogger() OptionLogger { return StdLogger{} }

func (StdLogger) Infof(ctx context.Context, format string, args ...any) {
	log.Printf(format, args...)
}

func (StdLogger) Errorf(ctx context.Context, format string, args ...any) {
	log.Printf(format, args...)
}

// sensitiveHeaderNames 中的请求/响应头在写日志时会被脱敏。
var sensitiveHeaderNames = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"x-api-key":           {},
	"api-key":             {},
	"x-auth-token":        {},
	"x-csrf-token":        {},
}

// redactHeaders 返回脱敏后的头副本，避免 token/cookie 进入日志。
func redactHeaders(h http.Header) http.Header {
	if len(h) == 0 {
		return h
	}
	out := make(http.Header, len(h))
	for k, vs := range h {
		if _, ok := sensitiveHeaderNames[strings.ToLower(k)]; ok {
			out[k] = []string{"[REDACTED]"}
			continue
		}
		out[k] = vs
	}
	return out
}

// truncateForLog 把 body 截断为最多 maxRunes 个字符，<=0 表示不输出。
func truncateForLog(b []byte, maxRunes int) string {
	if maxRunes <= 0 || len(b) == 0 {
		return ""
	}
	runes := []rune(string(b))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "...(truncated)"
}

// buildTLSConfig 构造 TLS 配置；指纹非法时返回一个"全部拒绝"的配置。
func (o ClientOptions) buildTLSConfig() *tls.Config {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: o.InsecureSkipVerify, //nolint:gosec // 由调用方显式开启
	}
	if o.CertFingerprint == "" {
		return cfg
	}
	pin, spki, err := parseFingerprint(o.CertFingerprint)
	if err != nil {
		cfg.VerifyPeerCertificate = func([][]byte, [][]*x509.Certificate) error { return err }
		return cfg
	}
	cfg.VerifyPeerCertificate = fingerprintVerifier(pin, spki)
	return cfg
}

// parseFingerprint 解析指纹配置，返回 SHA-256 摘要以及是否为 SPKI 指纹。
func parseFingerprint(s string) (pin []byte, spki bool, err error) {
	const prefix = "sha256/"
	if strings.HasPrefix(strings.ToLower(s), prefix) {
		raw, decErr := base64.StdEncoding.DecodeString(s[len(prefix):])
		if decErr != nil {
			return nil, false, fmt.Errorf("curlx: invalid certificate fingerprint %q: %w", s, decErr)
		}
		if len(raw) != sha256.Size {
			return nil, false, fmt.Errorf("curlx: invalid certificate fingerprint %q: want %d bytes, got %d", s, sha256.Size, len(raw))
		}
		return raw, true, nil
	}
	cleaned := strings.NewReplacer(":", "", " ", "", "-", "").Replace(s)
	raw, decErr := hex.DecodeString(cleaned)
	if decErr != nil {
		return nil, false, fmt.Errorf("curlx: invalid certificate fingerprint %q: %w", s, decErr)
	}
	if len(raw) != sha256.Size {
		return nil, false, fmt.Errorf("curlx: invalid certificate fingerprint %q: want %d bytes, got %d", s, sha256.Size, len(raw))
	}
	return raw, false, nil
}

// fingerprintVerifier 校验叶子证书（或证书链中任意一张）的指纹。
func fingerprintVerifier(pin []byte, spki bool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		for _, raw := range rawCerts {
			var sum [sha256.Size]byte
			if spki {
				cert, err := x509.ParseCertificate(raw)
				if err != nil {
					continue
				}
				sum = sha256.Sum256(cert.RawSubjectPublicKeyInfo)
			} else {
				sum = sha256.Sum256(raw)
			}
			if subtle.ConstantTimeCompare(sum[:], pin) == 1 {
				return nil
			}
		}
		return errors.New("curlx: certificate fingerprint mismatch")
	}
}
