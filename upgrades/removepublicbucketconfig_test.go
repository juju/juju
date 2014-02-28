// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/upgrades"
)

type removePublicBucketConfigSuite struct {
	jujutesting.JujuConnSuite
	ctx upgrades.Context
}

var _ = gc.Suite(&removePublicBucketConfigSuite{})

func (s *removePublicBucketConfigSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{dataDir: s.DataDir()},
		apiState:    apiState,
		st:          s.State,
	}
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	// Add in old public bucket config.
	newCfg, err := cfg.Apply(map[string]interface{}{
		"public-bucket":        "foo",
		"public-bucket-region": "bar",
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newCfg, cfg)
	c.Assert(err, gc.IsNil)
	cfg, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	allAttrs := cfg.AllAttrs()
	c.Assert(allAttrs["public-bucket"], gc.Equals, "foo")
	c.Assert(allAttrs["public-bucket-region"], gc.Equals, "bar")
}

func (s *removePublicBucketConfigSuite) assertConfigRemoved(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	allAttrs := cfg.AllAttrs()
	_, ok := allAttrs["public-bucket"]
	c.Assert(ok, jc.IsFalse)
	_, ok = allAttrs["public-bucket-region"]
	c.Assert(ok, jc.IsFalse)
}

func (s *removePublicBucketConfigSuite) TestPublicBucketConfigRemoved(c *gc.C) {
	err := upgrades.RemovePublicBucketConfig(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigRemoved(c)
}

func (s *removePublicBucketConfigSuite) TestIdempotent(c *gc.C) {
	err := upgrades.RemovePublicBucketConfig(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigRemoved(c)

	err = upgrades.RemovePublicBucketConfig(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigRemoved(c)
}
