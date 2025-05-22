// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package http

import (
	"context"
	"net/http"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type DialContextMiddlewareSuite struct {
	testhelpers.IsolationSuite
}

func TestDialContextMiddlewareSuite(t *stdtesting.T) {
	tc.Run(t, &DialContextMiddlewareSuite{})
}

var isLocalAddrTests = []struct {
	addr    string
	isLocal bool
}{
	{addr: "localhost:456", isLocal: true},
	{addr: "127.0.0.1:1234", isLocal: true},
	{addr: "[::1]:4567", isLocal: true},
	{addr: "localhost:smtp", isLocal: true},
	{addr: "123.45.67.5", isLocal: false},
	{addr: "0.1.2.3", isLocal: false},
	{addr: "10.0.43.6:12345", isLocal: false},
	{addr: ":456", isLocal: false},
	{addr: "12xz4.5.6", isLocal: false},
}

func (s *DialContextMiddlewareSuite) TestIsLocalAddr(c *tc.C) {
	for i, test := range isLocalAddrTests {
		c.Logf("test %d: %v", i, test.addr)
		c.Assert(isLocalAddr(test.addr), tc.Equals, test.isLocal)
	}
}

func (s *DialContextMiddlewareSuite) TestInsecureClientNoAccess(c *tc.C) {
	client := NewClient(
		WithTransportMiddlewares(
			DialContextMiddleware(NewLocalDialBreaker(false)),
		),
		WithSkipHostnameVerification(true),
	)
	_, err := client.Get(c.Context(), "http://0.1.2.3:1234")
	c.Assert(err, tc.ErrorMatches, `.*access to address "0.1.2.3:1234" not allowed`)
}

func (s *DialContextMiddlewareSuite) TestSecureClientNoAccess(c *tc.C) {
	client := NewClient(
		WithTransportMiddlewares(
			DialContextMiddleware(NewLocalDialBreaker(false)),
		),
	)
	_, err := client.Get(c.Context(), "http://0.1.2.3:1234")
	c.Assert(err, tc.ErrorMatches, `.*access to address "0.1.2.3:1234" not allowed`)
}

type LocalDialBreakerSuite struct {
	testhelpers.IsolationSuite
}

func TestLocalDialBreakerSuite(t *stdtesting.T) {
	tc.Run(t, &LocalDialBreakerSuite{})
}

func (s *LocalDialBreakerSuite) TestAllowed(c *tc.C) {
	breaker := NewLocalDialBreaker(true)

	for i, test := range isLocalAddrTests {
		c.Logf("test %d: %v", i, test.addr)
		allowed := breaker.Allowed(test.addr)
		c.Assert(allowed, tc.Equals, true)
	}
}

func (s *LocalDialBreakerSuite) TestLocalAllowed(c *tc.C) {
	breaker := NewLocalDialBreaker(false)

	for i, test := range isLocalAddrTests {
		c.Logf("test %d: %v", i, test.addr)
		allowed := breaker.Allowed(test.addr)
		c.Assert(allowed, tc.Equals, test.isLocal)
	}
}

func (s *LocalDialBreakerSuite) TestLocalAllowedAfterTrip(c *tc.C) {
	breaker := NewLocalDialBreaker(true)

	for i, test := range isLocalAddrTests {
		c.Logf("test %d: %v", i, test.addr)
		allowed := breaker.Allowed(test.addr)
		c.Assert(allowed, tc.Equals, true)

		breaker.Trip()

		allowed = breaker.Allowed(test.addr)
		c.Assert(allowed, tc.Equals, test.isLocal)

		// Reset the breaker.
		breaker.Trip()
	}
}

type RetrySuite struct {
	testhelpers.IsolationSuite
}

func TestRetrySuite(t *stdtesting.T) {
	tc.Run(t, &RetrySuite{})
}

func (s *RetrySuite) TestRetryNotRequired(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("GET", "http://meshuggah.rocks", nil)
	c.Assert(err, tc.IsNil)

	transport := NewMockRoundTripper(ctrl)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusOK,
	}, nil)

	middleware := makeRetryMiddleware(transport, RetryPolicy{
		Attempts: 3,
		Delay:    time.Second,
		MaxDelay: time.Minute,
	}, clock.WallClock, loggertesting.WrapCheckLog(c))

	resp, err := middleware.RoundTrip(req)
	c.Assert(err, tc.IsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *RetrySuite) TestRetryRequired(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("GET", "http://meshuggah.rocks", nil)
	c.Assert(err, tc.IsNil)

	transport := NewMockRoundTripper(ctrl)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusBadGateway,
	}, nil).Times(2)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusOK,
	}, nil)

	ch := make(chan time.Time)

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	clock.EXPECT().After(gomock.Any()).Return(ch).AnyTimes()

	retries := 3
	go func() {
		for i := 0; i < retries; i++ {
			ch <- time.Now()
		}
	}()

	middleware := makeRetryMiddleware(transport, RetryPolicy{
		Attempts: retries,
		Delay:    time.Second,
		MaxDelay: time.Minute,
	}, clock, loggertesting.WrapCheckLog(c))

	resp, err := middleware.RoundTrip(req)
	c.Assert(err, tc.IsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *RetrySuite) TestRetryRequiredUsingBackoff(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("GET", "http://meshuggah.rocks", nil)
	c.Assert(err, tc.IsNil)

	header := make(http.Header)
	header.Add("Retry-After", "42")

	transport := NewMockRoundTripper(ctrl)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     header,
	}, nil).Times(2)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusOK,
	}, nil)

	ch := make(chan time.Time)

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	clock.EXPECT().After(time.Second * 42).Return(ch).Times(2)

	retries := 3
	go func() {
		for i := 0; i < retries; i++ {
			ch <- time.Now()
		}
	}()

	middleware := makeRetryMiddleware(transport, RetryPolicy{
		Attempts: retries,
		Delay:    time.Second,
		MaxDelay: time.Minute,
	}, clock, loggertesting.WrapCheckLog(c))

	resp, err := middleware.RoundTrip(req)
	c.Assert(err, tc.IsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *RetrySuite) TestRetryRequiredUsingBackoffFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("GET", "http://meshuggah.rocks", nil)
	c.Assert(err, tc.IsNil)

	header := make(http.Header)
	header.Add("Retry-After", "2520")

	transport := NewMockRoundTripper(ctrl)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     header,
	}, nil)

	ch := make(chan time.Time)

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	clock.EXPECT().After(time.Minute * 42).Return(ch)

	retries := 3
	go func() {
		ch <- time.Now()
	}()

	middleware := makeRetryMiddleware(transport, RetryPolicy{
		Attempts: retries,
		Delay:    time.Minute,
		MaxDelay: time.Second,
	}, clock, loggertesting.WrapCheckLog(c))

	_, err = middleware.RoundTrip(req)
	c.Assert(err, tc.ErrorMatches, `API request retry is not accepting further requests until .*`)
}

