// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type BootstrapConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store jujuclient.BootstrapConfigStore
}

var _ = gc.Suite(&BootstrapConfigSuite{})

func (s *BootstrapConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	writeTestBootstrapConfigFile(c)
}

func (s *BootstrapConfigSuite) TestBootstrapConfigForControllerNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuBootstrapConfigPath())
	c.Assert(err, jc.ErrorIsNil)
	details, err := s.store.BootstrapConfigForController("not-found")
	c.Assert(err, gc.ErrorMatches, "bootstrap config for controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *BootstrapConfigSuite) TestBootstrapConfigForControllerNotFound(c *gc.C) {
	details, err := s.store.BootstrapConfigForController("not-found")
	c.Assert(err, gc.ErrorMatches, "bootstrap config for controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *BootstrapConfigSuite) TestBootstrapConfigForController(c *gc.C) {
	cfg, err := s.store.BootstrapConfigForController("aws-test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.NotNil)
	c.Assert(*cfg, jc.DeepEquals, testBootstrapConfig["aws-test"])
}

func (s *BootstrapConfigSuite) TestUpdateBootstrapConfigNewController(c *gc.C) {
	err := s.store.UpdateBootstrapConfig("new-controller", testBootstrapConfig["mallards"])
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.store.BootstrapConfigForController("new-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*cfg, jc.DeepEquals, testBootstrapConfig["mallards"])
}

func (s *BootstrapConfigSuite) TestUpdateBootstrapConfigOverwrites(c *gc.C) {
	err := s.store.UpdateBootstrapConfig("aws-test", testBootstrapConfig["mallards"])
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.store.BootstrapConfigForController("aws-test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*cfg, jc.DeepEquals, testBootstrapConfig["mallards"])
}
