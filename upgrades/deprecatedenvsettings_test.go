// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upgrades"
)

type processDeprecatedEnvSettingsSuite struct {
	jujutesting.JujuConnSuite
	ctx upgrades.Context
}

var _ = gc.Suite(&processDeprecatedEnvSettingsSuite{})

func (s *processDeprecatedEnvSettingsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{dataDir: s.DataDir()},
		apiState:    apiState,
		state:       s.State,
	}
	// Add in old environment settings.
	newCfg := map[string]interface{}{
		"public-bucket":         "foo",
		"public-bucket-region":  "bar",
		"public-bucket-url":     "shazbot",
		"default-instance-type": "vulch",
		"default-image-id":      "1234",
		"shared-storage-port":   1234,
	}
	err := s.State.UpdateEnvironConfig(newCfg, nil, nil)
	c.Assert(err, gc.IsNil)
}

func (s *processDeprecatedEnvSettingsSuite) TestEnvSettingsSet(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	allAttrs := cfg.AllAttrs()
	c.Assert(allAttrs["public-bucket"], gc.Equals, "foo")
	c.Assert(allAttrs["public-bucket-region"], gc.Equals, "bar")
	c.Assert(allAttrs["public-bucket-url"], gc.Equals, "shazbot")
	c.Assert(allAttrs["default-instance-type"], gc.Equals, "vulch")
	c.Assert(allAttrs["default-image-id"], gc.Equals, "1234")
	c.Assert(allAttrs["shared-storage-port"], gc.Equals, 1234)
}

func (s *processDeprecatedEnvSettingsSuite) assertConfigProcessed(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	allAttrs := cfg.AllAttrs()
	for _, deprecated := range []string{
		"public-bucket", "public-bucket-region", "public-bucket-url",
		"default-image-id", "default-instance-type", "shared-storage-port",
	} {
		_, ok := allAttrs[deprecated]
		c.Assert(ok, jc.IsFalse)
	}
}

func (s *processDeprecatedEnvSettingsSuite) TestOldConfigRemoved(c *gc.C) {
	err := upgrades.ProcessDeprecatedEnvSettings(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}

func (s *processDeprecatedEnvSettingsSuite) TestIdempotent(c *gc.C) {
	err := upgrades.ProcessDeprecatedEnvSettings(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)

	err = upgrades.ProcessDeprecatedEnvSettings(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}
