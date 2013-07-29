// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

// VirtualRoundTripper can be used to provide "http" responses without actually
// starting an HTTP server. It is used by calling:
// vfs := NewVirtualRoundTripper([]FileContent{<file contents>, <error urls>})
// http.DefaultTransport.(*http.Transport).RegisterProtocol("test", vfs)
// At which point requests to test:///foo will pull out the virtual content of
// the file named 'foo' passed into the RoundTripper constructor.
// If errorURLs are supplied, any URL which starts with the one of the map keys
// causes a response with the corresponding status code to be returned.
type VirtualRoundTripper struct {
	contents  []FileContent
	errorURLS map[string]int
}

var _ http.RoundTripper = (*VirtualRoundTripper)(nil)

// When using RegisterProtocol on http.Transport, you can't actually change the
// registration. So we provide a RoundTripper that simply proxies to whatever
// roundtripper we want for the current test.
type ProxyRoundTripper struct {
	Sub http.RoundTripper
}

var _ http.RoundTripper = (*ProxyRoundTripper)(nil)

// RegisterForScheme registers a ProxyRoundTripper as the default roundtripper
// for the given URL scheme.
//
// This cannot be undone, nor overwritten with a different roundtripper.
func (prt *ProxyRoundTripper) RegisterForScheme(scheme string) {
	http.DefaultTransport.(*http.Transport).RegisterProtocol(scheme, prt)
}

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

// Map the Path into Content based on FileContent.Name
func (v *VirtualRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	res := &http.Response{Proto: "HTTP/1.0",
		ProtoMajor: 1,
		Header:     make(http.Header),
		Close:      true,
	}
	full := req.URL.String()
	for urlPrefix, statusCode := range v.errorURLS {
		if strings.HasPrefix(full, urlPrefix) {
			res.Status = fmt.Sprintf("%d Error", statusCode)
			res.StatusCode = statusCode
			res.ContentLength = 0
			res.Body = ioutil.NopCloser(strings.NewReader(""))
			return res, nil
		}
	}
	for _, fc := range v.contents {
		if fc.Name == req.URL.Path {
			res.Status = "200 OK"
			res.StatusCode = http.StatusOK
			res.ContentLength = int64(len(fc.Content))
			res.Body = ioutil.NopCloser(strings.NewReader(fc.Content))
			return res, nil
		}
	}
	res.Status = "404 Not Found"
	res.StatusCode = http.StatusNotFound
	res.ContentLength = 0
	res.Body = ioutil.NopCloser(strings.NewReader(""))
	return res, nil
}

func NewVirtualRoundTripper(contents []FileContent, errorURLs map[string]int) *VirtualRoundTripper {
	return &VirtualRoundTripper{contents, errorURLs}
}
