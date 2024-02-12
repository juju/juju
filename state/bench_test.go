// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/uuid"
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
	app := s.AddTestingApplication(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		_, err := app.AddUnit(state.AddUnitParams{})
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
	app := s.AddTestingApplication(c, "wordpress", charm)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		unit, err := app.AddUnit(state.AddUnitParams{})
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
	ch := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	applicationCharmURL, _ := app.CharmURL()
	err = unit.SetCharmURL(*applicationCharmURL)
	c.Assert(err, jc.ErrorIsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		for n := 0; n < batches; n++ {
			_, err := s.State.AddMetrics(
				state.BatchParam{
					UUID:     uuid.MustNewUUID().String(),
					Created:  now,
					CharmURL: *applicationCharmURL,
					Metrics:  metrics,
					Unit:     unit.UnitTag(),
				},
			)
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
	ch := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	applicationCharmURL, _ := app.CharmURL()
	err = unit.SetCharmURL(*applicationCharmURL)
	c.Assert(err, jc.ErrorIsNil)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		for i := 0; i < numberOfMetrics; i++ {
			m, err := s.State.AddMetrics(
				state.BatchParam{
					UUID:     uuid.MustNewUUID().String(),
					Created:  oldTime,
					CharmURL: *applicationCharmURL,
					Metrics:  []state.Metric{{}},
					Unit:     unit.UnitTag(),
				},
			)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetSent(time.Now())
			c.Assert(err, jc.ErrorIsNil)
		}
		err := s.State.CleanupOldMetrics()
		c.Assert(err, jc.ErrorIsNil)
	}
}
