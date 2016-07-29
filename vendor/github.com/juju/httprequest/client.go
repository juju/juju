// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httprequest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"gopkg.in/errgo.v1"
)

// Doer is implemented by HTTP client packages
// to make an HTTP request. It is notably implemented
// by http.Client and httpbakery.Client.
//
// When httprequest uses a Doer value for requests
// with a non-empty body, it will use DoWithBody if
// the value implements it (see DoerWithBody).
// This enables httpbakery.Client to be used correctly.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DoerWithBody is implemented by HTTP clients that need
// to be able to retry HTTP requests with a body.
// It is notably implemented by httpbakery.Client.
type DoerWithBody interface {
	DoWithBody(req *http.Request, body io.ReadSeeker) (*http.Response, error)
}

// Client represents a client that can invoke httprequest endpoints.
type Client struct {
	// BaseURL holds the base URL to use when making
	// HTTP requests.
	BaseURL string

	// Doer holds a value that will be used to actually
	// make the HTTP request. If it is nil, http.DefaultClient
	// will be used instead. If the request has a non-empty body
	// and Doer implements DoerWithBody, DoWithBody
	// will be used instead.
	Doer Doer

	// If a request returns an HTTP response that signifies an
	// error, UnmarshalError is used to unmarshal the response into
	// an appropriate error. See ErrorUnmarshaler for a convenient
	// way to create an UnmarshalError function for a given type. If
	// this is nil, DefaultErrorUnmarshaler will be used.
	UnmarshalError func(resp *http.Response) error
}

// DefaultErrorUnmarshaler is the default error unmarshaler
// used by Client.
var DefaultErrorUnmarshaler = ErrorUnmarshaler(new(RemoteError))

// Call invokes the endpoint implied by the given params,
// which should be of the form accepted by the ArgT
// argument to a function passed to Handle, and
// unmarshals the response into the given response parameter,
// which should be a pointer to the response value.
//
// If params implements the HeaderSetter interface, its SetHeader method
// will be called to add additional headers to the HTTP request.
//
// If resp is nil, the response will be ignored if the
// request was successful.
//
// If resp is of type **http.Response, instead of unmarshaling
// into it, its element will be set to the returned HTTP
// response directly and the caller is responsible for
// closing its Body field.
//
// Any error that c.UnmarshalError or c.Doer returns will not
// have its cause masked.
func (c *Client) Call(params, resp interface{}) error {
	return c.CallURL(c.BaseURL, params, resp)
}

// CallURL is like Call except that the given URL is used instead of
// c.BaseURL.
func (c *Client) CallURL(url string, params, resp interface{}) error {
	rt, err := getRequestType(reflect.TypeOf(params))
	if err != nil {
		return errgo.Mask(err)
	}
	if rt.method == "" {
		return errgo.Newf("type %T has no httprequest.Route field", params)
	}
	reqURL, err := appendURL(url, rt.path)
	if err != nil {
		return errgo.Mask(err)
	}
	req, err := Marshal(reqURL.String(), rt.method, params)
	if err != nil {
		return errgo.Mask(err)
	}

	// Actually make the request.
	doer := c.Doer
	if doer == nil {
		doer = http.DefaultClient
	}
	var httpResp *http.Response
	body := req.Body.(BytesReaderCloser)
	// Always use DoWithBody when available.
	if doer1, ok := doer.(DoerWithBody); ok {
		req.Body = nil
		httpResp, err = doer1.DoWithBody(req, body)
	} else {
		httpResp, err = doer.Do(req)
	}
	if err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return c.unmarshalResponse(httpResp, resp)
}

// Do sends the given request and unmarshals its JSON
// result into resp, which should be a pointer to the response value.
// If an error status is returned, the error will be unmarshaled
// as in Client.Call. The req.Body field must be nil - any request
// body should be provided in the body parameter.
//
// If resp is nil, the response will be ignored if the response was
// successful.
//
// If resp is of type **http.Response, instead of unmarshaling
// into it, its element will be set to the returned HTTP
// response directly and the caller is responsible for
// closing its Body field.
//
// Any error that c.UnmarshalError or c.Doer returns will not
// have its cause masked.
//
// If req.URL does not have a host part it will be treated as relative to
// c.BaseURL. req.URL will be updated to the actual URL used.
func (c *Client) Do(req *http.Request, body io.ReadSeeker, resp interface{}) error {
	if req.URL.Host == "" {
		var err error
		req.URL, err = appendURL(c.BaseURL, req.URL.String())
		if err != nil {
			return errgo.Mask(err)
		}
	}
	if req.Body != nil {
		return errgo.Newf("%s %s: request body supplied unexpectedly", req.Method, req.URL)
	}
	inferContentLength(req, body)
	doer := c.Doer
	if doer == nil {
		doer = http.DefaultClient
	}
	var httpResp *http.Response
	var err error
	// Use DoWithBody when it's available and body is not nil.
	doer1, ok := doer.(DoerWithBody)
	if ok && body != nil {
		httpResp, err = doer1.DoWithBody(req, body)
	} else {
		if body != nil {
			// Work around Go issue 12796 by using a body
			// that can't be read from after it's been closed,
			// so if the caller of Do calls body.Close immediately after
			// Do returns, the http request logic won't be
			// able to call body.Close or body.Read
			// because the readStopper will prevent that.
			req.Body = &readStopper{r: body}
			defer req.Body.Close()
		}
		httpResp, err = doer.Do(req)
	}
	if err != nil {
		return errgo.NoteMask(err, fmt.Sprintf("%s %s", req.Method, req.URL), errgo.Any)
	}
	return c.unmarshalResponse(httpResp, resp)
}

