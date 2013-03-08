package jujutest

import (
	"bytes"
	"net/http"
)

// VirtualRoundTripper can be used to provide "http" responses without actually
// starting an HTTP server. It is used by calling:
// vfs := NewVirtualRoundTripper([]FileContent{<file contents>})
// http.DefaultTransport.(*http.Transport).RegisterProtocol("file", vfs)
// At which point requests to file:///foo will pull out the virtual content of
// the file named 'foo' passed into the RoundTripper constructor.
type VirtualRoundTripper struct {
	contents []FileContent
}

var _ http.RoundTripper = (*VirtualRoundTripper)(nil)

// When using RegisterProtocol on http.Transport, you can't actually change the
// registration. So we provide a RoundTripper that simply proxies to whatever
// we want as the current content.
type ProxyRoundTripper struct {
	Sub http.RoundTripper
}

var _ http.RoundTripper = (*ProxyRoundTripper)(nil)

func (prt *ProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if prt.Sub == nil {
		panic("An attempt was made to request file content without having" +
			" the virtual filesystem initialized.")
	}
	return prt.Sub.RoundTrip(req)
}

// A simple content structure to pass data into VirtualRoundTripper. When using
// VRT, requests that match 'Name' will be served the value in 'Content'
type FileContent struct {
	Name    string
	Content string
}

// bytes.Buffer doesn't provide a Close method, but http.Response needs a
// ReadCloser, so we implement a no-op Close method.
type BufferCloser struct {
	*bytes.Buffer
}

func (bc BufferCloser) Close() error { return nil }

// Map the Path into Content based on FileContent.Name
func (v *VirtualRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	res := &http.Response{Proto: "HTTP/1.0",
		ProtoMajor: 1,
		Header:     make(http.Header),
		Close:      true,
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
