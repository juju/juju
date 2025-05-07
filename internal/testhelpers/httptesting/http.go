// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httptesting

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/juju/tc"
)

// BodyAsserter represents a function that can assert the correctness of
// a JSON reponse.
type BodyAsserter func(c *tc.C, body json.RawMessage)

// JSONCallParams holds parameters for AssertJSONCall.
// If left empty, some fields will automatically be filled with defaults.
type JSONCallParams struct {
	// Do is used to make the HTTP request.
	// If it is nil, http.DefaultClient.Do will be used.
	// If the body reader implements io.Seeker,
	// req.Body will also implement that interface.
	Do func(req *http.Request) (*http.Response, error)

	// ExpectError holds the error regexp to match
	// against the error returned from the HTTP Do
	// request. If it is empty, the error is expected to be
	// nil.
	ExpectError string

	// Method holds the HTTP method to use for the call.
	// GET is assumed if this is empty.
	Method string

	// URL holds the URL to pass when making the request.
	// If the URL does not contain a host, a temporary
	// HTTP server is started running the Handler below
	// which is used for the host.
	URL string

	// Handler holds the handler to use to make the request.
	// It is ignored if the above URL field has a host part.
	Handler http.Handler

	// JSONBody specifies a JSON value to marshal to use
	// as the body of the request. If this is specified, Body will
	// be ignored and the Content-Type header will
	// be set to application/json. The request
	// body will implement io.Seeker.
	JSONBody interface{}

	// Body holds the body to send in the request.
	Body io.Reader

	// Header specifies the HTTP headers to use when making
	// the request.
	Header http.Header

	// ContentLength specifies the length of the body.
	// It may be zero, in which case the default net/http
	// content-length behaviour will be used.
	ContentLength int64

	// Username, if specified, is used for HTTP basic authentication.
	Username string

	// Password, if specified, is used for HTTP basic authentication.
	Password string

	// ExpectStatus holds the expected HTTP status code.
	// http.StatusOK is assumed if this is zero.
	ExpectStatus int

	// ExpectBody holds the expected JSON body.
	// This may be a function of type BodyAsserter in which case it
	// will be called with the http response body to check the
	// result.
	ExpectBody interface{}

	// ExpectHeader holds any HTTP headers that must be present in the response.
	// Note that the response may also contain headers not in this field.
	ExpectHeader http.Header

	// Cookies, if specified, are added to the request.
	Cookies []*http.Cookie
}

// AssertJSONCall asserts that when the given handler is called with
// the given parameters, the result is as specified.
func AssertJSONCall(c *tc.C, p JSONCallParams) {
	c.Logf("JSON call, url %q", p.URL)
	if p.ExpectStatus == 0 {
		p.ExpectStatus = http.StatusOK
	}
	rec := DoRequest(c, DoRequestParams{
		Do:            p.Do,
		ExpectError:   p.ExpectError,
		Handler:       p.Handler,
		Method:        p.Method,
		URL:           p.URL,
		Body:          p.Body,
		JSONBody:      p.JSONBody,
		Header:        p.Header,
		ContentLength: p.ContentLength,
		Username:      p.Username,
		Password:      p.Password,
		Cookies:       p.Cookies,
	})
	if p.ExpectError != "" {
		return
	}
	AssertJSONResponse(c, rec, p.ExpectStatus, p.ExpectBody)

	for k, v := range p.ExpectHeader {
		c.Assert(rec.HeaderMap[textproto.CanonicalMIMEHeaderKey(k)], tc.DeepEquals, v, tc.Commentf("header %q", k))
	}
}

