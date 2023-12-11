// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	charm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type deployerSuite struct {
	baseSuite
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig(c)
	cfg.DataDir = ""
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.State = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ObjectStore = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ControllerConfig = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewCharmRepo = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewCharmDownloader = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.CharmhubHTTPClient = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.LoggerFactory = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *deployerSuite) TestControllerCharmArchWithDefaultArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	deployer := makeBaseDeployer(cfg)

	arch := deployer.ControllerCharmArch()
	c.Assert(arch, gc.Equals, "amd64")
}

func (s *deployerSuite) TestControllerCharmArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	cfg.Constraints = constraints.Value{
		Arch: ptr("arm64"),
	}
	deployer := makeBaseDeployer(cfg)

	arch := deployer.ControllerCharmArch()
	c.Assert(arch, gc.Equals, "arm64")
}

func (s *deployerSuite) TestDeployLocalCharmThatDoesNotExist(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	deployer := makeBaseDeployer(cfg)

	_, _, err := deployer.DeployLocalCharm(context.Background(), arch.DefaultArchitecture, base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *deployerSuite) XTestDeployLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	deployer := makeBaseDeployer(cfg)

	url, origin, err := deployer.DeployLocalCharm(context.Background(), "arm64", base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "juju-controller")
	c.Assert(origin, gc.DeepEquals, charm.Origin{
		Source: charm.Local,
		Type:   "charm",
		Platform: charm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	})
}