// Get is a convenience method that uses c.Do to issue a GET request to
// the given URL. If the given URL does not have a host part then it will
// be treated as relative to c.BaseURL.
func (c *Client) Get(url string, resp interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return errgo.Notef(err, "cannot make request")
	}
	return c.Do(req, nil, resp)
}

func inferContentLength(req *http.Request, body io.ReadSeeker) {
	if body == nil {
		return
	}
	switch v := body.(type) {
	case *bytes.Reader:
		req.ContentLength = int64(v.Len())
	case *strings.Reader:
		req.ContentLength = int64(v.Len())
	}
}

// unmarshalResponse unmarshals
func (c *Client) unmarshalResponse(httpResp *http.Response, resp interface{}) error {
	if 200 <= httpResp.StatusCode && httpResp.StatusCode < 300 {
		if respPt, ok := resp.(**http.Response); ok {
			*respPt = httpResp
			return nil
		}
		defer httpResp.Body.Close()
		if err := UnmarshalJSONResponse(httpResp, resp); err != nil {
			return errgo.Notef(err, "%s %s", httpResp.Request.Method, httpResp.Request.URL)
		}
		return nil
	}
	defer httpResp.Body.Close()
	errUnmarshaler := c.UnmarshalError
	if errUnmarshaler == nil {
		errUnmarshaler = DefaultErrorUnmarshaler
	}
	err := errUnmarshaler(httpResp)
	if err == nil {
		err = errgo.Newf("unexpected HTTP response status: %s", httpResp.Status)
	}
	return errgo.NoteMask(err, httpResp.Request.Method+" "+httpResp.Request.URL.String(), errgo.Any)
}

// ErrorUnmarshaler returns a function which will unmarshal error
// responses into new values of the same type as template. The argument
// must be a pointer. A new instance of it is created every time the
// returned function is called.
func ErrorUnmarshaler(template error) func(*http.Response) error {
	t := reflect.TypeOf(template)
	if t.Kind() != reflect.Ptr {
		panic(errgo.Newf("cannot unmarshal errors into value of type %T", template))
	}
	t = t.Elem()
	return func(resp *http.Response) error {
		if 300 <= resp.StatusCode && resp.StatusCode < 400 {
			// It's a redirection error.
			loc, _ := resp.Location()
			return fmt.Errorf("unexpected redirect (status %s) from %q to %q", resp.Status, resp.Request.URL, loc)
		}
		errv := reflect.New(t)
		if err := UnmarshalJSONResponse(resp, errv.Interface()); err != nil {
			return fmt.Errorf("cannot unmarshal error response (status %s): %v", resp.Status, err)
		}
		return errv.Interface().(error)
	}
}

// UnmarshalJSONResponse unmarshals the given HTTP response
// into x, which should be a pointer to the result to be
// unmarshaled into.
func UnmarshalJSONResponse(resp *http.Response, x interface{}) error {
	// Try to read all the body so that we can reuse the
	// connection, but don't try *too* hard.
	defer io.Copy(ioutil.Discard, io.LimitReader(resp.Body, 8*1024))
	if x == nil {
		return nil
	}
	if err := checkIsJSON(resp.Header, resp.Body); err != nil {
		return errgo.Mask(err)
	}
	// Decode only a single JSON value, and then
	// discard the rest of the body so that we can
	// reuse the connection even if some foolish server
	// has put garbage on the end.
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(x); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// RemoteError holds the default type of a remote error
// used by Client when no custom error unmarshaler
// is set.
type RemoteError struct {
	// Message holds the error message.
	Message string

	// Code may hold a code that classifies the error.
	Code string `json:",omitempty"`

	// Info holds any other information associated with the error.
	Info *json.RawMessage `json:",omitempty"`
}

// Error implements the error interface.
func (e *RemoteError) Error() string {
	if e.Message == "" {
		return "httprequest: no error message found"
	}
	return "httprequest: " + e.Message
}

// appendURL returns the result of combining the
// given base URL and relative URL.
//
// The path of the relative URL will be appended
// to the base URL, separated by a slash (/) if
// needed.
//
// Any query parameters will be concatenated together.
//
// appendURL will return an error if relURLStr contains
// a host name.
func appendURL(baseURLStr, relURLStr string) (*url.URL, error) {
	b, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse %q", baseURLStr)
	}
	r, err := url.Parse(relURLStr)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse %q", relURLStr)
	}
	if r.Host != "" {
		return nil, errgo.Newf("relative URL specifies a host")
	}
	if r.Path != "" {
		b.Path = strings.TrimSuffix(b.Path, "/") + "/" + strings.TrimPrefix(r.Path, "/")
	}
	if r.RawQuery != "" {
		if b.RawQuery != "" {
			b.RawQuery += "&" + r.RawQuery
		} else {
			b.RawQuery = r.RawQuery
		}
	}
	return b, nil
}

// readStopper implements io.ReadCloser by preventing
// all reads after Close has been called.
// This is necessary to work around http://golang.org/issue/12796
//
// TODO export this, as it may be useful for other
// clients of net/http too?
type readStopper struct {
	mu sync.Mutex
	r  io.Reader
}

func (r *readStopper) Read(buf []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.r == nil {
		// Note: we have to use io.EOF here because otherwise
		// another connection can (in rare circumstances) be
		// polluted by the error returned here. Although this
		// means the file may appear truncated to the server,
		// that shouldn't matter because the body will only
		// be closed after the server has replied.
		return 0, io.EOF
	}
	return r.r.Read(buf)
}

func (r *readStopper) Close() error {
	r.mu.Lock()
	r.r = nil
	r.mu.Unlock()
	return nil
}