// AssertJSONResponse asserts that the given response recorder has
// recorded the given HTTP status, response body and content type. If
// expectBody is of type BodyAsserter it will be called with the response
// body to ensure the response is correct.
func AssertJSONResponse(c *tc.C, rec *httptest.ResponseRecorder, expectStatus int, expectBody interface{}) {
	c.Assert(rec.Code, tc.Equals, expectStatus, tc.Commentf("body: %s", rec.Body.Bytes()))

	// Ensure the response includes the expected body.
	if expectBody == nil {
		c.Assert(rec.Body.Bytes(), tc.HasLen, 0)
		return
	}
	c.Assert(rec.Header().Get("Content-Type"), tc.Equals, "application/json")

	if assertBody, ok := expectBody.(BodyAsserter); ok {
		var data json.RawMessage
		err := json.Unmarshal(rec.Body.Bytes(), &data)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("body: %s", rec.Body.Bytes()))
		assertBody(c, data)
		return
	}
	c.Assert(rec.Body.String(), tc.JSONEquals, expectBody)
}

// DoRequestParams holds parameters for DoRequest.
// If left empty, some fields will automatically be filled with defaults.
type DoRequestParams struct {
	// Do is used to make the HTTP request.
	// If it is nil, http.DefaultClient.Do will be used.
	// If the body reader implements io.Seeker,
	// req.Body will also implement that interface.
	Do func(req *http.Request) (*http.Response, error)

	// ExpectError holds the error regexp to match
	// against the error returned from the HTTP Do
	// request. If it is empty, the error is expected to be
	// nil.
	ExpectError string

	// ExpectStatus holds the expected HTTP status code.
	// If unset or zero, then no check is performed.
	ExpectStatus int

	// Method holds the HTTP method to use for the call.
	// GET is assumed if this is empty.
	Method string

	// URL holds the URL to pass when making the request.
	// If the URL does not contain a host, a temporary
	// HTTP server is started running the Handler below
	// which is used for the host.
	URL string

	// Handler holds the handler to use to make the request.
	// It is ignored if the above URL field has a host part.
	Handler http.Handler

	// JSONBody specifies a JSON value to marshal to use
	// as the body of the request. If this is specified, Body will
	// be ignored and the Content-Type header will
	// be set to application/json. The request
	// body will implement io.Seeker.
	JSONBody interface{}

	// Body holds the body to send in the request.
	Body io.Reader

	// Header specifies the HTTP headers to use when making
	// the request.
	Header http.Header

	// ContentLength specifies the length of the body.
	// It may be zero, in which case the default net/http
	// content-length behaviour will be used.
	ContentLength int64

	// Username, if specified, is used for HTTP basic authentication.
	Username string

	// Password, if specified, is used for HTTP basic authentication.
	Password string

	// Cookies, if specified, are added to the request.
	Cookies []*http.Cookie
}

