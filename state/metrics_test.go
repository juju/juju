// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
)

type MetricSuite struct {
	ConnSuite
}

var _ = gc.Suite(&MetricSuite{})

func (s *MetricSuite) TestAddNoMetrics(c *gc.C) {
	unit := s.assertAddUnit(c)
	_, err := unit.AddMetrics([]*state.Metric{})
	c.Assert(err, gc.ErrorMatches, "cannot add a batch of 0 metrics")
}

func (s *MetricSuite) TestAddMetric(c *gc.C) {
	unit := s.assertAddUnit(c)
	now := state.NowToTheSecond()
	m := state.NewMetric("item", "5", now, []byte("creds"))
	metricBatch, err := unit.AddMetrics([]*state.Metric{m})
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatch.Unit(), gc.Equals, "wordpress/0")
	c.Assert(metricBatch.CharmURL(), gc.Equals, "local:quantal/quantal-wordpress-3")
	c.Assert(metricBatch.Sent(), gc.Equals, false)
	c.Assert(metricBatch.Metrics(), gc.HasLen, 1)

	metric := metricBatch.Metrics()[0]
	c.Assert(metric.Key(), gc.Equals, "item")
	c.Assert(metric.Value(), gc.Equals, "5")
	c.Assert(metric.Time().Equal(now), jc.IsTrue)
	c.Assert(metric.Credentials(), gc.DeepEquals, []byte("creds"))

	saved, err := s.State.MetricBatch(metricBatch.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Unit(), gc.Equals, "wordpress/0")
	c.Assert(metricBatch.CharmURL(), gc.Equals, "local:quantal/quantal-wordpress-3")
	c.Assert(saved.Sent(), gc.Equals, false)
	c.Assert(saved.Metrics(), gc.HasLen, 1)
	metric = saved.Metrics()[0]
	c.Assert(metric.Key(), gc.Equals, "item")
	c.Assert(metric.Value(), gc.Equals, "5")
	c.Assert(metric.Time().Equal(now), jc.IsTrue)
	c.Assert(metric.Credentials(), gc.DeepEquals, []byte("creds"))
}

func assertUnitRemoved(c *gc.C, unit *state.Unit) {
	assertUnitDead(c, unit)
	err := unit.Remove()
	c.Assert(err, gc.IsNil)
}

func assertUnitDead(c *gc.C, unit *state.Unit) {
	err := unit.EnsureDead()
	c.Assert(err, gc.IsNil)
}

func (s *MetricSuite) assertAddUnit(c *gc.C) *state.Unit {
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.SetCharmURL(charm.URL())
	c.Assert(err, gc.IsNil)
	return unit
}

func (s *MetricSuite) TestAddMetricNonExitentUnit(c *gc.C) {
	unit := s.assertAddUnit(c)
	assertUnitRemoved(c, unit)
	now := state.NowToTheSecond()
	m := state.NewMetric("item", "5", now, []byte{})
	_, err := unit.AddMetrics([]*state.Metric{m})
	c.Assert(err, gc.ErrorMatches, `wordpress/0 not found`)
}

func (s *MetricSuite) TestAddMetricDeadUnit(c *gc.C) {
	unit := s.assertAddUnit(c)
	assertUnitDead(c, unit)
	now := state.NowToTheSecond()
	m := state.NewMetric("item", "5", now, []byte{})
	_, err := unit.AddMetrics([]*state.Metric{m})
	c.Assert(err, gc.ErrorMatches, `wordpress/0 not found`)
}

func (s *MetricSuite) TestSetMetricSent(c *gc.C) {
	unit := s.assertAddUnit(c)
	now := state.NowToTheSecond()
	m := state.NewMetric("item", "5", now, []byte{})
	added, err := unit.AddMetrics([]*state.Metric{m})
	c.Assert(err, gc.IsNil)
	saved, err := s.State.MetricBatch(added.UUID())
	c.Assert(err, gc.IsNil)
	err = saved.SetSent()
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
	saved, err = s.State.MetricBatch(added.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Sent(), jc.IsTrue)
}
