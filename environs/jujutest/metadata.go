package jujutest

import (
	"bytes"
	"net/http"
)

type FileContent struct {
	Name    string
	Content string
}

type BufferCloser struct {
	*bytes.Buffer
}

func (bc BufferCloser) Close() error { return nil }

type VirtualRoundTripper struct {
	contents []FileContent
}

var _ http.RoundTripper = (*VirtualRoundTripper)(nil)

func (v *VirtualRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	res := &http.Response{Proto: "HTTP/1.0",
		ProtoMajor: 1,
		Header: make(http.Header),
		Close: true,
	}
	for _, fc := range v.contents {
		if fc.Name == req.URL.Path {
			res.Status = "200 OK"
			res.StatusCode = http.StatusOK
			res.ContentLength = int64(len(fc.Content))
			res.Body = BufferCloser{bytes.NewBufferString(fc.Content)}
			return res, nil
		}
	}
	res.Status = "404 Not Found"
	res.StatusCode = http.StatusNotFound
	res.ContentLength = 0
	res.Body = BufferCloser{bytes.NewBufferString("")}
	return res, nil
}

func NewVirtualRoundTripper(contents []FileContent) *VirtualRoundTripper {
	return &VirtualRoundTripper{contents}
}