// DoRequest is the same as Do except that it returns
// an httptest.ResponseRecorder instead of an http.Response.
// This function exists for backward compatibility reasons.
func DoRequest(c *tc.C, p DoRequestParams) *httptest.ResponseRecorder {
	resp := Do(c, p)
	if p.ExpectError != "" {
		return nil
	}
	defer resp.Body.Close()
	rec := httptest.NewRecorder()
	h := rec.Header()
	for k, v := range resp.Header {
		h[k] = v
	}
	rec.WriteHeader(resp.StatusCode)
	_, err := io.Copy(rec.Body, resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	return rec
}

// Do invokes a request on the given handler with the given
// parameters and returns the resulting HTTP response.
// Note that, as with http.Client.Do, the response body
// must be closed.
func Do(c *tc.C, p DoRequestParams) *http.Response {
	if p.Method == "" {
		p.Method = "GET"
	}
	if p.Do == nil {
		p.Do = http.DefaultClient.Do
	}
	if reqURL, err := url.Parse(p.URL); err == nil && reqURL.Host == "" {
		srv := httptest.NewServer(p.Handler)
		defer srv.Close()
		p.URL = srv.URL + p.URL
	}
	if p.JSONBody != nil {
		data, err := json.Marshal(p.JSONBody)
		c.Assert(err, tc.ErrorIsNil)
		p.Body = bytes.NewReader(data)
	}
	// Note: we avoid NewRequest's odious reader wrapping by using
	// a custom nopCloser function.
	req, err := http.NewRequest(p.Method, p.URL, nopCloser(p.Body))
	c.Assert(err, tc.ErrorIsNil)
	if p.JSONBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, val := range p.Header {
		req.Header[key] = val
	}
	if p.ContentLength != 0 {
		req.ContentLength = p.ContentLength
	} else {
		req.ContentLength = bodyContentLength(p.Body)
	}
	if p.Username != "" || p.Password != "" {
		req.SetBasicAuth(p.Username, p.Password)
	}
	for _, cookie := range p.Cookies {
		req.AddCookie(cookie)
	}
	resp, err := p.Do(req)
	if p.ExpectError != "" {
		c.Assert(err, tc.ErrorMatches, p.ExpectError)
		return nil
	}
	// malformed error check here is required to ensure that we handle cases
	// where prior to go version 1.12 if you try and access HTTPS from a HTTP
	// end point you recieved garbage back. In go version 1.12 and higher, the
	// status code of 400 is returned. The issue with this is that we should
	// handle both go version <1.11 and go >=1.12 in the same way. Juju
	// shouldn't have to know about the idiosyncrasies of the go runtime.
	malformed := malformedError(err)
	if err != nil && !malformed {
		c.Assert(err, tc.ErrorIsNil)
	}
	if p.ExpectStatus != 0 {
		statusCode := http.StatusBadRequest
		if !malformed {
			statusCode = resp.StatusCode
		}
		c.Assert(statusCode, tc.Equals, p.ExpectStatus)
	}
	return resp
}

func malformedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "transport connection broken: malformed HTTP response")
}

// bodyContentLength returns the Content-Length
// to use for the given body. Usually http.NewRequest
// would infer this (and the cases here come directly
// from the logic in that function) but unfortunately
// there's no way to avoid the NopCloser wrapping
// for any of the types mentioned here.
func bodyContentLength(body io.Reader) int64 {
	n := 0
	switch v := body.(type) {
	case *bytes.Buffer:
		n = v.Len()
	case *bytes.Reader:
		n = v.Len()
	case *strings.Reader:
		n = v.Len()
	}
	return int64(n)
}

// nopCloser is like ioutil.NopCloser except that
// the returned value implements io.Seeker if
// r implements io.Seeker
func nopCloser(r io.Reader) io.ReadCloser {
	if r == nil {
		return nil
	}
	rc, ok := r.(io.ReadCloser)
	if ok {
		return rc
	}
	rs, ok := r.(io.ReadSeeker)
	if ok {
		return readSeekNopCloser{rs}
	}
	return ioutil.NopCloser(r)
}

type readSeekNopCloser struct {
	io.ReadSeeker
}

func (readSeekNopCloser) Close() error {
	return nil
}

// URLRewritingTransport is an http.RoundTripper that can rewrite request
// URLs. If the request URL has the prefix specified in Match that part
// will be changed to the value specified in Replace. RoundTripper will
// then be used to perform the resulting request. If RoundTripper is nil
// http.DefaultTransport will be used.
//
// This can be used in tests that, for whatever reason, need to make a
// call to a URL that's not in our control but we want to control the
// results of HTTP requests to that URL.
type URLRewritingTransport struct {
	MatchPrefix  string
	Replace      string
	RoundTripper http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t URLRewritingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.RoundTripper
	if rt == nil {
		rt = http.DefaultTransport
	}
	if !strings.HasPrefix(req.URL.String(), t.MatchPrefix) {
		return rt.RoundTrip(req)
	}
	req1 := *req
	var err error
	req1.URL, err = url.Parse(t.Replace + strings.TrimPrefix(req.URL.String(), t.MatchPrefix))
	if err != nil {
		panic(err)
	}
	resp, err := rt.RoundTrip(&req1)
	if resp != nil {
		resp.Request = req
	}
	return resp, err
}
