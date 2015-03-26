// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
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
		c.Assert(err, jc.ErrorIsNil)
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
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(unit, state.AssignClean)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (*BenchmarkSuite) BenchmarkAddMetrics1_1(c *gc.C)     { benchmarkAddMetrics(1, 1, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics1_10(c *gc.C)    { benchmarkAddMetrics(1, 10, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics1_100(c *gc.C)   { benchmarkAddMetrics(1, 100, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics100_1(c *gc.C)   { benchmarkAddMetrics(100, 1, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics100_10(c *gc.C)  { benchmarkAddMetrics(100, 10, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics100_100(c *gc.C) { benchmarkAddMetrics(100, 100, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics10_1(c *gc.C)    { benchmarkAddMetrics(10, 1, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics10_10(c *gc.C)   { benchmarkAddMetrics(10, 10, c) }
func (*BenchmarkSuite) BenchmarkAddMetrics10_100(c *gc.C)  { benchmarkAddMetrics(10, 100, c) }

func benchmarkAddMetrics(metricsPerBatch, batches int, c *gc.C) {
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	now := time.Now()
	metrics := make([]state.Metric, metricsPerBatch)
	for i := range metrics {
		metrics[i] = state.Metric{
			Key:   "metricKey",
			Value: "keyValue",
			Time:  now,
		}
	}
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	serviceCharmURL, _ := svc.CharmURL()
	err = unit.SetCharmURL(serviceCharmURL)
	c.Assert(err, jc.ErrorIsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		for n := 0; n < batches; n++ {
			_, err := unit.AddMetrics(utils.MustNewUUID().String(), now, "", metrics)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

// BenchmarkCleanupMetrics needs to add metrics each time over the cycle.
// Because of this the benchmark includes addmetric time
func (*BenchmarkSuite) BenchmarkCleanupMetrics(c *gc.C) {
	numberOfMetrics := 50
	var s ConnSuite
	s.SetUpSuite(c)
	defer s.TearDownSuite(c)
	s.SetUpTest(c)
	defer s.TearDownTest(c)
	oldTime := time.Now().Add(-(state.CleanupAge))
	charm := s.AddTestingCharm(c, "wordpress")
	svc := s.AddTestingService(c, "wordpress", charm)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	serviceCharmURL, _ := svc.CharmURL()
	err = unit.SetCharmURL(serviceCharmURL)
	c.Assert(err, jc.ErrorIsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		for i := 0; i < numberOfMetrics; i++ {
			m, err := unit.AddMetrics(utils.MustNewUUID().String(), oldTime, "", []state.Metric{{}})
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetSent()
			c.Assert(err, jc.ErrorIsNil)
		}
		err := s.State.CleanupOldMetrics()
		c.Assert(err, jc.ErrorIsNil)
	}
}
