// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/metricsender"
	jujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type SenderSuite struct {
	jujujutesting.ApiServerSuite
	unit           *state.Unit
	meteredService *state.Application
	clock          clock.Clock
}

var _ = gc.Suite(&SenderSuite{})

func (s *SenderSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:quantal/metered"})
	s.meteredService = f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	s.unit = f.MakeUnit(c, &factory.UnitParams{Application: s.meteredService, SetCharmURL: true})
	s.clock = testclock.NewClock(time.Now())
}

// startServer starts a test HTTP server, returning a function that should be
// run at the end of the test to clean up.
func (s *SenderSuite) startServer(c *gc.C, handler http.Handler) func() {
	ts := httptest.NewServer(handler)
	cleanup := metricsender.PatchHost(ts.URL)
	return func() {
		ts.Close()
		cleanup()
	}
}

var _ metricsender.MetricSender = (*metricsender.HTTPSender)(nil)

// TestHTTPSender checks that if the default sender
// is in use metrics get sent
func (s *SenderSuite) TestHTTPSender(c *gc.C) {
	metricCount := 3
	expectedCharmURL := s.unit.CharmURL()
	c.Assert(expectedCharmURL, gc.NotNil)

	receiverChan := make(chan wireformat.MetricBatch, metricCount)
	cleanup := s.startServer(c, testHandler(c, receiverChan, nil, 0))
	defer cleanup()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	now := time.Now()
	metrics := make([]*state.MetricBatch, metricCount)
	for i := range metrics {
		metrics[i] = f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now})
	}
	sender := metricsender.DefaultSenderFactory()("http://example.com")
	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(receiverChan, gc.HasLen, metricCount)
	close(receiverChan)
	for batch := range receiverChan {
		c.Assert(batch.CharmUrl, gc.Equals, *expectedCharmURL)
	}

	for _, metric := range metrics {
		m, err := st.MetricBatch(metric.UUID())
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

			resp.Ack(batch.ModelUUID, batch.UUID)

			if statusMap != nil {
				unitName, status, info := statusMap(batch.UnitName)
				resp.SetUnitStatus(batch.ModelUUID, unitName, status, info)
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
	}

	for _, test := range tests {
		killServer := s.startServer(c, errorHandler(c, test.errorCode))

		f, release := s.NewFactory(c, s.ControllerModelUUID())
		defer release()

		now := time.Now()
		batches := make([]*state.MetricBatch, 3)
		for i := range batches {
			batches[i] = f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false, Time: &now})
		}
		sender := metricsender.DefaultSenderFactory()("http://example.com")
		st := s.ControllerModel(c).State()
		err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 10, true)
		c.Assert(err, gc.ErrorMatches, test.expectedErr)
		for _, batch := range batches {
			m, err := st.MetricBatch(batch.UUID())
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	_ = f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})

	status, err := s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterNotSet)

	sender := metricsender.DefaultSenderFactory()("http://example.com")
	st := s.ControllerModel(c).State()
	err = metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)

	status, err = s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterGreen)
}

// TestMeterStatusInvalid checks that the metric sender deals with invalid
// meter status data properly.
func (s *SenderSuite) TestMeterStatusInvalid(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	unit1 := f.MakeUnit(c, &factory.UnitParams{Application: s.meteredService, SetCharmURL: true})
	unit2 := f.MakeUnit(c, &factory.UnitParams{Application: s.meteredService, SetCharmURL: true})
	unit3 := f.MakeUnit(c, &factory.UnitParams{Application: s.meteredService, SetCharmURL: true})

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

	_ = f.MakeMetric(c, &factory.MetricParams{Unit: unit1, Sent: false})
	_ = f.MakeMetric(c, &factory.MetricParams{Unit: unit2, Sent: false})
	_ = f.MakeMetric(c, &factory.MetricParams{Unit: unit3, Sent: false})

	for _, unit := range []*state.Unit{unit1, unit2, unit3} {
		status, err := unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Code, gc.Equals, state.MeterNotSet)
	}

	sender := metricsender.DefaultSenderFactory()("http://example.com")
	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 10, true)
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	_ = f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})
	cleanup := s.startServer(c, testHandler(c, nil, nil, 47*time.Hour))
	defer cleanup()
	sender := metricsender.DefaultSenderFactory()("http://example.com")
	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	mm, err := st.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.GracePeriod(), gc.Equals, 47*time.Hour)
}

func (s *SenderSuite) TestNegativeGracePeriodResponse(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	_ = f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})

	cleanup := s.startServer(c, testHandler(c, nil, nil, -47*time.Hour))
	defer cleanup()
	sender := metricsender.DefaultSenderFactory()("http://example.com")
	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	mm, err := st.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.GracePeriod(), gc.Equals, 24*time.Hour*7) //Default (unchanged)
}

func (s *SenderSuite) TestZeroGracePeriodResponse(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	_ = f.MakeMetric(c, &factory.MetricParams{Unit: s.unit, Sent: false})

	cleanup := s.startServer(c, testHandler(c, nil, nil, 0))
	defer cleanup()
	sender := metricsender.DefaultSenderFactory()("http://example.com")
	st := s.ControllerModel(c).State()
	err := metricsender.SendMetrics(context.Background(), TestSenderBackend{st, s.ControllerModel(c)}, sender, s.clock, 10, true)
	c.Assert(err, jc.ErrorIsNil)
	mm, err := st.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mm.GracePeriod(), gc.Equals, 24*time.Hour*7) //Default (unchanged)
}