func (s *RetrySuite) TestRetryRequiredUsingBackoffError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("GET", "http://meshuggah.rocks", nil)
	c.Assert(err, tc.IsNil)

	header := make(http.Header)
	header.Add("Retry-After", "!@1234391asd--\\123")

	transport := NewMockRoundTripper(ctrl)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     header,
	}, nil)

	ch := make(chan time.Time)

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	clock.EXPECT().After(time.Minute * 1).Return(ch)

	retries := 3
	go func() {
		ch <- time.Now()
	}()

	middleware := makeRetryMiddleware(transport, RetryPolicy{
		Attempts: retries,
		Delay:    time.Minute,
		MaxDelay: time.Second,
	}, clock, loggertesting.WrapCheckLog(c))

	_, err = middleware.RoundTrip(req)
	c.Assert(err, tc.ErrorMatches, `API request retry is not accepting further requests until .*`)
}

func (s *RetrySuite) TestRetryRequiredAndExceeded(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req, err := http.NewRequest("GET", "http://meshuggah.rocks", nil)
	c.Assert(err, tc.IsNil)

	transport := NewMockRoundTripper(ctrl)
	transport.EXPECT().RoundTrip(req).Return(&http.Response{
		StatusCode: http.StatusBadGateway,
	}, nil).Times(3)

	ch := make(chan time.Time)

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	clock.EXPECT().After(gomock.Any()).Return(ch).AnyTimes()

	retries := 3
	go func() {
		for i := 0; i < retries; i++ {
			ch <- time.Now()
		}
	}()

	middleware := makeRetryMiddleware(transport, RetryPolicy{
		Attempts: retries,
		Delay:    time.Second,
		MaxDelay: time.Minute,
	}, clock, loggertesting.WrapCheckLog(c))

	_, err = middleware.RoundTrip(req)
	c.Assert(err, tc.ErrorMatches, `attempt count exceeded: retryable error`)
}

func (s *RetrySuite) TestRetryRequiredContextKilled(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(c.Context())

	req, err := http.NewRequestWithContext(ctx, "GET", "http://meshuggah.rocks", nil)
	c.Assert(err, tc.IsNil)

	transport := NewMockRoundTripper(ctrl)

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now())

	middleware := makeRetryMiddleware(transport, RetryPolicy{
		Attempts: 3,
		Delay:    time.Second,
	}, clock, loggertesting.WrapCheckLog(c))

	// Nothing should run, the context has been cancelled.
	cancel()

	_, err = middleware.RoundTrip(req)
	c.Assert(err, tc.ErrorMatches, `context canceled`)
}
