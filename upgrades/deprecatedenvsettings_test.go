// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
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
		"tools-url":             "some.special.url.com",
	}
	err := s.State.UpdateEnvironConfig(newCfg, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *processDeprecatedEnvSettingsSuite) TestEnvSettingsSet(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := cfg.AllAttrs()
	c.Assert(allAttrs["public-bucket"], gc.Equals, "foo")
	c.Assert(allAttrs["public-bucket-region"], gc.Equals, "bar")
	c.Assert(allAttrs["public-bucket-url"], gc.Equals, "shazbot")
	c.Assert(allAttrs["default-instance-type"], gc.Equals, "vulch")
	c.Assert(allAttrs["default-image-id"], gc.Equals, "1234")
	c.Assert(allAttrs["shared-storage-port"], gc.Equals, 1234)
	c.Assert(allAttrs["tools-url"], gc.Equals, "some.special.url.com")
}

func (s *processDeprecatedEnvSettingsSuite) assertConfigProcessed(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := cfg.AllAttrs()
	for _, deprecated := range []string{
		"public-bucket", "public-bucket-region", "public-bucket-url",
		"default-image-id", "default-instance-type", "shared-storage-port", "tools-url",
	} {
		_, ok := allAttrs[deprecated]
		c.Assert(ok, jc.IsFalse)
	}
}

func (s *processDeprecatedEnvSettingsSuite) TestOldConfigRemoved(c *gc.C) {
	err := upgrades.ProcessDeprecatedEnvSettings(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigProcessed(c)
}

func (s *processDeprecatedEnvSettingsSuite) TestIdempotent(c *gc.C) {
	err := upgrades.ProcessDeprecatedEnvSettings(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigProcessed(c)

	err = upgrades.ProcessDeprecatedEnvSettings(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigProcessed(c)
}
