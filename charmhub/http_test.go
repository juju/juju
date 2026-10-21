// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type APIRequesterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&APIRequesterSuite{})

func (s *APIRequesterSuite) TestIsTimeout(c *gc.C) {
	for _, test := range []struct {
		name    string
		err     error
		timeout bool
	}{
		{name: "nil"},
		{name: "ordinary error", err: errors.New("boom")},
		{name: "non-timeout", err: &net.DNSError{}},
		{name: "timeout", err: &net.DNSError{IsTimeout: true}, timeout: true},
		{
			name: "HTTP wrapper masks underlying timeout",
			err: errors.Trace(&url.Error{
				Op: "Post", URL: "https://api.charmhub.io", Err: errors.Trace(&net.DNSError{IsTimeout: true}),
			}),
			timeout: true,
		},
		{
			name: "wrapped non-timeout",
			err:  &url.Error{Err: errors.Trace(&net.DNSError{})},
		},
	} {
		c.Check(isTimeout(test.err), gc.Equals, test.timeout, gc.Commentf("%s", test.name))
	}
}

func (s *APIRequesterSuite) TestDo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoWithFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), errors.Errorf("boom")) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	_, err := requester.Do(req) //nolint:bodyclose
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithInvalidContentType(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(invalidContentTypeResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	_, err := requester.Do(req) //nolint:bodyclose
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithNotFoundResponse(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(notFoundResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func (s *APIRequesterSuite) TestDoRetrySuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil) //nolint:bodyclose

	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoRetrySuccessBody(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("POST", "http://api.foo.bar", strings.NewReader("body"))
	c.Assert(err, jc.ErrorIsNil)

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(b), gc.Equals, "body")
		return nil, io.EOF
	})
	mockHTTPClient.EXPECT().Do(req).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(b), gc.Equals, "body")
		return emptyResponse(), nil //nolint:bodyclose
	})

	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoRetryMaxAttempts(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)

	start := time.Now()
	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	requester.retryDelay = time.Microsecond
	_, err := requester.Do(req) //nolint:bodyclose
	c.Assert(err, gc.ErrorMatches, `attempt count exceeded: EOF`)
	elapsed := time.Since(start)
	c.Assert(elapsed >= (1+2+4)*time.Microsecond, gc.Equals, true)
}

func (s *APIRequesterSuite) TestDoRetryNetworkTimeout(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "https://api.foo.bar")
	timeoutErr := &url.Error{
		Op:  "Get",
		URL: req.URL.String(),
		Err: errors.Trace(&net.DNSError{IsTimeout: true}),
	}
	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, timeoutErr).Times(2)

	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	requester.retryDelay = time.Microsecond
	_, err := requester.Do(req) //nolint:bodyclose
	c.Assert(err, gc.ErrorMatches, `attempt count exceeded: .*`)
}

func (s *APIRequesterSuite) TestDoDoesNotRetryPermanentErrors(c *gc.C) {
	for _, cause := range []error{
		context.Canceled,
		x509.UnknownAuthorityError{},
	} {
		ctrl := gomock.NewController(c)
		req := MustNewRequest(c, "https://api.foo.bar")
		wrappedErr := &url.Error{Op: "Get", URL: req.URL.String(), Err: cause}
		mockHTTPClient := NewMockHTTPClient(ctrl)
		mockHTTPClient.EXPECT().Do(req).Return(nil, wrappedErr)

		requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
		requester.retryDelay = time.Microsecond
		_, err := requester.Do(req) //nolint:bodyclose
		c.Check(err, jc.ErrorIs, cause)
		ctrl.Finish()
	}
}

