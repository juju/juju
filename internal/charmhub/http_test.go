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
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	jujuhttp "github.com/juju/juju/internal/http"
)

type APIRequesterSuite struct {
	baseSuite
}

var _ = tc.Suite(&APIRequesterSuite{})

func (s *APIRequesterSuite) TestDo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	resp, err := requester.Do(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoWithFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), errors.Errorf("boom"))

	requester := newAPIRequester(mockHTTPClient, s.logger)
	_, err := requester.Do(req)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithInvalidContentType(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(invalidContentTypeResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	_, err := requester.Do(req)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithNotFoundResponse(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(notFoundResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	resp, err := requester.Do(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)
}

func (s *APIRequesterSuite) TestDoRetrySuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)
	mockHTTPClient.EXPECT().Do(req).Return(emptyResponse(), nil)

	requester := newAPIRequester(mockHTTPClient, s.logger)
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoRetrySuccessBody(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("POST", "http://api.foo.bar", strings.NewReader("body"))
	c.Assert(err, tc.ErrorIsNil)

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(b), tc.Equals, "body")
		return nil, io.EOF
	})
	mockHTTPClient.EXPECT().Do(req).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(b), tc.Equals, "body")
		return emptyResponse(), nil
	})

	requester := newAPIRequester(mockHTTPClient, s.logger)
	requester.retryDelay = time.Microsecond
	resp, err := requester.Do(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoRetryMaxAttempts(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, `attempt count exceeded: EOF`)
	elapsed := time.Since(start)
	c.Assert(elapsed >= (1+2+4)*time.Microsecond, tc.Equals, true)
}

func (s *APIRequesterSuite) TestDoRetryContextCanceled(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel right away
	req, err := http.NewRequestWithContext(ctx, "GET", "http://api.foo.bar", nil)
	c.Assert(err, tc.ErrorIsNil)

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(req).Return(nil, io.EOF)

	start := time.Now()
	requester := newAPIRequester(mockHTTPClient, s.logger)
	requester.retryDelay = time.Second
	_, err = requester.Do(req)
	c.Assert(err, tc.ErrorMatches, `retry stopped`)
	elapsed := time.Since(start)
	c.Assert(elapsed < 250*time.Millisecond, tc.Equals, true)
}

type RESTSuite struct {
	baseSuite
}

var _ = tc.Suite(&RESTSuite{})

func (s *RESTSuite) TestGet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var recievedURL string

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		recievedURL = req.URL.String()
		return emptyResponse(), nil
	})

	base := MustMakePath(c, "http://api.foo.bar")

	client := newHTTPRESTClient(mockHTTPClient)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recievedURL, tc.Equals, "http://api.foo.bar")
}

func (s *RESTSuite) TestGetWithInvalidContext(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	_, err := client.Get(nil, base, &result)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *RESTSuite) TestGetWithFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(emptyResponse(), errors.Errorf("boom"))

	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *RESTSuite) TestGetWithFailureRetry(c *tc.C) {
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
	})(s.logger)
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Assert(called, tc.Equals, 3)
}

func (s *RESTSuite) TestGetWithFailureWithoutRetry(c *tc.C) {
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
	})(s.logger)
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Assert(called, tc.Equals, 1)
}

func (s *RESTSuite) TestGetWithNoRetry(c *tc.C) {
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
	})(s.logger)
	client := newHTTPRESTClient(httpClient)

	base := MustMakePath(c, server.URL)

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.Equals, 1)
}

func (s *RESTSuite) TestGetWithUnmarshalFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHTTPClient := NewMockHTTPClient(ctrl)
	mockHTTPClient.EXPECT().Do(gomock.Any()).Return(invalidResponse(), nil)

	client := newHTTPRESTClient(mockHTTPClient)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	_, err := client.Get(context.Background(), base, &result)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
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
