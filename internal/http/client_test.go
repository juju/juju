// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type clientSuite struct{}

func TestClientSuite(t *stdtesting.T) { tc.Run(t, &clientSuite{}) }
func (s *clientSuite) TestNewClient(c *tc.C) {
	client := NewClient()
	c.Assert(client, tc.NotNil)
}

type httpSuite struct {
	testhelpers.IsolationSuite
	server *httptest.Server
}

func TestHttpSuite(t *stdtesting.T) { tc.Run(t, &httpSuite{}) }
func (s *httpSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
}

func (s *httpSuite) TestInsecureClientAllowAccess(c *tc.C) {
	client := NewClient(WithSkipHostnameVerification(true))
	_, err := client.Get(c.Context(), s.server.URL)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *httpSuite) TestSecureClientAllowAccess(c *tc.C) {
	client := NewClient()
	_, err := client.Get(c.Context(), s.server.URL)
	c.Assert(err, tc.ErrorIsNil)
}

// NewClient with a default config used to overwrite http.DefaultClient.Jar
// field; add a regression test for that.
func (s *httpSuite) TestDefaultClientJarNotOverwritten(c *tc.C) {
	oldJar := http.DefaultClient.Jar

	jar, err := cookiejar.New(nil)
	c.Assert(err, tc.ErrorIsNil)

	client := NewClient(WithCookieJar(jar))

	hc := client.HTTPClient.(*http.Client)
	c.Assert(hc.Jar, tc.Equals, jar)
	c.Assert(http.DefaultClient.Jar, tc.Not(tc.Equals), jar)
	c.Assert(http.DefaultClient.Jar, tc.Equals, oldJar)

	http.DefaultClient.Jar = oldJar
}

func (s *httpSuite) TestRequestRecorder(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	dummyServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		_, _ = fmt.Fprintln(res, "they are listening...")
	}))
	defer dummyServer.Close()

	validTarget := fmt.Sprintf("%s/tin/foil", dummyServer.URL)
	validTargetURL, err := url.Parse(validTarget)
	c.Assert(err, tc.ErrorIsNil)

	invalidTarget := "btc://secret/wallet"
	invalidTargetURL, err := url.Parse(invalidTarget)
	c.Assert(err, tc.ErrorIsNil)

	recorder := NewMockRequestRecorder(ctrl)
	recorder.EXPECT().Record("GET", validTargetURL, gomock.AssignableToTypeOf(&http.Response{}), gomock.AssignableToTypeOf(time.Duration(42)))
	recorder.EXPECT().RecordError("PUT", invalidTargetURL, gomock.Any())

	client := NewClient(WithRequestRecorder(recorder))
	res, err := client.Get(c.Context(), validTarget)
	c.Assert(err, tc.ErrorIsNil)
	defer res.Body.Close()

	req, err := http.NewRequestWithContext(c.Context(), "PUT", invalidTarget, nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.Do(req)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *httpSuite) TestRetry(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	attempts := 0
	retries := 3
	dummyServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if attempts < retries-1 {
			res.WriteHeader(http.StatusBadGateway)
		} else {
			res.WriteHeader(http.StatusOK)
		}
		attempts++
		_, _ = fmt.Fprintln(res, "they are listening...")
	}))
	defer dummyServer.Close()

	validTarget := fmt.Sprintf("%s/tin/foil", dummyServer.URL)
	validTargetURL, err := url.Parse(validTarget)
	c.Assert(err, tc.ErrorIsNil)

	recorder := NewMockRequestRecorder(ctrl)
	recorder.EXPECT().Record("GET", validTargetURL, gomock.AssignableToTypeOf(&http.Response{}), gomock.AssignableToTypeOf(time.Duration(42))).Times(retries)

	client := NewClient(
		// We can use the request recorder to monitor how many retries have been
		// made.
		WithRequestRecorder(recorder),
		WithRequestRetrier(RetryPolicy{
			Delay:    time.Nanosecond,
			Attempts: retries,
			MaxDelay: time.Minute,
		}),
	)
	res, err := client.Get(c.Context(), validTarget)
	c.Assert(err, tc.ErrorIsNil)
	defer res.Body.Close()
}

