// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"errors"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer/metricobserver"
)

type observerFactorySuite struct {
	testing.IsolationSuite
	clock      *testing.Clock
	registerer fakePrometheusRegisterer
}

var _ = gc.Suite(&observerFactorySuite{})

func (s *observerFactorySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testing.NewClock(time.Time{})
	s.registerer = fakePrometheusRegisterer{}
}

func (*observerFactorySuite) TestNewObserverFactoryInvalidConfig(c *gc.C) {
	_, err := metricobserver.NewObserverFactory(metricobserver.Config{})
	c.Assert(err, gc.ErrorMatches, "validating config: nil Clock not valid")
}

func (s *observerFactorySuite) TestNewObserverFactoryRegisterError(c *gc.C) {
	s.registerer.SetErrors(errors.New("oy vey"))
	_, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:                s.clock,
		PrometheusRegisterer: &s.registerer,
	})
	c.Assert(err, gc.ErrorMatches, "oy vey")
}

func (s *observerFactorySuite) TestNewObserverFactoryRegister(c *gc.C) {
	f, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:                s.clock,
		PrometheusRegisterer: &s.registerer,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f, gc.NotNil)
	s.registerer.CheckCallNames(c, "Register", "Register")
}

type fakePrometheusRegisterer struct {
	prometheus.Registerer
	testing.Stub
}

func (r *fakePrometheusRegisterer) Register(c prometheus.Collector) error {
	r.MethodCall(r, "Register", c)
	return r.NextErr()
}

func (r *fakePrometheusRegisterer) Unregister(c prometheus.Collector) bool {
	return true
}
