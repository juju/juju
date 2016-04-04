// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	"github.com/juju/juju/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) TestStartFuncs(c *gc.C) {
	manifolds := unit.Manifolds(unit.ManifoldsConfig{
		Agent: fakeAgent{},
	})

	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *gc.C) {
	config := unit.ManifoldsConfig{
		Agent:               nil,
		LogSource:           nil,
		LeadershipGuarantee: 0,
	}

	manifolds := unit.Manifolds(config)
	expectedKeys := []string{
		unit.APIAddressUpdaterName,
		unit.APICallerName,
		unit.APIConfigWatcherName,
		unit.AgentName,
		unit.CharmDirName,
		unit.HookRetryStrategyName,
		unit.LeadershipTrackerName,
		unit.LogSenderName,
		unit.LoggingConfigUpdaterName,
		unit.MachineLockName,
		unit.MeterStatusName,
		unit.MetricCollectName,
		unit.MetricSenderName,
		unit.MetricSpoolName,
		unit.MigrationMinionName,
		unit.ProxyConfigUpdaterName,
		unit.UniterName,
		unit.UpgraderName,
	}
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	c.Assert(expectedKeys, jc.SameContents, keys)
}

type fakeAgent struct {
	agent.Agent
}
