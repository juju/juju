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
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/apiserver/metricsender/wireformat"
	"github.com/juju/juju/cert"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type SenderSuite struct {
	jujutesting.JujuConnSuite
	unit           *state.Unit
	meteredService *state.Service
}

var _ = gc.Suite(&SenderSuite{})

func createCerts(c *gc.C, serverName string) (*x509.CertPool, tls.Certificate) {
	certCaPem, keyCaPem, err := cert.NewCA("sender-test", time.Now().Add(time.Minute))
	c.Assert(err, jc.ErrorIsNil)
	certPem, keyPem, err := cert.NewServer(certCaPem, keyCaPem, time.Now().Add(time.Minute), []string{serverName})
	c.Assert(err, jc.ErrorIsNil)
	cert, err := tls.X509KeyPair([]byte(certPem), []byte(keyPem))
	c.Assert(err, jc.ErrorIsNil)
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM([]byte(certCaPem))
	return certPool, cert
}

func (s *SenderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	s.meteredService = s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.meteredService, SetCharmURL: true})
}

// startServer starts a server with TLS and the specified handler, returning a
// function that should be run at the end of the test to clean up.
func (s *SenderSuite) startServer(c *gc.C, handler http.Handler) func() {
	ts := httptest.NewUnstartedServer(handler)
	certPool, cert := createCerts(c, "127.0.0.1")
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	ts.StartTLS()
	cleanup := metricsender.PatchHostAndCertPool(ts.URL, certPool)
	return func() {
		ts.Close()
		cleanup()
	}
}

var _ metricsender.MetricSender = (*metricsender.HttpSender)(nil)

// TestHttpSender checks that if the default sender
// is in use metrics get sent
func (s *SenderSuite) TestHttpSender(c *gc.C) {
	metricCount := 3
	expectedCharmUrl, _ := s.unit.CharmURL()

	receiverChan := make(chan wireformat.MetricBatch, metricCount)
	cleanup := s.startServer(c, testHandler(c, receiverChan, nil, 0))
	defer cleanup()

	now := time.Now()
	metrics := make([]*state.MetricBatch, metricCount)
	for i := range metrics {
		metrics[i] = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now})
	}
	var sender metricsender.HttpSender
	err := metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(receiverChan, gc.HasLen, metricCount)
	close(receiverChan)
	for batch := range receiverChan {
		c.Assert(batch.CharmUrl, gc.Equals, expectedCharmUrl.String())
	}

	for _, metric := range metrics {
		m, err := s.State.MetricBatch(metric.UUID())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Sent(), jc.IsTrue)
	}
}

// StatusMap defines a type for a function that returns the status and information for a specified unit.
type StatusMap func(unitName string) (unit string, status string, info string)

func errorHandler(c *gc.C, errorCode int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(errorCode)
	}
}

func testHandler(c *gc.C, batches chan<- wireformat.MetricBatch, statusMap StatusMap, gracePeriod time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, gc.Equals, "POST")
		dec := json.NewDecoder(r.Body)
		enc := json.NewEncoder(w)
		var incoming []wireformat.MetricBatch
		err := dec.Decode(&incoming)
		c.Assert(err, jc.ErrorIsNil)

		var resp = make(wireformat.EnvironmentResponses)
		for _, batch := range incoming {
			c.Logf("received metrics batch: %+v", batch)

			resp.Ack(batch.EnvUUID, batch.UUID)

			if statusMap != nil {
				unitName, status, info := statusMap(batch.UnitName)
				resp.SetStatus(batch.EnvUUID, unitName, status, info)
			}

			select {
			case batches <- batch:
			default:
			}
		}
		uuid, err := utils.NewUUID()
		c.Assert(err, jc.ErrorIsNil)
		err = enc.Encode(wireformat.Response{
			UUID:           uuid.String(),
			EnvResponses:   resp,
			NewGracePeriod: gracePeriod,
		})
		c.Assert(err, jc.ErrorIsNil)
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

	for _, test := range tests {
		killServer := s.startServer(c, errorHandler(c, test.errorCode))

		now := time.Now()
		batches := make([]*state.MetricBatch, 3)
		for i := range batches {
			batches[i] = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now})
		}
		var sender metricsender.HttpSender
		err := metricsender.SendMetrics(s.State, &sender, 10)
		c.Assert(err, gc.ErrorMatches, test.expectedErr)
		for _, batch := range batches {
			m, err := s.State.MetricBatch(batch.UUID())
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(m.Sent(), jc.IsFalse)
		}
		killServer()
	}
}

// TestMeterStatus checks that the meter status information returned
// by the collector service is propagated to the unit.
// is in use metrics get sent
func (s *SenderSuite) TestMeterStatus(c *gc.C) {
	statusFunc := func(unitName string) (string, string, string) {
		return unitName, "GREEN", ""
	}

	cleanup := s.startServer(c, testHandler(c, nil, statusFunc, 0))
	defer cleanup()

	_ = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})

	status, err := s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterNotSet)

	var sender metricsender.HttpSender
	err = metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, jc.ErrorIsNil)

	status, err = s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterGreen)
}

// TestMeterStatusInvalid checks that the metric sender deals with invalid
// meter status data properly.
func (s *SenderSuite) TestMeterStatusInvalid(c *gc.C) {
	unit1 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.meteredService, SetCharmURL: true})
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.meteredService, SetCharmURL: true})
	unit3 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.meteredService, SetCharmURL: true})

	statusFunc := func(unitName string) (string, string, string) {
		switch unitName {
		case unit1.Name():
			// valid meter status
			return unitName, "GREEN", ""
		case unit2.Name():
			// invalid meter status
			return unitName, "blah", ""
		case unit3.Name():
			// invalid unit name
			return "no-such-unit", "GREEN", ""
		default:
			return unitName, "GREEN", ""
		}
	}

	cleanup := s.startServer(c, testHandler(c, nil, statusFunc, 0))
	defer cleanup()

	_ = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit1, Sent: false})
	_ = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit2, Sent: false})
	_ = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit3, Sent: false})

	for _, unit := range []*state.Unit{unit1, unit2, unit3} {
		status, err := unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Code, gc.Equals, state.MeterNotSet)
	}

	var sender metricsender.HttpSender
	err := metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, jc.ErrorIsNil)

	status, err := unit1.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterGreen)

	status, err = unit2.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterNotSet)

	status, err = unit3.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterNotSet)

}

func (s *SenderSuite) TestGracePeriodResponse(c *gc.C) {
	_ = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})
	cleanup := s.startServer(c, testHandler(c, nil, nil, 47*time.Hour))
	defer cleanup()
	var sender metricsender.HttpSender
	err := metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, jc.ErrorIsNil)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.GracePeriod(), gc.Equals, 47*time.Hour)
}

func (s *SenderSuite) TestNegativeGracePeriodResponse(c *gc.C) {
	_ = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})

	cleanup := s.startServer(c, testHandler(c, nil, nil, -47*time.Hour))
	defer cleanup()
	var sender metricsender.HttpSender
	err := metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, jc.ErrorIsNil)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.GracePeriod(), gc.Equals, 24*time.Hour*7) //Default (unchanged)
}

func (s *SenderSuite) TestZeroGracePeriodResponse(c *gc.C) {
	_ = s.Factory.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})

	cleanup := s.startServer(c, testHandler(c, nil, nil, 0))
	defer cleanup()
	var sender metricsender.HttpSender
	err := metricsender.SendMetrics(s.State, &sender, 10)
	c.Assert(err, jc.ErrorIsNil)
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.GracePeriod(), gc.Equals, 24*time.Hour*7) //Default (unchanged)
}
