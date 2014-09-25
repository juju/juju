// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/cert"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type SenderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&SenderSuite{})

func createCerts(c *gc.C, serverName string) (*x509.CertPool, tls.Certificate) {
	certCaPem, keyCaPem, err := cert.NewCA("sender-test", time.Now().Add(time.Minute))
	c.Assert(err, gc.IsNil)
	certPem, keyPem, err := cert.NewServer(certCaPem, keyCaPem, time.Now().Add(time.Minute), []string{serverName})
	c.Assert(err, gc.IsNil)
	cert, err := tls.X509KeyPair([]byte(certPem), []byte(keyPem))
	c.Assert(err, gc.IsNil)
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM([]byte(certCaPem))
	return certPool, cert
}

// TestDefaultSender check that if the default sender
// is in use metrics get sent
func (s *SenderSuite) TestDefaultSender(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	expectedCharmUrl, _ := unit.CharmURL()
	ts := httptest.NewUnstartedServer(testHandler(c, expectedCharmUrl.String()))
	defer ts.Close()
	certPool, cert := createCerts(c, "127.0.0.1")
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	ts.StartTLS()
	cleanup := metricsender.PatchHostAndCertPool(ts.URL, certPool)
	defer cleanup()

	now := time.Now()
	metrics := make([]*state.MetricBatch, 3)
	for i, _ := range metrics {
		metrics[i] = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	}
	sender := &metricsender.DefaultSender{}
	err := s.State.SendMetrics(sender, 10)
	c.Assert(err, gc.IsNil)
	for _, metric := range metrics {
		err = metric.Refresh()
		c.Assert(err, gc.IsNil)
		c.Assert(metric.Sent(), jc.IsTrue)
	}
}

func testHandler(c *gc.C, expectedCharmUrl string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, gc.Equals, "POST")
		dec := json.NewDecoder(r.Body)
		var v []map[string]interface{}
		err := dec.Decode(&v)
		c.Assert(err, gc.IsNil)
		c.Assert(v, gc.HasLen, 3)
		for _, metric := range v {
			c.Assert(metric["CharmUrl"], gc.Equals, expectedCharmUrl)
		}
	}
}

// TestErrorCodes checks that for a set of error codes SendMetrics returns an
// error and metrics are marked as not being sent
func (s *SenderSuite) TestErrorCodes(c *gc.C) {
	tests := []struct {
		errorCode   int
		expectedErr string
	}{
		{http.StatusBadRequest, "failed to send metrics http 400"},
		{http.StatusServiceUnavailable, "failed to send metrics http 503"},
		{http.StatusMovedPermanently, "failed to send metrics http 301"},
	}
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})

	for _, test := range tests {
		ts := httptest.NewUnstartedServer(errorHandler(c, test.errorCode))
		defer ts.Close()
		certPool, cert := createCerts(c, "127.0.0.1")
		ts.TLS = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		ts.StartTLS()
		cleanup := metricsender.PatchHostAndCertPool(ts.URL, certPool)
		defer cleanup()

		now := time.Now()
		metrics := make([]*state.MetricBatch, 3)
		for i, _ := range metrics {
			metrics[i] = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
		}
		sender := &metricsender.DefaultSender{}
		err := s.State.SendMetrics(sender, 10)
		c.Assert(err, gc.ErrorMatches, test.expectedErr)
		for _, metric := range metrics {
			err = metric.Refresh()
			c.Assert(err, gc.IsNil)
			c.Assert(metric.Sent(), jc.IsFalse)
		}
	}
}

func errorHandler(c *gc.C, errorCode int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(errorCode)
	}
}
