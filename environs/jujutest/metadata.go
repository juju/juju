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
// Use this in conjunction with ProxyRoundTripper. A ProxyRoundTripper is
// what gets registered as the default handler for a given protocol (such as
// "test") and then tests can direct the ProxyRoundTripper to delegate to a
// CannedRoundTripper. The reason for this is that we can register a
// roundtripper to handle a scheme, but there is no way to unregister it: you
// may need to re-use the same ProxyRoundTripper but use different
// CannedRoundTrippers to return different results.
type CannedRoundTripper struct {
	// files maps file names to their contents. If the roundtripper
	// receives a request for any of these files, and none of the entries
	// in errorURLs below matches, it will return the contents associated
	// with that filename here.
	// TODO(jtv): Do something more sensible here: either make files take
	// precedence over errors, or return the given error *with* the given
	// contents, or just disallow overlap.
	files map[string]string

	// errorURLs are prefixes that should return specific HTTP status
	// codes. If a request's URL matches any of these prefixes, the
	// associated error status is returned.
	// There is no clever longest-prefix selection here. If more than
	// one prefix matches, any one of them may be used.
	// TODO(jtv): Decide what to do about multiple matching prefixes.
	errorURLS map[string]int
}

var _ http.RoundTripper = (*CannedRoundTripper)(nil)

// ProxyRoundTripper is an http.RoundTripper implementation that does nothing
// but delegate to another RoundTripper. This lets tests change how they handle
// requests for a given scheme, despite the fact that the standard library does
// not support un-registration, or registration of a new roundtripper with a
// URL scheme that's already handled.
//
// Use the RegisterForScheme method to install this as the standard handler
// for a particular protocol. For example, if you call
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
// This cannot be undone, nor overwritten with a different roundtripper. If
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

func newHTTPResponse(status string, statusCode int, body string) *http.Response {
	return &http.Response{
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		Header:     make(http.Header),
		Close:      true,

		// Parameter fields:
		Status:        status,
		StatusCode:    statusCode,
		Body:          ioutil.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

// RoundTrip returns a canned error or body for the given request.
func (v *CannedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	full := req.URL.String()
	for urlPrefix, statusCode := range v.errorURLS {
		if strings.HasPrefix(full, urlPrefix) {
			status := fmt.Sprintf("%d Error", statusCode)
			return newHTTPResponse(status, statusCode, ""), nil
		}
	}
	if contents, found := v.files[req.URL.Path]; found {
		return newHTTPResponse("200 OK", http.StatusOK, contents), nil
	}
	return newHTTPResponse("404 Not Found", http.StatusNotFound, ""), nil
}

// NewCannedRoundTripper returns a CannedRoundTripper with the given canned
// responses.
func NewCannedRoundTripper(files map[string]string, errorURLs map[string]int) *CannedRoundTripper {
	return &CannedRoundTripper{files, errorURLs}
}
