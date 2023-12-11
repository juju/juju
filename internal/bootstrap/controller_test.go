// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/charm"
)

var (
	defaultBase = base.MustParseBaseFromString("22.04@ubuntu")
)

type ControllerSuite struct {
	baseSuite
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) TestPopulateControllerCharmLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := charm.Origin{
		Source: charm.Local,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalDeployment(origin)
	s.expectAddApplication(origin)
	s.expectCompletion()

	err := PopulateControllerCharm(context.Background(), s.deployer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestPopulateControllerCharmLocalCharmFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmError()

	err := PopulateControllerCharm(context.Background(), s.deployer)
	c.Assert(err, gc.ErrorMatches, `.*boom`)
}

func (s *ControllerSuite) TestPopulateControllerCharmCharmhubCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := charm.Origin{
		Source: charm.CharmHub,
		ID:     "deadbeef",
	}

	s.expectControllerAddress()
	s.expectCharmInfo()
	s.expectLocalCharmNotFound()
	s.expectCharmhubDeployment(origin)
	s.expectAddApplication(origin)
	s.expectCompletion()

	err := PopulateControllerCharm(context.Background(), s.deployer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) expectControllerAddress() {
	s.deployer.EXPECT().ControllerAddress(gomock.Any()).Return("10.0.0.1", nil)
}

func (s *ControllerSuite) expectCharmInfo() {
	s.deployer.EXPECT().ControllerCharmArch().Return(arch.DefaultArchitecture)
	s.deployer.EXPECT().ControllerCharmBase().Return(defaultBase, nil)
}

func (s *ControllerSuite) expectLocalDeployment(origin charm.Origin) {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return("juju-controller", &origin, nil)
}

func (s *ControllerSuite) expectLocalCharmNotFound() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return("", nil, errors.NotFoundf("not found"))
}

func (s *ControllerSuite) expectLocalCharmError() {
	s.deployer.EXPECT().DeployLocalCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return("", nil, errors.Errorf("boom"))
}

func (s *ControllerSuite) expectCharmhubDeployment(origin charm.Origin) {
	s.deployer.EXPECT().DeployCharmhubCharm(gomock.Any(), arch.DefaultArchitecture, defaultBase).Return("juju-controller", &origin, nil)
}

func (s *ControllerSuite) expectAddApplication(origin charm.Origin) {
	s.deployer.EXPECT().AddControllerApplication(gomock.Any(), "juju-controller", &origin, "10.0.0.1").Return(s.unit, nil)
}

func (s *ControllerSuite) expectCompletion() {
	s.deployer.EXPECT().CompleteProcess(gomock.Any(), s.unit).Return(nil)
}
