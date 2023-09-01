package curlx_test

import (
	"context"
	"testing"

	"code.yun.ink/open/curlx"
)

func TestCurlx(t *testing.T) {
	p := curlx.CurlParams{}
	p.Url = "http://www.baidu.com"
	p.Method = "GET"

	resp,code,err := curlx.NewCurlx().Send(context.Background(), &p)
	t.Log(resp,code,err)

}
