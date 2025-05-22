// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprobebinder_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/internal/observability/probe"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasprobebinder"
	"github.com/juju/juju/internal/worker/caasprober"
)

type binderSuite struct{}

func TestBinderSuite(t *stdtesting.T) {
	tc.Run(t, &binderSuite{})
}

func (s *binderSuite) TestBindingUnbinding(c *tc.C) {
	probes := caasprober.NewCAASProbes()
	worker, err := caasprobebinder.NewProbeBinder(probes, map[string]probe.ProbeProvider{
		"a": probe.LivenessProvider(probe.Failure),
		"b": probe.ReadinessProvider(probe.Failure),
		"c": probe.StartupProvider(probe.Failure),
	})
	c.Assert(err, tc.ErrorIsNil)
	checkAdded := func(pt probe.ProbeType) {
		for i := 0; i < 10; i++ {
			agg, ok := probes.ProbeAggregate(pt)
			c.Assert(ok, tc.IsTrue)
			ok, n, err := agg.Probe()
			c.Assert(err, tc.ErrorIsNil)
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
		c.Assert(ok, tc.IsTrue)
		ok, n, err := agg.Probe()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ok, tc.IsTrue)
		c.Assert(n, tc.Equals, 0)
	}
	checkRemoved(probe.ProbeLiveness)
	checkRemoved(probe.ProbeReadiness)
	checkRemoved(probe.ProbeStartup)
}
