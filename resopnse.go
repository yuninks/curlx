package curlx

import (
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// Response response object
type Response struct {
	Response *http.Response
	Request  *http.Request
	Body     []byte
	Err      error
}

func (l *Response) Close() error {
	if l.Response.Body != nil {
		return l.Response.Body.Close()
	}
	return nil
}

// GetRequest get request object
func (r *Response) GetRequest() *http.Request {
	return r.Request
}

func (r *Response) GetResponse() *http.Response {
	return r.Response
}

// GetBody parse response body
func (r *Response) GetBody() ([]byte, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	if r.Body != nil {
		return r.Body, nil
	}
	if r.Response == nil {
		return nil, nil
	}
	body := []byte{}
	var err error
	if r.Response.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(r.Response.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		body, err = io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
	} else {
		body, err = io.ReadAll(r.Response.Body)
		if err != nil {
			return nil, err
		}
	}
	// close body
	r.Response.Body.Close()

	r.Body = body
	return body, err
}

func (r Response) GetStatusCode() int {
	if r.Response == nil {
		return 0
	}
	return r.Response.StatusCode
}

// IsTimeout get if request is timeout
func (r *Response) IsTimeout() bool {
	if r.Err == nil {
		return false
	}
	netErr, ok := r.Err.(net.Error)
	if !ok {
		return false
	}
	if netErr.Timeout() {
		return true
	}

	return false
}

// GetParsedBody parse response body with gjson
func (r *Response) GetParsedBody() (*gjson.Result, error) {
	body, err := r.GetBody()
	if err != nil {
		return nil, err
	}
	pb := gjson.ParseBytes(body)

	return &pb, nil
}

// GetHeaders get response headers
func (r *Response) GetHeaders() map[string][]string {
	return r.Response.Header
}

// GetHeader get response header
func (r *Response) GetHeader(name string) []string {
	headers := r.GetHeaders()
	for k, v := range headers {
		if strings.EqualFold(name, k) {
			return v
		}
	}

	return nil
}

// GetHeaderLine get a single response header
func (r *Response) GetHeaderLine(name string) string {
	header := r.GetHeader(name)
	if len(header) > 0 {
		return header[0]
	}

	return ""
}

// HasHeader get if header exsits in response headers
func (r *Response) HasHeader(name string) bool {
	headers := r.GetHeaders()
	for k := range headers {
		if strings.EqualFold(name, k) {
			return true
		}
	}

	return false
}
