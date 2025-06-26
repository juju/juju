// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	corebase "github.com/juju/juju/core/base"
)

type deployerIAASSuite struct {
	baseSuite
}

func TestDeployerIAASSuite(t *testing.T) {
	tc.Run(t, &deployerIAASSuite{})
}

func (s *deployerIAASSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, tc.IsNil)

	cfg = s.newConfig(c)
	cfg.ApplicationService = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.HostBaseFn = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *deployerIAASSuite) TestControllerCharmBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.Equals, corebase.MakeDefaultBase("ubuntu", "22.04"))
}

func (s *deployerIAASSuite) newDeployer(c *tc.C) *IAASDeployer {
	deployer, err := NewIAASDeployer(s.newConfig(c))
	c.Assert(err, tc.IsNil)
	return deployer
}

func (s *deployerIAASSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	return ctrl
}

func (s *deployerIAASSuite) newConfig(c *tc.C) IAASDeployerConfig {
	return IAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		ApplicationService: s.iaasApplicationService,
		HostBaseFn: func() (corebase.Base, error) {
			return corebase.MakeDefaultBase("ubuntu", "22.04"), nil
		},
	}
}
