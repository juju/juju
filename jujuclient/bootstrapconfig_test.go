// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type BootstrapConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store jujuclient.BootstrapConfigStore
}

var _ = tc.Suite(&BootstrapConfigSuite{})

func (s *BootstrapConfigSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	writeTestBootstrapConfigFile(c)
}

func (s *BootstrapConfigSuite) TestBootstrapConfigForControllerNoFile(c *tc.C) {
	err := os.Remove(jujuclient.JujuBootstrapConfigPath())
	c.Assert(err, jc.ErrorIsNil)
	details, err := s.store.BootstrapConfigForController("not-found")
	c.Assert(err, tc.ErrorMatches, "bootstrap config for controller not-found not found")
	c.Assert(details, tc.IsNil)
}

func (s *BootstrapConfigSuite) TestBootstrapConfigForControllerNotFound(c *tc.C) {
	details, err := s.store.BootstrapConfigForController("not-found")
	c.Assert(err, tc.ErrorMatches, "bootstrap config for controller not-found not found")
	c.Assert(details, tc.IsNil)
}

func (s *BootstrapConfigSuite) TestBootstrapConfigForController(c *tc.C) {
	cfg, err := s.store.BootstrapConfigForController("aws-test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, tc.NotNil)
	c.Assert(*cfg, jc.DeepEquals, testBootstrapConfig["aws-test"])
}

func (s *BootstrapConfigSuite) TestUpdateBootstrapConfigNewController(c *tc.C) {
	err := s.store.UpdateBootstrapConfig("new-controller", testBootstrapConfig["mallards"])
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.store.BootstrapConfigForController("new-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*cfg, jc.DeepEquals, testBootstrapConfig["mallards"])
}

func (s *BootstrapConfigSuite) TestUpdateBootstrapConfigOverwrites(c *tc.C) {
	err := s.store.UpdateBootstrapConfig("aws-test", testBootstrapConfig["mallards"])
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.store.BootstrapConfigForController("aws-test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*cfg, jc.DeepEquals, testBootstrapConfig["mallards"])
}
