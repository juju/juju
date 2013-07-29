// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

// CannedRoundTripper can be used to provide canned "http" responses without
// actually starting an HTTP server.
//
// Use this in conjunction with ProxyRoundTripper.  A ProxyRoundTripper is
// what gets registered as the default handler for a given protocol (such as
// "test") and then tests can direct the ProxyRoundTripper to delegate to a
// CannedRoundTripper.  The reason for this is that we can register a
// roundtripper to handle a scheme, but there is no way to unregister it: you
// may need to re-use the same ProxyRoundTripper but use different
// CannedRoundTrippers to return different results.
type CannedRoundTripper struct {
	// contents are file names and their contents.  If the roundtripper
	// receives a request for any of these files, and none of the entries
	// in errorURLs below matches, it will return the contents associated
	// with that filename here.
	// TODO: Do something more sensible here: either make files take
	// precedence over errors, or return the given error *with* the given
	// contents, or just disallow overlap.
	// TODO: Any reason why this isn't a map!?
	contents []FileContent

	// errorURLs are prefixes that should return specific HTTP status
	// codes.  If a requested file's path matches any of these prefixes,
	// the associated error is returned.
	// There is no clever longest-prefix selection here.  If more than
	// one prefix matches, any one of them may be used.
	// TODO: Decide what to do about multiple matching prefixes.
	errorURLS map[string]int
}

var _ http.RoundTripper = (*CannedRoundTripper)(nil)

// ProxyRoundTripper is a RoundTripper implementation that does nothing but
// delegate to another RoundTripper.  This lets tests change how they handle
// requests for a given scheme, despite the fact that the standard library
// does not support un-registration, or registration of a new roundtripper
// with a URL scheme that's already handled.
//
// Use the RegisterForScheme method to install this as the standard handler
// for a particular protocol.  For example, if you call
// prt.RegisterForScheme("test") then afterwards, any request to "test:///foo"
// will be routed to prt.
type ProxyRoundTripper struct {
	// Sub is the roundtripper that this roundtripper delegates to, if any.
	// If you leave this nil, this roundtripper is effectively disabled.
	Sub http.RoundTripper
}

var _ http.RoundTripper = (*ProxyRoundTripper)(nil)

// RegisterForScheme registers a ProxyRoundTripper as the default roundtripper
// for the given URL scheme.
//
// This cannot be undone, nor overwritten with a different roundtripper.  If
// you change your mind later about what the roundtripper should do, set its
// "Sub" field to delegate to a different roundtripper (or to nil if you don't
// want to handle its requests at all any more).
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

// A simple content structure to pass data into CannedRoundTripper. When using
// VRT, requests that match 'Name' will be served the value in 'Content'
type FileContent struct {
	Name    string
	Content string
}

// Map the Path into Content based on FileContent.Name
func (v *CannedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
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

func NewCannedRoundTripper(contents []FileContent, errorURLs map[string]int) *CannedRoundTripper {
	return &CannedRoundTripper{contents, errorURLs}
}