func (s *httpSuite) TestRetryExceeded(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	retries := 3
	dummyServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintln(res, "they are listening...")
	}))
	defer dummyServer.Close()

	validTarget := fmt.Sprintf("%s/tin/foil", dummyServer.URL)
	validTargetURL, err := url.Parse(validTarget)
	c.Assert(err, tc.ErrorIsNil)

	recorder := NewMockRequestRecorder(ctrl)
	recorder.EXPECT().Record("GET", validTargetURL, gomock.AssignableToTypeOf(&http.Response{}), gomock.AssignableToTypeOf(time.Duration(42))).Times(retries)

	client := NewClient(
		// We can use the request recorder to monitor how many retries have been
		// made.
		WithRequestRecorder(recorder),
		WithRequestRetrier(RetryPolicy{
			Delay:    time.Nanosecond,
			Attempts: retries,
			MaxDelay: time.Minute,
		}),
	)
	_, err = client.Get(c.Context(), validTarget)
	c.Assert(err, tc.ErrorMatches, `.*attempt count exceeded: retryable error`)
}

type httpTLSServerSuite struct {
	testhelpers.IsolationSuite
	server *httptest.Server
}

func TestHttpTLSServerSuite(t *stdtesting.T) { tc.Run(t, &httpTLSServerSuite{}) }
func (s *httpTLSServerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	// NewTLSServer returns a server which serves TLS, but
	// its certificates are not validated by the default
	// OS certificates, so any HTTPS request will fail
	// unless a non-validating client is used.
	s.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
}

func (s *httpTLSServerSuite) TearDownTest(c *tc.C) {
	if s.server != nil {
		s.server.Close()
	}
	s.IsolationSuite.TearDownTest(c)
}

func (s *httpTLSServerSuite) TestValidatingClientGetter(c *tc.C) {
	client := NewClient()
	_, err := client.Get(c.Context(), s.server.URL)
	c.Assert(err, tc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
}

func (s *httpTLSServerSuite) TestNonValidatingClientGetter(c *tc.C) {
	client := NewClient(WithSkipHostnameVerification(true))
	resp, err := client.Get(c.Context(), s.server.URL)
	c.Assert(err, tc.IsNil)
	_ = resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *httpTLSServerSuite) TestGetHTTPClientWithCertsVerify(c *tc.C) {
	s.testGetHTTPClientWithCerts(c, true)
}

func (s *httpTLSServerSuite) TestGetHTTPClientWithCertsNoVerify(c *tc.C) {
	s.testGetHTTPClientWithCerts(c, false)
}

func (s *httpTLSServerSuite) testGetHTTPClientWithCerts(c *tc.C, skip bool) {
	caPEM := new(bytes.Buffer)
	err := pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.server.Certificate().Raw,
	})
	c.Assert(err, tc.IsNil)

	client := NewClient(
		WithCACertificates(caPEM.String()),
		WithSkipHostnameVerification(skip),
	)
	resp, err := client.Get(c.Context(), s.server.URL)
	c.Assert(err, tc.IsNil)
	c.Assert(resp.Body.Close(), tc.IsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *clientSuite) TestDisableKeepAlives(c *tc.C) {
	client := NewClient()
	transport := client.Client().Transport.(*http.Transport)
	c.Assert(transport.DisableKeepAlives, tc.Equals, false)

	client = NewClient(WithDisableKeepAlives(false))
	transport = client.Client().Transport.(*http.Transport)
	c.Assert(transport.DisableKeepAlives, tc.Equals, false)

	client = NewClient(WithDisableKeepAlives(true))
	transport = client.Client().Transport.(*http.Transport)
	c.Assert(transport.DisableKeepAlives, tc.Equals, true)
}