func (s *APIRequesterSuite) TestDoRetryTLSHandshakeTimeout(c *gc.C) {
	releaseHandshake := make(chan struct{})
	var handshakes int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", jsonContentType)
		_, _ = io.Copy(w, req.Body)
	}))
	server.TLS = &tls.Config{
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			if atomic.AddInt32(&handshakes, 1) == 1 {
				<-releaseHandshake
			}
			return nil, nil
		},
	}
	server.StartTLS()
	defer server.Close()
	defer close(releaseHandshake)

	httpClient := jujuhttp.NewClient(
		jujuhttp.WithCACertificates(string(pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: server.Certificate().Raw,
		}))),
		jujuhttp.WithTLSHandshakeTimeout(time.Second),
		jujuhttp.WithRequestRetrier(defaultRetryPolicy()),
	)
	defer httpClient.HTTPClient.(*http.Client).CloseIdleConnections()
	requester := newAPIRequester(httpClient, &FakeLogger{})
	requester.retryDelay = time.Microsecond

	body := `{"actions":[],"context":[]}`
	req, err := http.NewRequest("POST", server.URL+"/v2/charms/refresh", strings.NewReader(body))
	c.Assert(err, jc.ErrorIsNil)
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(responseBody), gc.Equals, body)
	c.Check(atomic.LoadInt32(&handshakes), gc.Equals, int32(2))
}

func (s *APIRequesterSuite) TestDoRetryContextCanceled(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel right away
	req, err := http.NewRequestWithContext(ctx, "GET", "http://api.foo.bar", nil)
	c.Assert(err, jc.ErrorIsNil)

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)

	start := time.Now()
	requester := newAPIRequester(mockHTTPClient, &FakeLogger{})
	requester.retryDelay = time.Second
	_, err = requester.Do(req) //nolint:bodyclose
	c.Assert(err, gc.ErrorMatches, `retry stopped`)
	elapsed := time.Since(start)
	c.Assert(elapsed < 250*time.Millisecond, gc.Equals, true)
}

type RESTSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RESTSuite{})

func (s *RESTSuite) TestGet(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var recievedURL string

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		recievedURL = req.URL.String()
	}).Return(emptyResponse(), nil) //nolint:bodyclose

	base := MustMakePath(c, "http://api.foo.bar")

	client := newHTTPRESTClient(mockHTTPClient)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recievedURL, gc.Equals, "http://api.foo.bar")
}

func (s *RESTSuite) TestGetWithInvalidContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	_, err := client.Get(nil, base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *RESTSuite) TestGetWithFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(emptyResponse(), errors.Errorf("boom")) //nolint:bodyclose

	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *RESTSuite) TestGetWithFailureRetry(c *gc.C) {
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	httpClient := requestHTTPClient(nil, jujuhttp.RetryPolicy{
		Attempts: 3,
		Delay:    testing.ShortWait,
		MaxDelay: testing.LongWait,
	})(&FakeLogger{})
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Assert(called, gc.Equals, 3)
}

func (s *RESTSuite) TestGetWithFailureWithoutRetry(c *gc.C) {
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	httpClient := requestHTTPClient(nil, jujuhttp.RetryPolicy{
		Attempts: 3,
		Delay:    testing.ShortWait,
		MaxDelay: testing.LongWait,
	})(&FakeLogger{})
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Assert(called, gc.Equals, 1)
}

func (s *RESTSuite) TestGetWithNoRetry(c *gc.C) {
	var called int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "{}")
	}))
	defer server.Close()

	httpClient := requestHTTPClient(nil, jujuhttp.RetryPolicy{
		Attempts: 3,
		Delay:    testing.ShortWait,
		MaxDelay: testing.LongWait,
	})(&FakeLogger{})
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *RESTSuite) TestGetWithUnmarshalFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(invalidResponse(), nil) //nolint:bodyclose

	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func emptyResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusOK,
		Body:       MakeNopCloser(bytes.NewBufferString("{}")),
	}
}

func invalidResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusOK,
		Body:       MakeNopCloser(bytes.NewBufferString("/\\!")),
	}
}

func invalidContentTypeResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("text/plain"),
		StatusCode: http.StatusNotFound,
		Body:       MakeNopCloser(bytes.NewBufferString("")),
	}
}

func notFoundResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusNotFound,
		Body: MakeNopCloser(bytes.NewBufferString(`
{
	"code":"404",
	"message":"not-found"
}
		`)),
	}
}
