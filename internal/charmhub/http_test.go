// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type APIRequesterSuite struct {
	baseSuite
}

var _ = gc.Suite(&APIRequesterSuite{})

func (s *APIRequesterSuite) TestDo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoWithFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), errors.Errorf("boom"))

	requester := newAPIRequester(mockHTTPClient, s.logger)
	_, err := requester.Do(req)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithInvalidContentType(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(invalidContentTypeResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	_, err := requester.Do(req)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithNotFoundResponse(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(notFoundResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func (s *APIRequesterSuite) TestDoRetrySuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
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
		return emptyResponse(), nil
	})

	requester := newAPIRequester(mockHTTPClient, s.logger)
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
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
	requester := newAPIRequester(mockHTTPClient, s.logger)
	requester.retryDelay = time.Microsecond
	_, err := requester.Do(req)
	c.Assert(err, gc.ErrorMatches, `attempt count exceeded: EOF`)
	elapsed := time.Since(start)
	c.Assert(elapsed >= (1+2+4)*time.Microsecond, gc.Equals, true)
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
	requester := newAPIRequester(mockHTTPClient, s.logger)
	requester.retryDelay = time.Second
	_, err = requester.Do(req)
	c.Assert(err, gc.ErrorMatches, `retry stopped`)
	elapsed := time.Since(start)
	c.Assert(elapsed < 250*time.Millisecond, gc.Equals, true)
}

type RESTSuite struct {
	baseSuite
}

var _ = gc.Suite(&RESTSuite{})

func (s *RESTSuite) TestGet(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var recievedURL string

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		recievedURL = req.URL.String()
	}).Return(emptyResponse(), nil)

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
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(emptyResponse(), errors.Errorf("boom"))

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
	})(s.loggerFactory)
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
	})(s.loggerFactory)
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
	})(s.loggerFactory)
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
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(invalidResponse(), nil)

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
