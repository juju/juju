// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.provider.azure.internal.azuretesting")

// FakeCredential is a credential that returns a fake token.
type FakeCredential struct{}

func (c *FakeCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "FakeToken"}, nil
}

// Body implements acceptable body over a string.
type Body struct {
	src    []byte
	buf    []byte
	isOpen bool
}

// NewBody creates a new instance of Body.
func NewBody(s string) *Body {
	return (&Body{src: []byte(s)}).reset()
}

// Read reads into the passed byte slice and returns the bytes read.
func (body *Body) Read(b []byte) (n int, err error) {
	if !body.IsOpen() {
		return 0, fmt.Errorf("ERROR: Body has been closed")
	}
	if len(body.buf) == 0 {
		return 0, io.EOF
	}
	n = copy(b, body.buf)
	body.buf = body.buf[n:]
	return n, nil
}

// Close closes the body.
func (body *Body) Close() error {
	if body.isOpen {
		body.isOpen = false
	}
	return nil
}

// IsOpen returns true if the Body has not been closed, false otherwise.
func (body *Body) IsOpen() bool {
	return body.isOpen
}

func (body *Body) reset() *Body {
	body.isOpen = true
	body.buf = body.src
	return body
}

// Length returns the number of bytes in the body.
func (body *Body) Length() int64 {
	if body == nil {
		return 0
	}
	return int64(len(body.src))
}

// NewRequest instantiates a new request.
func NewRequest() *http.Request {
	return NewRequestWithContent("")
}

// NewRequestWithContent instantiates a new request using the passed string for the body content.
func NewRequestWithContent(c string) *http.Request {
	r, _ := http.NewRequest("GET", "https://microsoft.com/a/b/c/", NewBody(c))
	return r
}

// NewResponse instantiates a new response.
func NewResponse() *http.Response {
	return NewResponseWithContent("")
}

// NewResponseWithContent instantiates a new response with the passed string as the body content.
func NewResponseWithContent(c string) *http.Response {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Body:       NewBody(c),
		Request:    NewRequest(),
	}
}

// NewResponseWithStatus instantiates a new response using the passed string and integer as the
// status and status code.
func NewResponseWithStatus(s string, c int) *http.Response {
	resp := NewResponse()
	resp.Status = s
	resp.StatusCode = c
	return resp
}

// NewResponseWithBodyAndStatus instantiates a new response using the specified mock body,
// status and status code
func NewResponseWithBodyAndStatus(body *Body, c int, s string) *http.Response {
	resp := NewResponse()
	resp.Body = body
	resp.ContentLength = body.Length()
	resp.Status = s
	resp.StatusCode = c
	return resp
}

type response struct {
	r *http.Response
	e error
	d time.Duration
}

// MockSender provides a mechanism to test requests and
// provide responses to/from the Azure cloud API.
type MockSender struct {
	attempts       int
	responses      []response
	numResponses   int
	repeatResponse []int
	err            error
	repeatError    int

	// PathPattern, if non-empty, is assumed to be a regular expression
	// that must match the request path.
	PathPattern string
}

// Do implements policy.Policy.
func (s *MockSender) Do(req *http.Request) (resp *http.Response, err error) {
	if s.PathPattern != "" {
		matched, err := regexp.MatchString(s.PathPattern, req.URL.Path)
		if err != nil {
			return nil, err
		}
		if !matched {
			return nil, fmt.Errorf(
				"request path %q did not match pattern %q",
				req.URL.Path, s.PathPattern,
			)
		}
	}
	s.attempts++

	if len(s.responses) > 0 {
		resp = s.responses[0].r
		if resp != nil {
			if b, ok := resp.Body.(*Body); ok {
				b.reset()
			}
		} else {
			err = s.responses[0].e
		}
		select {
		case <-time.After(s.responses[0].d):
			// do nothing
		case <-req.Context().Done():
			err = req.Context().Err()
			return
		}
		s.repeatResponse[0]--
		if s.repeatResponse[0] == 0 {
			s.responses = s.responses[1:]
			s.repeatResponse = s.repeatResponse[1:]
		}
	} else {
		resp = NewResponse()
	}
	if resp != nil {
		resp.Request = req
	}

	if s.err != nil {
		err = s.err
		s.repeatError--
		if s.repeatError == 0 {
			s.err = nil
		}
	}

	return
}

// AppendResponse adds the passed http.Response to the response stack.
func (c *MockSender) AppendResponse(resp *http.Response) {
	c.AppendAndRepeatResponse(resp, 1)
}

// AppendAndRepeatResponse adds the passed http.Response to the response stack along with a
// repeat count. A negative repeat count will return the response for all remaining calls to Do.
func (c *MockSender) AppendAndRepeatResponse(resp *http.Response, repeat int) {
	c.appendAndRepeat(response{r: resp}, repeat)
}

func (c *MockSender) appendAndRepeat(resp response, repeat int) {
	if c.responses == nil {
		c.responses = []response{resp}
		c.repeatResponse = []int{repeat}
	} else {
		c.responses = append(c.responses, resp)
		c.repeatResponse = append(c.repeatResponse, repeat)
	}
	c.numResponses++
}

// Attempts returns the number of times Do was called.
func (c *MockSender) Attempts() int {
	return c.attempts
}

// SetError sets the error Do should return.
func (c *MockSender) SetError(err error) {
	c.SetAndRepeatError(err, 1)
}

// SetAndRepeatError sets the error Do should return and how many calls to Do will return the error.
// A negative repeat value will return the error for all remaining calls to Do.
func (c *MockSender) SetAndRepeatError(err error, repeat int) {
	c.err = err
	c.repeatError = repeat
}

// NumResponses returns the number of responses that have been added to the sender.
func (c *MockSender) NumResponses() int {
	return c.numResponses
}

const fakeTenantId = "11111111-1111-1111-1111-111111111111"

// NewSenderWithValue returns a *mocks.Sender that marshals the provided object
// to JSON and sets it as the content. This function will panic if marshalling
// fails.
func NewSenderWithValue(v interface{}) *MockSender {
	content, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	sender := &MockSender{}
	resp := NewResponseWithContent(string(content))
	SetResponseHeaderValues(resp, "WWW-Authenticate", []string{
		fmt.Sprintf(
			`authorization="https://testing.invalid/%s" scope="scope" resource="resource"`,
			fakeTenantId,
		),
	})
	sender.AppendResponse(resp)
	return sender
}

// SetResponseHeaderValues adds a header containing all the passed string values.
func SetResponseHeaderValues(resp *http.Response, h string, values []string) {
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	for _, v := range values {
		resp.Header.Add(h, v)
	}
}

// Senders is a Sender that includes a collection of Senders, which
// will be called in sequence.
type Senders []policy.Transporter

func (s *Senders) Do(req *http.Request) (*http.Response, error) {
	logger.Debugf(req.Context(), "Senders.Do(%s)", req.URL)
	if len(*s) == 0 {
		response := NewResponseWithStatus("", http.StatusInternalServerError)
		return response, fmt.Errorf("no sender for %q", req.URL)
	}
	sender := (*s)[0]
	if ms, ok := sender.(*MockSender); !ok || ms.Attempts() >= ms.NumResponses()-1 {
		*s = (*s)[1:]
	}
	return sender.Do(req)
}

// SerialSender is a Sender that permits only one active Do call
// at a time.
type SerialSender struct {
	mu sync.Mutex
	s  policy.Transporter
}

func (s *SerialSender) Do(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.Do(req)
}

func NewSerialSender(s policy.Transporter) *SerialSender {
	return &SerialSender{s: s}
}
