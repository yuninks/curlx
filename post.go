package curlx

import (
	"io"
	"net/http"
)

func Post(url string, contentType string, body io.Reader) (resp *http.Response, err error) {
	return http.Post(url, contentType, body)
}
