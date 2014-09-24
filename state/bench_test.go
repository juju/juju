// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type BenchmarkSuite struct {
}

var _ = gc.Suite(&BenchmarkSuite{})

func (*BenchmarkSuite) BenchmarkAddUnit(c *gc.C) {
	// TODO(rog) embed ConnSuite in BenchmarkSuite when
	// gocheck calls appropriate fixture methods for benchmark
	// functions.
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		_, err := svc.AddUnit()
		c.Assert(err, gc.IsNil)
	}
}

func (*BenchmarkSuite) BenchmarkAddAndAssignUnit(c *gc.C) {
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		unit, err := svc.AddUnit()
		c.Assert(err, gc.IsNil)
		err = s.State.AssignUnit(unit, state.AssignClean)
		c.Assert(err, gc.IsNil)
	}
}

func (*BenchmarkSuite) BenchmarkAddMetrics(c *gc.C) {
	metricsperBatch := 100
	batches := 10
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	now := time.Now()
	metrics := make([]state.Metric, metricsperBatch)
	for i, _ := range metrics {
		metrics[i] = state.Metric{
			Key:         "metricKey",
			Value:       "keyValue",
			Time:        now,
			Credentials: []byte("creds"),
		}
	}
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	serviceCharmURL, _ := svc.CharmURL()
	err = unit.SetCharmURL(serviceCharmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(err, gc.IsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		for n := 0; n < batches; n++ {
			_, err := unit.AddMetrics(now, metrics)
			c.Assert(err, gc.IsNil)
		}
	}
}

func (*BenchmarkSuite) BenchmarkCleanupMetrics(c *gc.C) {
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	oldTime := time.Now().Add(-(time.Hour * 25))
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	serviceCharmURL, _ := svc.CharmURL()
	err = unit.SetCharmURL(serviceCharmURL)
	c.Assert(err, gc.IsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		for i := 0; i < 50; i++ {
			m, err := unit.AddMetrics(oldTime, []state.Metric{{}})
			c.Assert(err, gc.IsNil)
			err = m.SetSent()
			c.Assert(err, gc.IsNil)
		}
		err := s.State.CleanupOldMetrics()
		c.Assert(err, gc.IsNil)
	}
}
