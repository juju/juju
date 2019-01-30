// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer/metricobserver"
)

type observerFactorySuite struct {
	testing.IsolationSuite
	clock *testclock.Clock
}

var _ = gc.Suite(&observerFactorySuite{})

func (s *observerFactorySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Time{})
}

func (*observerFactorySuite) TestNewObserverFactoryInvalidConfig(c *gc.C) {
	_, err := metricobserver.NewObserverFactory(metricobserver.Config{})
	c.Assert(err, gc.ErrorMatches, "validating config: nil Clock not valid")
}

func (s *observerFactorySuite) TestNewObserverFactoryRegister(c *gc.C) {
	metricsCollector, finish := createMockMetrics(c, gomock.AssignableToTypeOf(prometheus.Labels{}))
	defer finish()

	f, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:            s.clock,
		MetricsCollector: metricsCollector,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f, gc.NotNil)
}
