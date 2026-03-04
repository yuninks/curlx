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
	response *http.Response
	request  *http.Request
	body     []byte
	err      error
}

func (l *Response) Close() error {
	if l.response.Body != nil {
		return l.response.Body.Close()
	}
	return nil
}

// GetRequest get request object
func (r *Response) GetRequest() *http.Request {
	return r.request
}

func (r *Response) GetResponse() *http.Response {
	return r.response
}

// GetBody parse response body
func (r *Response) GetBody() ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.body != nil {
		return r.body, nil
	}
	if r.response == nil {
		return nil, nil
	}
	body := []byte{}
	var err error
	if r.response.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(r.response.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		body, err = io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
	} else {
		body, err = io.ReadAll(r.response.Body)
		if err != nil {
			return nil, err
		}
	}
	// close body
	r.response.Body.Close()

	r.body = body
	return body, err
}

func (r Response) GetStatusCode() int {
	if r.response == nil {
		return 0
	}
	return r.response.StatusCode
}

// IsTimeout get if request is timeout
func (r *Response) IsTimeout() bool {
	if r.err == nil {
		return false
	}
	netErr, ok := r.err.(net.Error)
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
	if r.response == nil {
		return nil
	}
	return r.response.Header
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
