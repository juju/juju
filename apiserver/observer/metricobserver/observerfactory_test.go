// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/observer/metricobserver"
)

type observerFactorySuite struct {
	testing.IsolationSuite
	clock *testclock.Clock
}

var _ = tc.Suite(&observerFactorySuite{})

func (s *observerFactorySuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Time{})
}

func (*observerFactorySuite) TestNewObserverFactoryInvalidConfig(c *tc.C) {
	_, err := metricobserver.NewObserverFactory(metricobserver.Config{})
	c.Assert(err, tc.ErrorMatches, "validating config: nil Clock not valid")
}

func (s *observerFactorySuite) TestNewObserverFactoryRegister(c *tc.C) {
	metricsCollector, finish := createMockMetrics(c, gomock.AssignableToTypeOf(prometheus.Labels{}))
	defer finish()

	f, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:            s.clock,
		MetricsCollector: metricsCollector,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f, tc.NotNil)
}
