// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprobebinder_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/observability/probe"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasprobebinder"
	"github.com/juju/juju/internal/worker/caasprober"
)

type binderSuite struct{}

var _ = gc.Suite(&binderSuite{})

func (s *binderSuite) TestBindingUnbinding(c *gc.C) {
	probes := caasprober.NewCAASProbes()
	worker, err := caasprobebinder.NewProbeBinder(probes, map[string]probe.ProbeProvider{
		"a": probe.LivenessProvider(probe.Failure),
		"b": probe.ReadinessProvider(probe.Failure),
		"c": probe.StartupProvider(probe.Failure),
	})
	c.Assert(err, jc.ErrorIsNil)
	checkAdded := func(pt probe.ProbeType) {
		for i := 0; i < 10; i++ {
			agg, ok := probes.ProbeAggregate(pt)
			c.Assert(ok, jc.IsTrue)
			ok, n, err := agg.Probe()
			c.Assert(err, jc.ErrorIsNil)
			if ok == false && n == 1 {
				return
			}
			time.Sleep(testing.ShortWait)
		}
		c.Fatalf("failed to wait for probe %s to be registered", pt)
	}
	checkAdded(probe.ProbeLiveness)
	checkAdded(probe.ProbeReadiness)
	checkAdded(probe.ProbeStartup)

	workertest.CleanKill(c, worker)

	checkRemoved := func(pt probe.ProbeType) {
		agg, ok := probes.ProbeAggregate(pt)
		c.Assert(ok, jc.IsTrue)
		ok, n, err := agg.Probe()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ok, jc.IsTrue)
		c.Assert(n, gc.Equals, 0)
	}
	checkRemoved(probe.ProbeLiveness)
	checkRemoved(probe.ProbeReadiness)
	checkRemoved(probe.ProbeStartup)
}
