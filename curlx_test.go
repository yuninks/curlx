package curlx

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	resp, code, err := NewCurlx().Send(context.Background(),
		SetParamsUrl("https://www.baidu.com"),
		SetParamsMethod(MethodGet),
	)
	t.Log(resp, code, err)

}

func TestForm(t *testing.T) {

	file, err := os.Open("./go.mod")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	b, _ := io.ReadAll(file)

	s := []FormParam{
		{
			FieldName: "file",
			FileName:  file.Name(),
			FieldType: "file",
			FileBytes: b,
		},
	}

	p := ClientParams{
		Url:    "http://tech-dev.sealmoo.com/api/material/upload",
		Method: "POST",
		Body:   s,
		Headers: map[string]interface{}{
			"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0ZW5hbnRfaWQiOjAsImNsaWVudF9pZCI6MCwidXNlcl9pZCI6MSwiZXhwIjoxNzAxMzk3NzkxfQ.9_uJ6y8I4JZTwgSenwHC_01nddLuI4zmgpyPhn5M6j8",
		},
		ContentType: ContentTypeForm,
	}
	resp, code, err := NewCurlx().Send(context.Background(), SetParamsAll(p))
	fmt.Println(resp, code, err)
}

func TestProxy(t *testing.T) {
	c := NewCurlx()
	c.WithProxySocks5("127.0.0.1:1080")
	res, code, err := c.Send(context.Background(), SetParamsUrl("https://www.google.com"),SetParamsMethod(MethodGet))
	t.Log(string(res), code, err)
}
