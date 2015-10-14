// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
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

// TODO(cmars) 2015/08/10: rework this into builtin Engine cycle checker.
func (s *ManifoldsSuite) TestAcyclic(c *gc.C) {
	manifolds := unit.Manifolds(unit.ManifoldsConfig{
		Agent: fakeAgent{},
	})
	err := dependency.Validate(manifolds)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldsSuite) TestManifoldNames(c *gc.C) {
	config := unit.ManifoldsConfig{
		Agent:               nil,
		LogSource:           nil,
		LeadershipGuarantee: 0,
	}

	manifolds := unit.Manifolds(config)
	expectedKeys := []string{
		unit.AgentName,
		unit.APIAdddressUpdaterName,
		unit.APICallerName,
		unit.APIInfoGateName,
		unit.LeadershipTrackerName,
		unit.LoggingConfigUpdaterName,
		unit.LogSenderName,
		unit.MachineLockName,
		unit.ProxyConfigUpdaterName,
		unit.RsyslogConfigUpdaterName,
		unit.UniterName,
		unit.UpgraderName,
		unit.MetricSpoolName,
		unit.MetricCollectName,
		unit.MeterStatusName,
		unit.MetricSenderName,
		unit.CharmDirName,
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
